// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"context"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v6/commons/constants"
	libZap "github.com/LerianStudio/lib-observability/v2/zap"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestUpdatedAmountsFromFee_PreservesLegacyRouteAndPromotesUUIDRoute(t *testing.T) {
	t.Parallel()

	uuidRoute := uuid.NewString()
	tests := []struct {
		name        string
		key         string
		expected    string
		expectRoute bool
	}{
		{
			name:        "fee debit UUID route",
			key:         "@payer->fee2->" + uuidRoute,
			expected:    uuidRoute,
			expectRoute: true,
		},
		{
			name:        "fee credit UUID route",
			key:         "@fee-revenue->fee_source2->@payer->" + uuidRoute,
			expected:    uuidRoute,
			expectRoute: true,
		},
		{
			name:     "legacy non UUID route",
			key:      "@payer->fee2->legacy-route",
			expected: "legacy-route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			legs := updatedAmountsFromFee(map[string]transaction.Amount{
				tt.key: {Asset: "USD", Value: decimal.NewFromInt(1)},
			})
			require.Len(t, legs, 1)
			assert.Equal(t, tt.expected, legs[0].Route)

			if tt.expectRoute {
				require.NotNil(t, legs[0].RouteID)
				assert.Equal(t, tt.expected, *legs[0].RouteID)
			} else {
				assert.Nil(t, legs[0].RouteID)
			}
		})
	}
}

func TestCalculateFeePreservingLegs_UUIDRoutesReachPostFeeValidation(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	fromRoute := uuid.NewString()
	toRoute := uuid.NewString()
	deductible := false
	feeCalc := &model.FeeCalculate{Transaction: transaction.Transaction{Send: transaction.Send{
		Asset: "USD",
		Value: decimal.NewFromInt(100),
		Source: transaction.Source{From: []transaction.FromTo{{
			AccountAlias: "@payer",
			Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
			IsFrom:       true,
		}}},
		Distribute: transaction.Distribute{To: []transaction.FromTo{{
			AccountAlias: "@receiver",
			Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
		}}},
	}}}
	feePackage := &pack.Package{
		Fees: map[string]model.Fee{"routed": {
			CalculationModel: &model.CalculationModel{
				ApplicationRule: feeconstant.AppRuleFlatFee,
				Calculations:    []model.Calculation{{Type: feeconstant.FeeTypeFlat, Value: "1"}},
			},
			ReferenceAmount:  feeconstant.ReferenceAmountOriginalAmount,
			Priority:         1,
			IsDeductibleFrom: &deductible,
			CreditAccount:    "@fee-revenue",
			RouteFrom:        &fromRoute,
			RouteTo:          &toRoute,
		}},
		WaivedAccounts: &[]string{},
	}
	response := &transaction.Responses{
		From: map[string]transaction.Amount{"@payer": {Asset: "USD", Value: decimal.NewFromInt(100)}},
		To:   map[string]transaction.Amount{"@receiver": {Asset: "USD", Value: decimal.NewFromInt(100)}},
	}

	err := CalculateFeePreservingLegs(logger, feeCalc, feePackage, response, "USD", nil)
	require.NoError(t, err)
	require.Len(t, feeCalc.Transaction.Send.Source.From, 2)
	require.Len(t, feeCalc.Transaction.Send.Distribute.To, 2)
	assert.Equal(t, fromRoute, feeCalc.Transaction.Send.Source.From[1].Route)
	require.NotNil(t, feeCalc.Transaction.Send.Source.From[1].RouteID)
	assert.Equal(t, fromRoute, *feeCalc.Transaction.Send.Source.From[1].RouteID)
	assert.Equal(t, toRoute, feeCalc.Transaction.Send.Distribute.To[1].Route)
	require.NotNil(t, feeCalc.Transaction.Send.Distribute.To[1].RouteID)
	assert.Equal(t, toRoute, *feeCalc.Transaction.Send.Distribute.To[1].RouteID)

	for i := range feeCalc.Transaction.Send.Source.From {
		feeCalc.Transaction.Send.Source.From[i].IsFrom = true
	}
	transaction.ApplyDefaultBalanceKeys(feeCalc.Transaction.Send.Source.From)
	transaction.ApplyDefaultBalanceKeys(feeCalc.Transaction.Send.Distribute.To)
	transaction.MutateConcatAliases(feeCalc.Transaction.Send.Source.From)
	transaction.MutateConcatAliases(feeCalc.Transaction.Send.Distribute.To)

	validation, err := transaction.ValidateSendSourceAndDistribute(context.Background(), feeCalc.Transaction, constant.CREATED)
	require.NoError(t, err)
	assert.Equal(t, fromRoute, validation.OperationRoutesFrom[feeCalc.Transaction.Send.Source.From[1].AccountAlias])
	assert.Equal(t, toRoute, validation.OperationRoutesTo[feeCalc.Transaction.Send.Distribute.To[1].AccountAlias])
}

func TestMaterializeAmountsAfterFee_OrdersSyntheticLegsByNumericFeeIndex(t *testing.T) {
	t.Parallel()

	original := []transaction.FromTo{{
		AccountAlias: "@payer",
		Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
	}}
	amounts := map[string]transaction.Amount{
		"@payer":                       {Asset: "USD", Value: decimal.NewFromInt(100)},
		"@payer->fee10->route-fee-ten": {Asset: "USD", Value: decimal.NewFromInt(10)},
		"@payer->fee2->route-fee-two":  {Asset: "USD", Value: decimal.NewFromInt(2)},
	}

	legs, err := materializeAmountsAfterFee(original, amounts)
	require.NoError(t, err)
	require.Len(t, legs, 3)
	assert.Equal(t, "route-fee-two", legs[1].Route)
	assert.Equal(t, decimal.NewFromInt(2), legs[1].Amount.Value)
	assert.Equal(t, "route-fee-ten", legs[2].Route)
	assert.Equal(t, decimal.NewFromInt(10), legs[2].Amount.Value)
}
