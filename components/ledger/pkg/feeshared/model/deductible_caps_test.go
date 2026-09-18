// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/stretchr/testify/require"
)

// deductibleFee builds a complete fee carrying a single calculation, the shape the
// caps below are measured against.
func deductibleFee(calcType, value string, deductible bool) Fee {
	rule := FlatFee
	if calcType == Percentage {
		rule = Percentual
	}

	return Fee{
		FeeLabel:         "Taxa",
		ReferenceAmount:  OriginalAmount,
		Priority:         2,
		CreditAccount:    "fee_account",
		IsDeductibleFrom: boolPtr(deductible),
		CalculationModel: &CalculationModel{
			ApplicationRule: rule,
			Calculations:    []Calculation{{Type: calcType, Value: value}},
		},
	}
}

// A deductible fee comes out of the payment, so a percentage above 100 takes more
// than the payment carries. The cap does not depend on the package declaring a
// minimum: the flat cap does, because it has nothing to compare against without one.
func TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		minAmount    string
		calc         Calculation
		isDeductible bool
		wantCode     string
	}{
		{
			name:         "deductible percentage over 100 without a minimum",
			minAmount:    "",
			calc:         Calculation{Type: Percentage, Value: "150"},
			isDeductible: true,
			wantCode:     constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name:         "deductible percentage at 100 without a minimum",
			minAmount:    "",
			calc:         Calculation{Type: Percentage, Value: "100"},
			isDeductible: true,
		},
		{
			name:         "percentage over 100 that is not deductible",
			minAmount:    "",
			calc:         Calculation{Type: Percentage, Value: "150"},
			isDeductible: false,
		},
		{
			name:         "deductible flat without a minimum has nothing to exceed",
			minAmount:    "",
			calc:         Calculation{Type: Flat, Value: "1000"},
			isDeductible: true,
		},
		{
			name:         "deductible percentage over 100 with a minimum",
			minAmount:    "100",
			calc:         Calculation{Type: Percentage, Value: "150"},
			isDeductible: true,
			wantCode:     constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name:         "deductible flat over the minimum",
			minAmount:    "100",
			calc:         Calculation{Type: Flat, Value: "150"},
			isDeductible: true,
			wantCode:     constant.ErrCalculationValueFlatFee.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &CalculationModel{
				ApplicationRule: MaxBetween,
				Calculations:    []Calculation{tt.calc},
			}

			err := validateCalculationValues(model, tt.minAmount, "fee1", tt.isDeductible)

			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantCode)
		})
	}
}

// The update boundary answers the same cap as a create for a patch that carries fees
// and no minimum, instead of letting the refusal happen deeper under the code that
// describes flipping the deductible flag.
func TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum(t *testing.T) {
	t.Parallel()

	up := &UpdatePackageInput{
		Fee: map[string]Fee{"fee1": deductibleFee(Percentage, "150", true)},
	}

	require.Nil(t, up.MinAmount, "the patch under test carries no minimum")
	require.ErrorContains(t, up.ValidateFees(), constant.ErrCalculationValuePercentage.Error())
}
