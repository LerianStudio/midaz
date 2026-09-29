// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// fakeReservationTenancy answers every Applies with a fixed outcome and
// counts the calls.
type fakeReservationTenancy struct {
	applies bool
	err     error
	calls   int
}

func (f *fakeReservationTenancy) Applies(context.Context) (bool, error) {
	f.calls++

	return f.applies, f.err
}

// tenancyCases are the three answers a tenancy can give: the tenant takes
// part in the reservation seam, it does not, or the answer is unknown.
func tenancyCases() []struct {
	name    string
	applies bool
	err     error
	want    error
	persist bool
} {
	outage := fmt.Errorf("%w: active tenant list unavailable", constant.ErrTenantServiceUnavailable)

	return []struct {
		name    string
		applies bool
		err     error
		want    error
		persist bool
	}{
		{name: "ledger tenant is refused", applies: true, want: constant.ErrContextLimitsUnavailable},
		{name: "validations-only tenant is persisted", applies: false, persist: true},
		{name: "unknown tenancy refuses the write", applies: false, err: outage, want: constant.ErrTenantServiceUnavailable},
	}
}

func TestUpdateLimitDefinitionPolicyFollowsTenancy(t *testing.T) {
	t.Parallel()

	for _, tc := range tenancyCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := NewMockLimitRepository(ctrl)
			audit := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			cmd, err := NewUpdateLimitCommand(repo, testutil.NewDefaultMockClock(), audit, txBeginner)
			require.NoError(t, err)

			tenancy := &fakeReservationTenancy{applies: tc.applies, err: tc.err}
			cmd.ContextLimits = definitionPolicyFixture(t).ScopedTo(tenancy)

			account := testutil.MustDeterministicUUID(941)
			merchant := testutil.MustDeterministicUUID(942)
			limit, err := model.NewLimit("Existing", model.LimitTypeDaily, decimal.NewFromInt(100), "BRL", []model.Scope{{AccountID: &account}}, nil, testutil.FixedTime())
			require.NoError(t, err)

			repo.EXPECT().GetByID(gomock.Any(), limit.ID).Return(limit, nil)

			if tc.persist {
				expectTxSuccess(t, repo, audit, txBeginner, pgdbMocks.NewMockTx(ctrl), limit.ID)
			} else {
				txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			}

			scopes := []model.Scope{{MerchantID: &merchant}}
			result, err := cmd.Execute(t.Context(), limit.ID, &UpdateLimitInput{Scopes: &scopes})
			require.Equal(t, 1, tenancy.calls, "the tenancy is asked once per write")

			if tc.persist {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, &merchant, result.Scopes[0].MerchantID)

				return
			}

			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestActivateLimitDefinitionPolicyFollowsTenancy(t *testing.T) {
	t.Parallel()

	for _, tc := range tenancyCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := NewMockLimitRepository(ctrl)
			audit := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			cmd, err := NewActivateLimitCommand(repo, testutil.NewDefaultMockClock(), audit, txBeginner)
			require.NoError(t, err)

			tenancy := &fakeReservationTenancy{applies: tc.applies, err: tc.err}
			cmd.ContextLimits = definitionPolicyFixture(t).ScopedTo(tenancy)

			merchant := testutil.MustDeterministicUUID(943)
			limit := &model.Limit{
				ID: testutil.MustDeterministicUUID(944), Name: "Merchant limit", LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.NewFromInt(100), Asset: "BRL", Scopes: []model.Scope{{MerchantID: &merchant}},
				Status: model.LimitStatusInactive, CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
			}

			repo.EXPECT().GetByID(gomock.Any(), limit.ID).Return(limit, nil)

			if tc.persist {
				expectLimitStatusTxSuccess(t, txBeginner, pgdbMocks.NewMockTx(ctrl), repo, audit, limit.ID, model.LimitStatusActive,
					model.AuditEventLimitActivated, model.AuditActionActivate, "Limit activated via API", gomock.Any())
			} else {
				txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			}

			result, err := cmd.Execute(t.Context(), limit.ID)
			require.Equal(t, 1, tenancy.calls, "the tenancy is asked once per write")

			if tc.persist {
				require.NoError(t, err)
				require.Equal(t, model.LimitStatusActive, result.Status)

				return
			}

			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestContextLimitDefinitionPolicyScopedToLeavesTheOriginalUnscoped(t *testing.T) {
	t.Parallel()

	merchant := testutil.MustDeterministicUUID(945)
	broad := &model.Limit{
		ID: testutil.MustDeterministicUUID(946), Name: "Merchant limit", LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.NewFromInt(100), Asset: "BRL", Scopes: []model.Scope{{MerchantID: &merchant}},
	}

	original := definitionPolicyFixture(t)
	tenancy := &fakeReservationTenancy{applies: false}
	scoped := original.ScopedTo(tenancy)

	require.NotSame(t, original, scoped, "scoping returns a copy")
	require.Nil(t, original.tenancy, "the original policy keeps no tenancy")
	require.Same(t, tenancy, scoped.tenancy)
	require.Equal(t, original.bounds, scoped.bounds)
	require.Equal(t, original.maxScopes, scoped.maxScopes)
	require.Equal(t, original.maxScopeBytes, scoped.maxScopeBytes)

	require.NoError(t, scoped.validate(t.Context(), broad), "a tenant outside the seam keeps any scope")
	require.ErrorIs(t, original.validate(t.Context(), broad), constant.ErrContextLimitsUnavailable,
		"the unscoped original still governs every tenant")
	require.Equal(t, 1, tenancy.calls, "only the scoped copy asks the tenancy")

	var disabled *ContextLimitDefinitionPolicy
	require.Nil(t, disabled.ScopedTo(tenancy), "scoping the legacy profile installs no policy")
}
