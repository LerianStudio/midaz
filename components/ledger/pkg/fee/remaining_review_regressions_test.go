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

func TestUpdatedAmountsFromFee_UUIDLookingLegacyRouteIsNeverPromoted(t *testing.T) {
	t.Parallel()

	uuidRoute := uuid.NewString()
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "fee debit UUID-looking legacy route",
			key:      "@payer->fee2->" + uuidRoute,
			expected: uuidRoute,
		},
		{
			name:     "fee credit UUID-looking legacy route",
			key:      "@fee-revenue->fee_source2->@payer->" + uuidRoute,
			expected: uuidRoute,
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
			}, nil)
			require.Len(t, legs, 1)
			assert.Equal(t, tt.expected, legs[0].Route)
			assert.Nil(t, legs[0].RouteID)
		})
	}
}

func TestCalculateFeePreservingLegs_ExplicitOperationRoutesReachPostFeeValidation(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	legacyFrom := uuid.NewString()
	legacyTo := uuid.NewString()
	operationFrom := uuid.NewString()
	operationTo := uuid.NewString()
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
			ReferenceAmount:      feeconstant.ReferenceAmountOriginalAmount,
			Priority:             1,
			IsDeductibleFrom:     &deductible,
			CreditAccount:        "@fee-revenue",
			RouteFrom:            &legacyFrom,
			RouteTo:              &legacyTo,
			OperationRouteFromID: &operationFrom,
			OperationRouteToID:   &operationTo,
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
	assert.Equal(t, legacyFrom, feeCalc.Transaction.Send.Source.From[1].Route)
	require.NotNil(t, feeCalc.Transaction.Send.Source.From[1].RouteID)
	assert.Equal(t, operationFrom, *feeCalc.Transaction.Send.Source.From[1].RouteID)
	assert.Equal(t, legacyTo, feeCalc.Transaction.Send.Distribute.To[1].Route)
	require.NotNil(t, feeCalc.Transaction.Send.Distribute.To[1].RouteID)
	assert.Equal(t, operationTo, *feeCalc.Transaction.Send.Distribute.To[1].RouteID)

	for i := range feeCalc.Transaction.Send.Source.From {
		feeCalc.Transaction.Send.Source.From[i].IsFrom = true
	}
	transaction.ApplyDefaultBalanceKeys(feeCalc.Transaction.Send.Source.From)
	transaction.ApplyDefaultBalanceKeys(feeCalc.Transaction.Send.Distribute.To)
	transaction.MutateConcatAliases(feeCalc.Transaction.Send.Source.From)
	transaction.MutateConcatAliases(feeCalc.Transaction.Send.Distribute.To)

	validation, err := transaction.ValidateSendSourceAndDistribute(context.Background(), feeCalc.Transaction, constant.CREATED)
	require.NoError(t, err)
	assert.Equal(t, operationFrom, validation.OperationRoutesFrom[feeCalc.Transaction.Send.Source.From[1].AccountAlias])
	assert.Equal(t, operationTo, validation.OperationRoutesTo[feeCalc.Transaction.Send.Distribute.To[1].AccountAlias])
}

func TestCalculateFeePreservingLegs_ExplicitOperationRoutesSurviveMultipleAndDeductibleFees(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	deductible := true
	nonDeductible := false
	deductibleTo := uuid.NewString()
	nonDeductibleFrom := uuid.NewString()
	nonDeductibleTo := uuid.NewString()
	deductibleLabel := "deductible-credit"
	nonDeductibleFromLabel := "non-deductible-debit"
	nonDeductibleToLabel := "non-deductible-credit"

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
		Fees: map[string]model.Fee{
			"deductible": {
				CalculationModel: &model.CalculationModel{
					ApplicationRule: feeconstant.AppRuleFlatFee,
					Calculations:    []model.Calculation{{Type: feeconstant.FeeTypeFlat, Value: "10"}},
				},
				ReferenceAmount:    feeconstant.ReferenceAmountOriginalAmount,
				Priority:           1,
				IsDeductibleFrom:   &deductible,
				CreditAccount:      "@fee-deductible",
				RouteTo:            &deductibleLabel,
				OperationRouteToID: &deductibleTo,
			},
			"non-deductible": {
				CalculationModel: &model.CalculationModel{
					ApplicationRule: feeconstant.AppRuleFlatFee,
					Calculations:    []model.Calculation{{Type: feeconstant.FeeTypeFlat, Value: "5"}},
				},
				ReferenceAmount:      feeconstant.ReferenceAmountOriginalAmount,
				Priority:             2,
				IsDeductibleFrom:     &nonDeductible,
				CreditAccount:        "@fee-non-deductible",
				RouteFrom:            &nonDeductibleFromLabel,
				RouteTo:              &nonDeductibleToLabel,
				OperationRouteFromID: &nonDeductibleFrom,
				OperationRouteToID:   &nonDeductibleTo,
			},
		},
		WaivedAccounts: &[]string{},
	}
	response := &transaction.Responses{
		From: map[string]transaction.Amount{"@payer": {Asset: "USD", Value: decimal.NewFromInt(100)}},
		To:   map[string]transaction.Amount{"@receiver": {Asset: "USD", Value: decimal.NewFromInt(100)}},
	}

	require.NoError(t, CalculateFeePreservingLegs(logger, feeCalc, feePackage, response, "USD", nil))
	assert.True(t, decimal.NewFromInt(105).Equal(feeCalc.Transaction.Send.Value))
	require.Len(t, feeCalc.Transaction.Send.Source.From, 2)
	require.Len(t, feeCalc.Transaction.Send.Distribute.To, 3)

	nonDeductibleDebit := feeCalc.Transaction.Send.Source.From[1]
	assert.Equal(t, nonDeductibleFromLabel, nonDeductibleDebit.Route)
	require.NotNil(t, nonDeductibleDebit.RouteID)
	assert.Equal(t, nonDeductibleFrom, *nonDeductibleDebit.RouteID)

	deductibleCredit := feeCalc.Transaction.Send.Distribute.To[1]
	assert.Equal(t, "@fee-deductible", deductibleCredit.AccountAlias)
	assert.Equal(t, deductibleLabel, deductibleCredit.Route)
	require.NotNil(t, deductibleCredit.RouteID)
	assert.Equal(t, deductibleTo, *deductibleCredit.RouteID)

	nonDeductibleCredit := feeCalc.Transaction.Send.Distribute.To[2]
	assert.Equal(t, "@fee-non-deductible", nonDeductibleCredit.AccountAlias)
	assert.Equal(t, nonDeductibleToLabel, nonDeductibleCredit.Route)
	require.NotNil(t, nonDeductibleCredit.RouteID)
	assert.Equal(t, nonDeductibleTo, *nonDeductibleCredit.RouteID)
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

	legs, err := materializeAmountsAfterFee(original, amounts, nil)
	require.NoError(t, err)
	require.Len(t, legs, 3)
	assert.Equal(t, "route-fee-two", legs[1].Route)
	assert.Equal(t, decimal.NewFromInt(2), legs[1].Amount.Value)
	assert.Equal(t, "route-fee-ten", legs[2].Route)
	assert.Equal(t, decimal.NewFromInt(10), legs[2].Amount.Value)
}

func TestMaterializeAmountsAfterFee_RouteLabelCannotOverrideStructuralFeeIndex(t *testing.T) {
	t.Parallel()

	original := []transaction.FromTo{{
		AccountAlias: "@payer",
		Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
	}}
	amounts := map[string]transaction.Amount{
		"@payer":                        {Asset: "USD", Value: decimal.NewFromInt(100)},
		"@payer->fee2->fee_source10":    {Asset: "USD", Value: decimal.NewFromInt(2)},
		"@payer->fee3->route-fee-three": {Asset: "USD", Value: decimal.NewFromInt(3)},
	}

	legs, err := materializeAmountsAfterFee(original, amounts, nil)
	require.NoError(t, err)
	require.Len(t, legs, 3)
	assert.Equal(t, "fee_source10", legs[1].Route)
	assert.Equal(t, decimal.NewFromInt(2), legs[1].Amount.Value)
	assert.Equal(t, "route-fee-three", legs[2].Route)
	assert.Equal(t, decimal.NewFromInt(3), legs[2].Amount.Value)
}
