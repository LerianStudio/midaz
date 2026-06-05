// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package fee

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/zap"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FuzzCalculateFee_Amount fuzzes the flat fee calculation with varying transaction amounts
// and fee values to detect panics, overflows, or unexpected errors.
func FuzzCalculateFee_Amount(f *testing.F) {
	// Seed corpus with representative values
	f.Add(int64(1000), int64(100))     // Normal case: 1000 amount, 100 fee
	f.Add(int64(0), int64(0))          // Zero case
	f.Add(int64(1), int64(1))          // Minimum positive values
	f.Add(int64(999999999), int64(50)) // Large amount, small fee
	f.Add(int64(100), int64(999999))   // Small amount, large fee

	f.Fuzz(func(t *testing.T, amount int64, feeValue int64) {
		// Skip negative values since financial amounts must be non-negative
		if amount < 0 || feeValue < 0 {
			return
		}

		logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
		amountDec := decimal.NewFromInt(amount)
		feeValueDec := decimal.NewFromInt(feeValue)

		feeCalc := &model.FeeCalculate{
			Transaction: transaction.Transaction{
				Send: transaction.Send{
					Asset: "BRL",
					Value: amountDec,
					Source: transaction.Source{
						From: []transaction.FromTo{{
							Amount: &transaction.Amount{Asset: "BRL", Value: amountDec},
						}},
					},
					Distribute: transaction.Distribute{
						To: []transaction.FromTo{{
							Amount: &transaction.Amount{Asset: "BRL", Value: amountDec},
						}},
					},
				},
			},
		}

		fee := model.Fee{
			FeeLabel: "FuzzFee",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: constant.AppRuleFlatFee,
				Calculations: []model.Calculation{{
					Type:  constant.FeeTypeFlat,
					Value: feeValueDec.String(),
				}},
			},
			ReferenceAmount:  "originalAmount",
			Priority:         1,
			IsDeductibleFrom: func() *bool { b := false; return &b }(),
			CreditAccount:    "@fee_account",
		}

		p := &pack.Package{
			ID:             uuid.New(),
			Fees:           map[string]model.Fee{"fuzz": fee},
			WaivedAccounts: &[]string{},
		}

		resp := &transaction.Responses{
			From: map[string]transaction.Amount{
				"@from_account": {Asset: "BRL", Value: amountDec},
			},
			To: map[string]transaction.Amount{
				"@to_account": {Asset: "BRL", Value: amountDec},
			},
		}

		// The function must not panic regardless of input
		_ = CalculateFee(logger, feeCalc, p, resp, "BRL", nil)
	})
}

// FuzzCalculateFee_Percentage fuzzes the percentage fee calculation with varying
// transaction amounts and percentage values.
func FuzzCalculateFee_Percentage(f *testing.F) {
	// Seed corpus with representative values
	f.Add(int64(1000), int64(10))      // Normal: 10% of 1000
	f.Add(int64(1000), int64(100))     // Edge: 100% of 1000
	f.Add(int64(0), int64(50))         // Zero amount
	f.Add(int64(1), int64(1))          // Minimum positive values
	f.Add(int64(999999999), int64(99)) // Large amount, near-max percentage

	f.Fuzz(func(t *testing.T, amount int64, percentValue int64) {
		// Skip negative values and percentages above 100
		if amount < 0 || percentValue < 0 || percentValue > 100 {
			return
		}

		logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
		amountDec := decimal.NewFromInt(amount)
		percentDec := decimal.NewFromInt(percentValue)

		feeCalc := &model.FeeCalculate{
			Transaction: transaction.Transaction{
				Send: transaction.Send{
					Asset: "BRL",
					Value: amountDec,
					Source: transaction.Source{
						From: []transaction.FromTo{{
							Amount: &transaction.Amount{Asset: "BRL", Value: amountDec},
						}},
					},
					Distribute: transaction.Distribute{
						To: []transaction.FromTo{{
							Amount: &transaction.Amount{Asset: "BRL", Value: amountDec},
						}},
					},
				},
			},
		}

		fee := model.Fee{
			FeeLabel: "FuzzPercentFee",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: constant.AppRulePercentual,
				Calculations: []model.Calculation{{
					Type:  constant.FeeTypePercentage,
					Value: percentDec.String(),
				}},
			},
			ReferenceAmount:  "originalAmount",
			Priority:         1,
			IsDeductibleFrom: func() *bool { b := false; return &b }(),
			CreditAccount:    "@fee_account",
		}

		p := &pack.Package{
			ID:             uuid.New(),
			Fees:           map[string]model.Fee{"fuzz": fee},
			WaivedAccounts: &[]string{},
		}

		resp := &transaction.Responses{
			From: map[string]transaction.Amount{
				"@from_account": {Asset: "BRL", Value: amountDec},
			},
			To: map[string]transaction.Amount{
				"@to_account": {Asset: "BRL", Value: amountDec},
			},
		}

		// The function must not panic regardless of input
		_ = CalculateFee(logger, feeCalc, p, resp, "BRL", nil)
	})
}
