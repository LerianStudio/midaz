// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg"
)

func requireDeferrableRefusal(t *testing.T, err error) {
	t.Helper()

	var refusal pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, "0530", refusal.Code)
}

func TestCreatePackageInput_ValidateFees_Deferrable(t *testing.T) {
	create := func(fee string) error {
		var input CreatePackageInput
		require.NoError(t, json.Unmarshal([]byte(`{"minimumAmount":"100","fees":{"fee1":{"priority":1,"referenceAmount":"originalAmount",
			"calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"10"}]},`+fee+`}}}`), &input))

		return input.ValidateFees()
	}

	requireDeferrableRefusal(t, create(`"isDeductibleFrom":true,"deferrable":true`))
	assert.NoError(t, create(`"isDeductibleFrom":false,"deferrable":true`))
	assert.NoError(t, create(`"isDeductibleFrom":true`), "absent deferrable is false")

	requireDeferrableRefusal(t, (&Fee{
		FeeLabel: "new", ReferenceAmount: OriginalAmount, Priority: 1, CreditAccount: "@fees",
		IsDeductibleFrom: boolPtr(true), Deferrable: boolPtr(true),
		CalculationModel: &CalculationModel{ApplicationRule: FlatFee, Calculations: []Calculation{{Type: Flat, Value: "10"}}},
	}).ValidateNewFee("new", decimal.NewFromInt(100)))
}

func TestFee_SetAndValidateHasFieldsToUpdate_Deferrable(t *testing.T) {
	stored := func(deductible, deferrable bool) map[string]Fee {
		return map[string]Fee{"fee1": {
			FeeLabel: "stored", ReferenceAmount: OriginalAmount, Priority: 1, CreditAccount: "@fees",
			IsDeductibleFrom: &deductible, Deferrable: &deferrable,
			CalculationModel: &CalculationModel{ApplicationRule: FlatFee, Calculations: []Calculation{{Type: Flat, Value: "10"}}},
		}}
	}
	patch := func(fee Fee, existing map[string]Fee) (bson.M, bool, error) {
		fields := bson.M{}
		updated, err := fee.SetAndValidateHasFieldsToUpdate(context.Background(), fee.IsDeductibleFrom, decimal.NewFromInt(100),
			existing, "fee1", uuid.New(), uuid.New(), fields, nil)

		return fields, updated, err
	}

	_, _, err := patch(Fee{Deferrable: boolPtr(true)}, stored(true, false))
	requireDeferrableRefusal(t, err)

	_, _, err = patch(Fee{IsDeductibleFrom: boolPtr(true)}, stored(false, true))
	requireDeferrableRefusal(t, err)

	fields, updated, err := patch(Fee{Deferrable: boolPtr(true)}, stored(false, false))
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, bson.M{"fees.fee1.deferrable": true}, fields)

	fields, updated, err = patch(Fee{Deferrable: boolPtr(false)}, stored(false, true))
	require.NoError(t, err)
	assert.True(t, updated, "turning deferrable off edits the fee, it does not remove it")
	assert.Equal(t, bson.M{"fees.fee1.deferrable": false}, fields)
}
