// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// A patch that moves the minimum must leave every fee the package keeps within it.
// The fees the patch restates are validated as they are applied, against this same
// new minimum, so they are skipped here.
func TestUpdatePackageInputValidateStoredFeesAgainstMinimum(t *testing.T) {
	tests := []struct {
		name       string
		newMinimum *string
		storedFees map[string]Fee
		patch      map[string]Fee
		wantCode   string
	}{
		{
			name:       "minimum lowered below a stored deductible flat fee",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "minimum lowered to exactly the stored deductible flat fee",
			newMinimum: stringPtr("25"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
		},
		{
			name:       "stored flat fee is not deductible",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", false)},
		},
		{
			name:       "patch restates the calculations of the offending fee",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": deductibleFee(Flat, "1", true)},
		},
		{
			name:       "patch names the fee but changes only its label",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {FeeLabel: "Novo rotulo"}},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "patch key differs in case from the stored key",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"feeOne": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"FeeOne": deductibleFee(Flat, "1", true)},
		},
		{
			name:       "patch carries no minimum",
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
		},
		{
			name:       "stored deductible percentage above 100",
			newMinimum: stringPtr("50"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Percentage, "150", true)},
			wantCode:   constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name:       "stored fee without a calculation model",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": {FeeLabel: "Taxa", IsDeductibleFrom: boolPtr(true)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{MinAmount: tt.newMinimum, Fee: tt.patch}

			err := up.ValidateStoredFeesAgainstMinimum(tt.storedFees)

			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantCode)
		})
	}
}

// The minimum a patched fee is measured against is the one the package will carry.
func TestUpdatePackageInputEffectiveMinimumAmount(t *testing.T) {
	stored := decimal.NewFromInt(100)

	t.Run("patch carries a minimum", func(t *testing.T) {
		up := &UpdatePackageInput{MinAmount: stringPtr("900")}

		effective, err := up.EffectiveMinimumAmount(stored)

		require.NoError(t, err)
		require.True(t, effective.Equal(decimal.NewFromInt(900)), "got %s", effective)
	})

	t.Run("patch carries no minimum", func(t *testing.T) {
		up := &UpdatePackageInput{}

		effective, err := up.EffectiveMinimumAmount(stored)

		require.NoError(t, err)
		require.True(t, effective.Equal(stored), "got %s", effective)
	})

	t.Run("patch carries an unparseable minimum", func(t *testing.T) {
		up := &UpdatePackageInput{MinAmount: stringPtr("100,00")}

		_, err := up.EffectiveMinimumAmount(stored)

		require.ErrorContains(t, err, constant.ErrConvertToDecimal.Error())
	})
}
