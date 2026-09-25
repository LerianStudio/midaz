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
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func definitionPolicyFixture(t *testing.T) *ContextLimitDefinitionPolicy {
	t.Helper()
	policy, err := NewContextLimitDefinitionPolicy(mocks.NewMockLimitAssetRepository(gomock.NewController(t)), tracercontract.Limits{MaxAccounts: 2, MaxEntries: 4, MaxTextBytes: 256, MaxIntegerDigits: 8, MaxFractionDigits: 8}, 2, 4096)
	require.NoError(t, err)
	return policy
}

func TestSharedLimitCreateRejectsIneligibleDefinitionsBeforeWrite(t *testing.T) {
	for _, scenario := range []string{"broad", "fraction", "integer"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cmd, err := NewCreateLimitCommand(NewMockLimitRepository(ctrl), testutil.NewDefaultMockClock(), NewMockAuditWriter(ctrl), pgdbMocks.NewMockTxBeginner(ctrl))
			require.NoError(t, err)
			cmd.NativeAssetCodes, cmd.ContextLimits = true, definitionPolicyFixture(t)
			id := testutil.MustDeterministicUUID(911)
			input := &CreateLimitInput{Name: "Account limit", LimitType: model.LimitTypeDaily, Asset: "wBTC", MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{AccountID: &id}}}
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
