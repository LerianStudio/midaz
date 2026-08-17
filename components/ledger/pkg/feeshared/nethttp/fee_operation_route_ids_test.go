// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

func TestFeeOperationRouteIDsAreOptionalUUIDsOnCreateAndUpdate(t *testing.T) {
	t.Parallel()

	enabled := true
	deductible := false
	validFrom := uuid.NewString()
	validTo := uuid.NewString()
	invalid := "not-a-uuid"

	createInput := func(fromID, toID *string) *model.CreatePackageInput {
		return &model.CreatePackageInput{
			FeeGroupLabel: "routed fees",
			LedgerID:      uuid.NewString(),
			MinAmount:     "1",
			MaxAmount:     "1000",
			Enable:        &enabled,
			Fee: map[string]model.Fee{"service": {
				FeeLabel: "service fee",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: model.FlatFee,
					Calculations:    []model.Calculation{{Type: model.Flat, Value: "1"}},
				},
				ReferenceAmount:      model.OriginalAmount,
				Priority:             1,
				IsDeductibleFrom:     &deductible,
				CreditAccount:        "@fee-revenue",
				OperationRouteFromID: fromID,
				OperationRouteToID:   toID,
			}},
		}
	}

	require.NoError(t, ValidateStruct(createInput(&validFrom, &validTo)))
	assert.Error(t, ValidateStruct(createInput(&invalid, &validTo)))
	assert.Error(t, ValidateStruct(createInput(&validFrom, &invalid)))

	require.NoError(t, createInput(&validFrom, &validTo).ValidateFees())
	assert.Error(t, createInput(&invalid, &validTo).ValidateFees())
	assert.Error(t, createInput(&validFrom, &invalid).ValidateFees())

	validUpdate := &model.UpdatePackageInput{Fee: map[string]model.Fee{"service": {
		OperationRouteFromID: &validFrom,
		OperationRouteToID:   &validTo,
	}}}
	require.NoError(t, validUpdate.ValidateFees())

	invalidUpdate := &model.UpdatePackageInput{Fee: map[string]model.Fee{"service": {
		OperationRouteFromID: &invalid,
	}}}
	assert.Error(t, invalidUpdate.ValidateFees())
}
