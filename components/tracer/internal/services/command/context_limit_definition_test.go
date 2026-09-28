// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func definitionPolicyFixture(t *testing.T) *ContextLimitDefinitionPolicy {
	t.Helper()
	policy, err := NewContextLimitDefinitionPolicy(tracercontract.Limits{MaxAccounts: 2, MaxEntries: 4, MaxTextBytes: 256, MaxIntegerDigits: 8, MaxFractionDigits: 8}, 2, 4096)
	require.NoError(t, err)
	return policy
}

func TestSharedLimitCreateRejectsIneligibleDefinitionsBeforeWrite(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"broad", "fraction", "integer"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			cmd, err := NewCreateLimitCommand(NewMockLimitRepository(ctrl), testutil.NewDefaultMockClock(), NewMockAuditWriter(ctrl), pgdbMocks.NewMockTxBeginner(ctrl))
			require.NoError(t, err)
			cmd.ContextLimits = definitionPolicyFixture(t)
			id := testutil.MustDeterministicUUID(911)
			input := &CreateLimitInput{Name: "Account limit", LimitType: model.LimitTypeDaily, Asset: "BTC", MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{AccountID: &id}}}
			switch scenario {
			case "broad":
				input.Scopes = []model.Scope{{SegmentID: &id}}
			case "fraction":
				input.MaxAmount = decimal.RequireFromString("0.000000001")
			case "integer":
				input.MaxAmount = decimal.RequireFromString("100000000")
			}
			result, err := cmd.Execute(t.Context(), input)
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, result)
		})
	}
}

func TestSharedLimitUpdateRejectsResourceOverflowBeforeWrite(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMockLimitRepository(ctrl)
	cmd, err := NewUpdateLimitCommand(repo, testutil.NewDefaultMockClock(), nil, pgdbMocks.NewMockTxBeginner(ctrl))
	require.NoError(t, err)
	cmd.ContextLimits = definitionPolicyFixture(t)
	account := testutil.MustDeterministicUUID(912)
	limit, err := model.NewLimit("Existing", model.LimitTypeDaily, decimal.NewFromInt(100), "USD", []model.Scope{{AccountID: &account}}, nil, testutil.FixedTime())
	require.NoError(t, err)
	repo.EXPECT().GetByID(gomock.Any(), limit.ID).Return(limit, nil)
	amount := decimal.RequireFromString("0.000000001")
	result, err := cmd.Execute(t.Context(), limit.ID, &UpdateLimitInput{MaxAmount: &amount})
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, result)
	limit.MaxAmount = decimal.RequireFromString("100.000000000000000")
	require.NoError(t, cmd.ContextLimits.validate(t.Context(), limit), "padding is not financial precision")
}

func TestNewContextLimitDefinitionPolicyRejectsMissingBounds(t *testing.T) {
	t.Parallel()
	bounds := tracercontract.Limits{MaxAccounts: 2, MaxEntries: 4, MaxTextBytes: 256, MaxIntegerDigits: 8, MaxFractionDigits: 8}
	for name, build := range map[string]func() (*ContextLimitDefinitionPolicy, error){
		"no scopes":      func() (*ContextLimitDefinitionPolicy, error) { return NewContextLimitDefinitionPolicy(bounds, 0, 4096) },
		"no scope bytes": func() (*ContextLimitDefinitionPolicy, error) { return NewContextLimitDefinitionPolicy(bounds, 2, 0) },
		"invalid bounds": func() (*ContextLimitDefinitionPolicy, error) {
			return NewContextLimitDefinitionPolicy(tracercontract.Limits{}, 2, 4096)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			policy, err := build()
			require.Error(t, err)
			require.Nil(t, policy)
		})
	}
}

func TestContextLimitActivationUsesOnlyTheLimitDefinition(t *testing.T) {
	t.Parallel()
	account := testutil.MustDeterministicUUID(913)
	segment := testutil.MustDeterministicUUID(914)
	for _, tc := range []struct {
		name   string
		asset  string
		scopes []model.Scope
		want   error
	}{
		{name: "account-only BTC activates", asset: "BTC", scopes: []model.Scope{{AccountID: &account}}},
		{name: "long code activates", asset: "LERIANPOINTS", scopes: []model.Scope{{AccountID: &account}}},
		{name: "broad scope is ineligible", asset: "BTC", scopes: []model.Scope{{SegmentID: &segment}}, want: constant.ErrContextLimitsUnavailable},
		{name: "lowercase code is ineligible", asset: "btc", scopes: []model.Scope{{AccountID: &account}}, want: constant.ErrContextLimitsUnavailable},
		{name: "empty code is ineligible", asset: "", scopes: []model.Scope{{AccountID: &account}}, want: constant.ErrContextLimitsUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limit := &model.Limit{ID: testutil.MustDeterministicUUID(915), Name: "Account limit", LimitType: model.LimitTypeDaily, MaxAmount: decimal.NewFromInt(100), Asset: tc.asset, Scopes: tc.scopes, Status: model.LimitStatusInactive}
			err := definitionPolicyFixture(t).validateActivation(t.Context(), limit)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}

	var disabled *ContextLimitDefinitionPolicy
	require.NoError(t, disabled.validateActivation(t.Context(), &model.Limit{Asset: "btc"}), "the legacy profile installs no policy")
	require.ErrorIs(t, definitionPolicyFixture(t).validateActivation(t.Context(), nil), constant.ErrContextLimitsUnavailable)
}

func TestSharedLimitActivateRejectsIneligibleLimitBeforeWrite(t *testing.T) {
	t.Parallel()
	for _, status := range []model.LimitStatus{model.LimitStatusInactive, model.LimitStatusActive} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			repo := NewMockLimitRepository(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
			cmd, err := NewActivateLimitCommand(repo, testutil.NewDefaultMockClock(), NewMockAuditWriter(ctrl), txBeginner)
			require.NoError(t, err)
			cmd.ContextLimits = definitionPolicyFixture(t)
			segment := testutil.MustDeterministicUUID(916)
			limit := &model.Limit{ID: testutil.MustDeterministicUUID(917), Name: "Broad limit", LimitType: model.LimitTypeDaily, MaxAmount: decimal.NewFromInt(100), Asset: "BTC", Scopes: []model.Scope{{SegmentID: &segment}}, Status: status, CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime()}
			repo.EXPECT().GetByID(gomock.Any(), limit.ID).Return(limit, nil)
			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			result, err := cmd.Execute(t.Context(), limit.ID)
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, result)
		})
	}
}

func TestSharedLimitActivateAcceptsEligibleAccountOnlyLimit(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMockLimitRepository(ctrl)
	audit := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	tx := pgdbMocks.NewMockTx(ctrl)
	cmd, err := NewActivateLimitCommand(repo, testutil.NewDefaultMockClock(), audit, txBeginner)
	require.NoError(t, err)
	cmd.ContextLimits = definitionPolicyFixture(t)
	account := testutil.MustDeterministicUUID(918)
	limit := &model.Limit{ID: testutil.MustDeterministicUUID(919), Name: "Account limit", LimitType: model.LimitTypeDaily, MaxAmount: decimal.NewFromInt(100), Asset: "BTC", Scopes: []model.Scope{{AccountID: &account}}, Status: model.LimitStatusInactive, CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime()}
	repo.EXPECT().GetByID(gomock.Any(), limit.ID).Return(limit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, tx, repo, audit, limit.ID, model.LimitStatusActive, model.AuditEventLimitActivated, model.AuditActionActivate, "Limit activated via API", gomock.Any())
	result, err := cmd.Execute(t.Context(), limit.ID)
	require.NoError(t, err)
	require.Equal(t, model.LimitStatusActive, result.Status)
	require.Equal(t, "BTC", result.Asset)
}
