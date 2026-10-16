// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	scopeTestOrganizationID = uuid.MustParse("5c0e0000-0000-4000-8000-000000000001")
	scopeTestLedgerID       = uuid.MustParse("5c0e0000-0000-4000-8000-000000000002")
	scopeTestAccountID      = uuid.MustParse("5c0e0000-0000-4000-8000-000000000003")
)

var (
	scopeTestEnforce  = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}
	scopeTestAdvisory = mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory}
)

// scopeTestSources is a source list whose first internal leg is @source.
var scopeTestSources = []string{"@source#" + constant.DefaultBalanceKey}

// scopeTestBalances returns the balance @source resolves to, owned by the
// scope test organization and ledger.
func scopeTestBalances(accountID string) []*mmodel.Balance {
	return []*mmodel.Balance{{
		OrganizationID: scopeTestOrganizationID.String(),
		LedgerID:       scopeTestLedgerID.String(),
		Alias:          "@source",
		Key:            constant.DefaultBalanceKey,
		AccountID:      accountID,
		AccountType:    "deposit",
	}}
}

func scopeTestPtr(value string) *string { return &value }

func TestReservationActive(t *testing.T) {
	reserver := &stubReserver{}

	tests := []struct {
		name     string
		reserver TracerReserver
		settings mmodel.TracerSettings
		skip     bool
		want     bool
	}{
		{name: "nil reserver", reserver: nil, settings: scopeTestEnforce, want: false},
		{name: "mode off", reserver: reserver, settings: mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, want: false},
		{name: "empty mode", reserver: reserver, settings: mmodel.TracerSettings{}, want: false},
		{name: "honored skip", reserver: reserver, settings: scopeTestEnforce, skip: true, want: false},
		{name: "advisory", reserver: reserver, settings: scopeTestAdvisory, want: true},
		{name: "enforce", reserver: reserver, settings: scopeTestEnforce, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reservationActive(tc.reserver, tc.settings, tc.skip))
		})
	}
}

func TestReserveSourceAccount_InactiveSeamReadsNothing(t *testing.T) {
	ctx, span, logger := anchorDeps()

	tests := []struct {
		name     string
		reserver TracerReserver
		settings mmodel.TracerSettings
		skip     bool
	}{
		{name: "nil reserver", reserver: nil, settings: scopeTestEnforce},
		{name: "mode off", reserver: &stubReserver{}, settings: mmodel.TracerSettings{Mode: mmodel.TracerModeOff}},
		{name: "empty mode", reserver: &stubReserver{}, settings: mmodel.TracerSettings{}},
		{name: "honored skip", reserver: &stubReserver{}, settings: scopeTestEnforce, skip: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := account.NewMockRepository(ctrl)
			repo.EXPECT().ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			uc := &UseCase{TracerReserver: tc.reserver, AccountRepo: repo}

			got, err := uc.reserveSourceAccount(ctx, span, logger, tc.settings, tc.skip,
				scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
			require.NoError(t, err)
			assert.Equal(t, tracer.ReserveAccount{}, got, "an inactive seam builds no reserve account")
		})
	}
}

func TestReserveSourceAccount_NoInternalSourceReadsNothing(t *testing.T) {
	ctx, span, logger := anchorDeps()
	ctrl := gomock.NewController(t)
	repo := account.NewMockRepository(ctrl)
	repo.EXPECT().ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}

	got, err := uc.reserveSourceAccount(ctx, span, logger, scopeTestEnforce, false,
		[]string{"@external/BRL#" + constant.DefaultBalanceKey}, scopeTestBalances(scopeTestAccountID.String()))
	require.NoError(t, err)
	assert.Equal(t, tracer.ReserveAccount{}, got, "an external-only source carries no account")
}

func TestReserveSourceAccount_NilAccountRepoSendsNoScope(t *testing.T) {
	ctx, span, logger := anchorDeps()
	uc := &UseCase{TracerReserver: &stubReserver{}}

	got, err := uc.reserveSourceAccount(ctx, span, logger, scopeTestEnforce, false,
		scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
	require.NoError(t, err)
	assert.Equal(t, tracer.ReserveAccount{AccountID: scopeTestAccountID.String(), Type: "deposit"}, got)
}

func TestReserveSourceAccount_InvalidIDSendsNoScope(t *testing.T) {
	ctx, span, logger := anchorDeps()

	tests := []struct {
		name     string
		balances func() []*mmodel.Balance
	}{
		{name: "account id is not a uuid", balances: func() []*mmodel.Balance {
			return scopeTestBalances("acc-source-1")
		}},
		{name: "organization id is not a uuid", balances: func() []*mmodel.Balance {
			balances := scopeTestBalances(scopeTestAccountID.String())
			balances[0].OrganizationID = "org"

			return balances
		}},
		{name: "ledger id is not a uuid", balances: func() []*mmodel.Balance {
			balances := scopeTestBalances(scopeTestAccountID.String())
			balances[0].LedgerID = ""

			return balances
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := account.NewMockRepository(ctrl)
			repo.EXPECT().ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}
			balances := tc.balances()

			got, err := uc.reserveSourceAccount(ctx, span, logger, scopeTestEnforce, false, scopeTestSources, balances)
			require.NoError(t, err)
			assert.Equal(t, tracer.ReserveAccount{AccountID: balances[0].AccountID, Type: "deposit"}, got)
		})
	}
}

func TestReserveSourceAccount_ResolvesScopeFromBalanceLedger(t *testing.T) {
	tests := []struct {
		name          string
		segmentID     *string
		portfolioID   *string
		wantSegment   string
		wantPortfolio string
	}{
		{name: "segment and portfolio", segmentID: scopeTestPtr("seg-1"), portfolioID: scopeTestPtr("port-1"), wantSegment: "seg-1", wantPortfolio: "port-1"},
		{name: "segment only", segmentID: scopeTestPtr("seg-1"), wantSegment: "seg-1"},
		{name: "portfolio only", portfolioID: scopeTestPtr("port-1"), wantPortfolio: "port-1"},
		{name: "neither", wantSegment: "", wantPortfolio: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, span, ended := recordingSpan(t)
			ctrl := gomock.NewController(t)
			repo := account.NewMockRepository(ctrl)
			repo.EXPECT().
				ListAccountsByIDs(gomock.Any(), scopeTestOrganizationID, scopeTestLedgerID, []uuid.UUID{scopeTestAccountID}).
				Return([]*mmodel.Account{{ID: scopeTestAccountID.String(), SegmentID: tc.segmentID, PortfolioID: tc.portfolioID}}, nil).
				Times(1)

			uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}

			got, err := uc.reserveSourceAccount(ctx, span, &libLog.NopLogger{}, scopeTestEnforce, false,
				scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
			require.NoError(t, err)
			assert.Equal(t, tracer.ReserveAccount{
				AccountID: scopeTestAccountID.String(), Type: "deposit",
				SegmentID: tc.wantSegment, PortfolioID: tc.wantPortfolio,
			}, got)

			attrs := spanAttributes(ended())
			assert.Equal(t, tc.wantSegment != "", attrs["app.tracer.source_segment_resolved"].AsBool())
			assert.Equal(t, tc.wantPortfolio != "", attrs["app.tracer.source_portfolio_resolved"].AsBool())
		})
	}
}

func TestReserveSourceAccount_NotFoundSendsNoScope(t *testing.T) {
	for _, settings := range []mmodel.TracerSettings{scopeTestEnforce, scopeTestAdvisory} {
		t.Run(string(settings.Mode), func(t *testing.T) {
			ctx, span, logger := anchorDeps()
			ctrl := gomock.NewController(t)
			repo := account.NewMockRepository(ctrl)
			repo.EXPECT().
				ListAccountsByIDs(gomock.Any(), scopeTestOrganizationID, scopeTestLedgerID, []uuid.UUID{scopeTestAccountID}).
				Return([]*mmodel.Account{}, nil).
				Times(1)

			uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}

			got, err := uc.reserveSourceAccount(ctx, span, logger, settings, false,
				scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
			require.NoError(t, err, "an account gone since the balance load never fails the create")
			assert.Equal(t, tracer.ReserveAccount{AccountID: scopeTestAccountID.String(), Type: "deposit"}, got)
		})
	}
}

func TestReserveSourceAccount_TechnicalErrorAdvisorySendsNoScope(t *testing.T) {
	ctx, span, ended := recordingSpan(t)
	ctrl := gomock.NewController(t)
	repo := account.NewMockRepository(ctrl)
	repo.EXPECT().
		ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection reset")).
		Times(1)

	uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}

	got, err := uc.reserveSourceAccount(ctx, span, &libLog.NopLogger{}, scopeTestAdvisory, false,
		scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
	require.NoError(t, err, "advisory never blocks")
	assert.Equal(t, tracer.ReserveAccount{AccountID: scopeTestAccountID.String(), Type: "deposit"}, got)

	spans := ended()
	require.Len(t, spans, 1)

	var names []string
	for _, event := range spans[0].Events() {
		names = append(names, event.Name)
	}

	assert.Contains(t, names, reserveSourceScopeUnresolvedEvent)
}

func TestReserveSourceAccount_TechnicalErrorEnforceFails(t *testing.T) {
	ctx, span, logger := anchorDeps()
	cause := errors.New("connection reset")
	ctrl := gomock.NewController(t)
	repo := account.NewMockRepository(ctrl)
	repo.EXPECT().
		ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, cause).
		Times(1)

	uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}

	got, err := uc.reserveSourceAccount(ctx, span, logger, scopeTestEnforce, false,
		scopeTestSources, scopeTestBalances(scopeTestAccountID.String()))
	require.Error(t, err)
	assert.ErrorIs(t, err, cause, "the technical error is wrapped, not replaced")
	assert.Equal(t, tracer.ReserveAccount{}, got)
}
