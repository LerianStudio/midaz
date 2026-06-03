// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package model

import (
	"testing"

	pkgHttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
)

// FuzzCreatePackageInput_Validate fuzzes the CreatePackageInput validation
// with arbitrary string inputs for amount fields and fee group labels.
// This tests for panics, unexpected nil dereferences, and validation edge cases.
func FuzzCreatePackageInput_Validate(f *testing.F) {
	// Seed corpus with representative values
	f.Add("Pacote Padrao", "100.00", "1000.00")      // Normal valid input
	f.Add("", "100", "1000")                         // Empty label
	f.Add("Test", "0", "0")                          // Zero amounts
	f.Add("Test", "not-a-number", "1000")            // Invalid min amount
	f.Add("Test", "100", "not-a-number")             // Invalid max amount
	f.Add("Test", "1000", "100")                     // Min greater than max
	f.Add("Test", "100,50", "1000,00")               // Comma-separated decimals
	f.Add("Test", "-100", "1000")                    // Negative min amount
	f.Add("Test", "99999999999999999999.99", "0.01") // Very large min, small max

	f.Fuzz(func(t *testing.T, feeGroupLabel, minAmount, maxAmount string) {
		enable := true

		input := &CreatePackageInput{
			FeeGroupLabel: feeGroupLabel,
			LedgerID:      "00000000-0000-0000-0000-000000000001",
			MinAmount:     minAmount,
			MaxAmount:     maxAmount,
			Enable:        &enable,
			Fee: map[string]Fee{
				"testFee": {
					FeeLabel: "Test Fee",
					CalculationModel: &CalculationModel{
						ApplicationRule: "flatFee",
						Calculations: []Calculation{
							{Type: "flat", Value: "10"},
						},
					},
					ReferenceAmount:  "originalAmount",
					Priority:         1,
					IsDeductibleFrom: &enable,
					CreditAccount:    "credit_account",
				},
			},
		}

		// ValidateStruct must not panic regardless of input
		_ = pkgHttp.ValidateStruct(input)

		// ValidateMinAndMaxAmount must not panic regardless of input
		_ = input.ValidateMinAndMaxAmount()

		// ValidateFees must not panic regardless of input
		_ = input.ValidateFees()
	})
}
