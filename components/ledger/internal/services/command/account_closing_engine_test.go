// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountClosingMarkerStore is the protection surface of these tests: it records
// which accounts are owned so the resolution of an execution can be observed.
type accountClosingMarkerStore struct {
	owned map[uuid.UUID]string
}

func newAccountClosingMarkerStore() *accountClosingMarkerStore {
	return &accountClosingMarkerStore{owned: map[uuid.UUID]string{}}
}

func (s *accountClosingMarkerStore) reset() { s.owned = map[uuid.UUID]string{} }

func (s *accountClosingMarkerStore) ownedAccounts() int { return len(s.owned) }

func (s *accountClosingMarkerStore) GetAccountClosingMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, bool, error) {
	return "", false, nil
}

func (s *accountClosingMarkerStore) GetAccountClosedMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (s *accountClosingMarkerStore) SetAccountClosedMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}

func (s *accountClosingMarkerStore) AcquireAccountAdminOwnership(_ context.Context, _, _, accountID uuid.UUID, token string) (bool, error) {
	if _, owned := s.owned[accountID]; owned {
		return false, nil
	}

	s.owned[accountID] = token

	return true, nil
}

func (s *accountClosingMarkerStore) ReleaseAccountAdminOwnership(_ context.Context, _, _, accountID uuid.UUID, token string) (bool, error) {
	if s.owned[accountID] != token {
		return false, nil
	}

	delete(s.owned, accountID)

	return true, nil
}

// accountClosingEngineError is an engine failure with a chosen code and outcome.
type accountClosingEngineError struct {
	code          string
	indeterminate bool
}

func (e accountClosingEngineError) Error() string              { return "engine " + e.code }
func (e accountClosingEngineError) EngineFailureCode() string  { return e.code }
func (e accountClosingEngineError) OutcomeIndeterminate() bool { return e.indeterminate }

func TestAccountClosingEngineRefusalsMapToTheirBusinessCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		want error
	}{
		{name: "closed account", code: "account_closed", want: constant.ErrAccountClosed},
		{name: "closing in progress", code: "account_closing_in_progress", want: constant.ErrAccountClosingInProgress},
		{name: "admission not confirmed", code: "admission_not_confirmed", want: constant.ErrAccountClosingProtectionIndeterminate},
		{name: "unreadable marker", code: "account_protection_unreadable", want: constant.ErrAccountClosingProtectionIndeterminate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mapped := MapEngineError(accounting.Execution{}, accountClosingEngineError{code: test.code})
			require.NotNil(t, mapped)
			assert.Equal(t, test.want.Error(), accountClosingBusinessCode(t, mapped))
		})
	}
}

// accountClosingBusinessCode extracts the public code of a mapped business error.
func accountClosingBusinessCode(t *testing.T, err error) string {
	t.Helper()

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		return unprocessable.Code
	}

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var unavailable pkg.ServiceUnavailableError
	if errors.As(err, &unavailable) {
		return unavailable.Code
	}

	t.Fatalf("unexpected mapped error type %T", err)

	return ""
}

func TestAccountClosingIndeterminateEngineFailureStaysTechnical(t *testing.T) {
	t.Parallel()

	// An unresolved outcome may have moved money: it must never be reported as a
	// refusal the caller can act on.
	mapped := MapEngineError(accounting.Execution{}, accountClosingEngineError{code: "account_closed", indeterminate: true})
	require.ErrorContains(t, mapped, "engine technical failure")
}

func TestResolveEngineAdmissionsKeepsAnUnresolvedExecutionProtected(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	accountID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	store := newAccountClosingMarkerStore()

	tests := []struct {
		name      string
		outcome   EngineExecutionOutcome
		err       error
		wantOwned int
	}{
		{name: "success releases", outcome: EngineExecutionOutcome{Executed: true}, wantOwned: 0},
		{name: "never submitted releases", outcome: EngineExecutionOutcome{}, err: errors.New("prepare failed"), wantOwned: 0},
		{
			name: "confirmed precommit refusal releases", outcome: EngineExecutionOutcome{Executed: true},
			err: accountClosingEngineError{code: "account_closed"}, wantOwned: 0,
		},
		{
			name: "unknown outcome keeps the ownership", outcome: EngineExecutionOutcome{Executed: true},
			err: accountClosingEngineError{code: "indeterminate", indeterminate: true}, wantOwned: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store.reset()

			ctx, sink := accountprotection.ContextWithSink(context.Background())

			admission, err := accountprotection.NewGuard(nil, store).AcquireAdmission(ctx, organizationID, ledgerID, []uuid.UUID{accountID})
			require.NoError(t, err)
			require.True(t, accountprotection.AdoptAdmission(ctx, admission))

			resolveEngineAdmissions(sink, accounting.Execution{}, test.outcome, test.err)
			sink.Release(ctx)

			assert.Equal(t, test.wantOwned, store.ownedAccounts())
		})
	}
}

// TestAccountClosingCompatibilityPathRefusesAnUnavailableAccount covers AS-03 and
// AS-06 on the paths that admit balances without reaching the engine, where the
// same controls are read inside the atomic execution. The check is markers only:
// no ownership is taken and no account row is read.
func TestAccountClosingCompatibilityPathRefusesAnUnavailableAccount(t *testing.T) {
	balances := []*mmodel.Balance{{
		ID:        "cccccccc-0000-0000-0000-000000000001",
		AccountID: protectionAccountID.String(),
		Alias:     "@source",
		Key:       constant.DefaultBalanceKey,
	}}

	t.Run("closed account", func(t *testing.T) {
		m := newProtectionMocks(t)
		m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return(protectionClosedAt, true, nil)

		err := m.uc.ensureBalanceAccountsAvailable(context.Background(), protectionOrgID, protectionLedgerID, balances)
		require.Error(t, err)
		assert.Equal(t, constant.ErrAccountClosed.Error(), accountClosingBusinessCode(t, err))
	})

	t.Run("closing in progress", func(t *testing.T) {
		m := newProtectionMocks(t)
		m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return(time.Time{}, false, nil)
		m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return(uuid.NewString(), true, nil)

		err := m.uc.ensureBalanceAccountsAvailable(context.Background(), protectionOrgID, protectionLedgerID, balances)
		require.Error(t, err)
		assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), accountClosingBusinessCode(t, err))
	})

	t.Run("unreadable marker", func(t *testing.T) {
		m := newProtectionMocks(t)
		m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return(time.Time{}, false, errors.New("cache unavailable"))

		err := m.uc.ensureBalanceAccountsAvailable(context.Background(), protectionOrgID, protectionLedgerID, balances)
		require.Error(t, err)
		assert.Equal(t, constant.ErrAccountClosingProtectionIndeterminate.Error(), accountClosingBusinessCode(t, err))
	})

	t.Run("open account passes without reading its row", func(t *testing.T) {
		m := newProtectionMocks(t)
		m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return(time.Time{}, false, nil)
		m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
			Return("", false, nil)

		require.NoError(t, m.uc.ensureBalanceAccountsAvailable(context.Background(), protectionOrgID, protectionLedgerID, balances))
	})
}
