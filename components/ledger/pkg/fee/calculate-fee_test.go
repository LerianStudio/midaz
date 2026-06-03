// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/zap"
	transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// Unit test for CalculateFee in pkg/fee
func TestCalculateFee_Basic(t *testing.T) {
	t.Parallel()

	// Create a mock logger
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	// Setup FeeCalculate input
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}
	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}
	fees := map[string]model.Fee{"test": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{
			"@from_account": {
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
			},
		},
		To: map[string]transaction.Amount{
			"@to_account": {
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
			},
		},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	expectedValue := decimal.NewFromInt(1100)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedValue),
		"expected Send.Value=%s, got %s", expectedValue.String(), feeCalc.Transaction.Send.Value.String())
	assert.NotEmpty(t, feeCalc.Transaction.Send.Source.From)
	assert.NotEmpty(t, feeCalc.Transaction.Send.Distribute.To)
}

func TestCalculateFee_Percentage(t *testing.T) {
	t.Parallel()

	// Create a mock logger
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}
	fee := model.Fee{
		FeeLabel: "PercentFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}
	fees := map[string]model.Fee{"percent": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}
	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}
	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	expectedValue := decimal.NewFromInt(1100) // 1000 + 10% = 1100
	// Compare using .Equal to avoid scale mismatch
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedValue), "expected %s, got %s", expectedValue.String(), feeCalc.Transaction.Send.Value.String())
}

func TestCalculateFee_InvalidApplicationRule(t *testing.T) {
	t.Parallel()

	// Create a mock logger
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}
	fee := model.Fee{
		FeeLabel: "InvalidRule",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "invalidRule",
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}
	fees := map[string]model.Fee{"invalid": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}
	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}
	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
}

func TestCalculateFee_InvalidFeeValue(t *testing.T) {
	t.Parallel()

	// Create a mock logger
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}
	fee := model.Fee{
		FeeLabel: "InvalidValue",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "not_a_number",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}
	fees := map[string]model.Fee{"invalid": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}
	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}
	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
}

func TestIsRepeatingDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    decimal.Decimal
		expected bool
	}{
		{
			name:     "0.25 should not be repeating",
			value:    decimal.NewFromFloat(0.25),
			expected: false,
		},
		{
			name:     "0.5 should not be repeating",
			value:    decimal.NewFromFloat(0.5),
			expected: false,
		},
		{
			name:     "0.1 should not be repeating",
			value:    decimal.NewFromFloat(0.1),
			expected: false,
		},
		{
			name:     "0.333333 should be repeating",
			value:    decimal.NewFromFloat(1.0 / 3.0),
			expected: true,
		},
		{
			name:     "0.666666 should be repeating",
			value:    decimal.NewFromFloat(2.0 / 3.0),
			expected: true,
		},
		{
			name:     "0.142857 should be repeating (1/7)",
			value:    decimal.NewFromFloat(1.0 / 7.0),
			expected: true,
		},
		{
			name:     "0.75 should not be repeating",
			value:    decimal.NewFromFloat(0.75),
			expected: false,
		},
		{
			name:     "0.125 should not be repeating",
			value:    decimal.NewFromFloat(0.125),
			expected: false,
		},
		{
			name:     "0.0 should not be repeating",
			value:    decimal.Zero,
			expected: false,
		},
		{
			name:     "1.0 should not be repeating",
			value:    decimal.NewFromInt(1),
			expected: false,
		},
		{
			name:     "Integer without decimal part should not be repeating",
			value:    decimal.NewFromInt(5),
			expected: false,
		},
		{
			name:     "0.090909 should be repeating",
			value:    decimal.NewFromFloat(1.0 / 11.0),
			expected: true,
		},
		{
			name:     "0.166666 should be repeating",
			value:    decimal.NewFromFloat(1.0 / 6.0),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRepeatingDecimal(tt.value)
			if result != tt.expected {
				t.Errorf("isRepeatingDecimal(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

// TestFindPackageToCalculateFee tests all package search scenarios
func TestFindPackageToCalculateFee(t *testing.T) {
	route1 := "debitoted"
	route2 := "creditfrom"
	segmentID1 := uuid.New()
	segmentID2 := uuid.New()

	tests := []struct {
		name             string
		packages         []*pack.Package
		transactionRoute string
		segmentID        *uuid.UUID
		amount           decimal.Decimal
		expectedPackage  *pack.Package
		expectedError    bool
		errorMessage     string
	}{
		{
			name: "Encontra pacote único por rota",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
				{ID: uuid.New(), TransactionRoute: &route2, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			},
			transactionRoute: route1,
			segmentID:        nil,
			amount:           decimal.NewFromInt(500),
			expectedPackage:  &pack.Package{TransactionRoute: &route1},
			expectedError:    false,
		},
		{
			name: "Encontra pacote único por segmento",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, SegmentID: &segmentID1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
				{ID: uuid.New(), TransactionRoute: &route1, SegmentID: &segmentID2, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			},
			transactionRoute: route1,
			segmentID:        &segmentID1,
			amount:           decimal.NewFromInt(500),
			expectedPackage:  &pack.Package{SegmentID: &segmentID1},
			expectedError:    false,
		},
		{
			name: "Encontra pacote único por valor",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(600), MaximumAmount: decimal.NewFromInt(1000)},
			},
			transactionRoute: route1,
			segmentID:        nil,
			amount:           decimal.NewFromInt(300),
			expectedPackage:  &pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
			expectedError:    false,
		},
		{
			name: "Retorna nil quando nenhum pacote encontrado por rota",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
			},
			transactionRoute: route2,
			segmentID:        nil,
			amount:           decimal.NewFromInt(300),
			expectedPackage:  nil,
			expectedError:    false,
		},
		{
			name: "Retorna nil quando valor está fora do range de todos os pacotes",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(600), MaximumAmount: decimal.NewFromInt(800)},
			},
			transactionRoute: route1,
			segmentID:        nil,
			amount:           decimal.NewFromInt(50),
			expectedPackage:  nil,
			expectedError:    false,
		},
		{
			name: "Retorna erro quando múltiplos pacotes encontrados",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			},
			transactionRoute: route1,
			segmentID:        nil,
			amount:           decimal.NewFromInt(500),
			expectedPackage:  nil,
			expectedError:    true,
			errorMessage:     "more than one package was found",
		},
		{
			name: "Filtra por rota vazia quando TransactionRoute é nil",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: nil, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
				{ID: uuid.New(), TransactionRoute: &route1, MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			},
			transactionRoute: "",
			segmentID:        nil,
			amount:           decimal.NewFromInt(500),
			expectedPackage:  &pack.Package{TransactionRoute: nil},
			expectedError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FindPackageToCalculateFee(tt.packages, tt.transactionRoute, tt.segmentID, tt.amount)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
				assert.Nil(t, result)
			} else {
				if tt.expectedPackage == nil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
					if tt.expectedPackage.TransactionRoute != nil {
						assert.Equal(t, *tt.expectedPackage.TransactionRoute, *result.TransactionRoute)
					}
					if tt.expectedPackage.SegmentID != nil {
						assert.Equal(t, *tt.expectedPackage.SegmentID, *result.SegmentID)
					}
				}
				assert.NoError(t, err)
			}
		})
	}
}

// TestCalculateFee_MaxBetweenTypes tests calculation with maxBetweenTypes rule
func TestCalculateFee_MaxBetweenTypes(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "MaxBetweenFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: constant.FeeTypeFlat, Value: "50"},
				{Type: constant.FeeTypePercentage, Value: "5"},
			},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"max": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	expectedValue := decimal.NewFromInt(1050)
	assert.True(t, feeCalc.Transaction.Send.Value.GreaterThanOrEqual(expectedValue))
}

// TestCalculateFee_AfterFeesAmount tests calculation with afterFeesAmount
func TestCalculateFee_AfterFeesAmount(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "AfterFeesFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "10",
			}},
		},
		ReferenceAmount:  constant.ReferenceAmountAfterFeesAmount,
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"after": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	assert.Greater(t, feeCalc.Transaction.Send.Value.IntPart(), int64(1000))
}

// TestCalculateFee_IsDeductibleFrom tests calculation with isDeductibleFrom = true
func TestCalculateFee_IsDeductibleFrom(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "DeductibleFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := true; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"deductible": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(decimal.NewFromInt(1000)),
		"expected Send.Value=1000, got %s", feeCalc.Transaction.Send.Value.String())
}

// TestCalculateFee_IsDeductibleFrom_ExemptSourceSkipsFee verifies that when the
// source (From) account is exempt via waivedAccounts, a deductible fee is not applied
// to the destination (To) accounts. This is the fix for FINDING-001: the transaction
// initiator's exemption status determines whether the fee triggers at all.
func TestCalculateFee_IsDeductibleFrom_ExemptSourceSkipsFee(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	amount := decimal.NewFromInt(1000)
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: amount,
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: "premium_alice",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "regular_bob",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "DeductibleFlatFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := true; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"deductible": fee}
	waived := []string{"premium_alice"}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &waived,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"premium_alice": {Asset: "BRL", Value: amount}},
		To:   map[string]transaction.Amount{"regular_bob": {Asset: "BRL", Value: amount}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	// send.value should NOT be increased (deductible fees don't increase send.value)
	assert.Equal(t, amount, feeCalc.Transaction.Send.Value)

	// Destination should receive the full amount — no deduction applied
	assert.Equal(t, amount, resp.To["regular_bob"].Value,
		"exempt source account should prevent deductible fee from being applied to destination")

	// feeExemption metadata should be set
	assert.NotNil(t, feeCalc.Transaction.Metadata, "metadata should be set when all source accounts are exempt")
	exemption, ok := feeCalc.Transaction.Metadata["feeExemption"].(map[string]any)
	assert.True(t, ok, "feeExemption should be a map")
	assert.Equal(t, true, exemption["exempt"])
	assert.Equal(t, "all_source_accounts_exempt", exemption["reason"])
	assert.Equal(t, "All source accounts are exempt from fees.", exemption["message"])
}

// TestCalculateFee_IsDeductibleFrom_NonExemptSourceAppliesFee verifies that when
// the source account is NOT exempt, the deductible fee is correctly deducted from
// the destination's share.
func TestCalculateFee_IsDeductibleFrom_NonExemptSourceAppliesFee(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	amount := decimal.NewFromInt(1000)
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: amount,
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: "regular_carlos",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "regular_bob",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "DeductibleFlatFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := true; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"deductible": fee}
	waived := []string{"premium_alice"}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &waived,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"regular_carlos": {Asset: "BRL", Value: amount}},
		To:   map[string]transaction.Amount{"regular_bob": {Asset: "BRL", Value: amount}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	// send.value should NOT increase for deductible fees
	assert.Equal(t, amount, feeCalc.Transaction.Send.Value)

	// Destination should receive amount minus fee deduction (1000 - 10 = 990)
	assert.True(t, decimal.NewFromInt(990).Equal(resp.To["regular_bob"].Value),
		"non-exempt source should allow deductible fee to be applied to destination, got %s", resp.To["regular_bob"].Value)

	// feeExemption metadata should NOT be set when fee is applied normally
	if feeCalc.Transaction.Metadata != nil {
		_, hasExemption := feeCalc.Transaction.Metadata["feeExemption"]
		assert.False(t, hasExemption, "feeExemption should not be set when fee is applied normally")
	}
}

// TestCalculateFee_NonDeductible_ExemptSourceSkipsFee verifies that when all source
// (From) accounts are exempt via waivedAccounts, a non-deductible fee is not applied
// and the feeExemption metadata is set on the transaction.
func TestCalculateFee_NonDeductible_ExemptSourceSkipsFee(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	amount := decimal.NewFromInt(1000)
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: amount,
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: "premium_alice",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "regular_bob",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
			},
		},
	}

	isDeductible := false
	fee := model.Fee{
		FeeLabel: "NonDeductibleFlatFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		CreditAccount:    "@fee_account",
		IsDeductibleFrom: &isDeductible,
	}

	fees := map[string]model.Fee{"flat": fee}
	waived := []string{"premium_alice"}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &waived,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"premium_alice": {Asset: "BRL", Value: amount}},
		To:   map[string]transaction.Amount{"regular_bob": {Asset: "BRL", Value: amount}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	// send.value should NOT be increased (fee was skipped)
	assert.Equal(t, amount, feeCalc.Transaction.Send.Value)

	// feeExemption metadata should be set
	assert.NotNil(t, feeCalc.Transaction.Metadata, "metadata should be set when all source accounts are exempt")
	exemption, ok := feeCalc.Transaction.Metadata["feeExemption"].(map[string]any)
	assert.True(t, ok, "feeExemption should be a map")
	assert.Equal(t, true, exemption["exempt"])
	assert.Equal(t, "all_source_accounts_exempt", exemption["reason"])
	assert.Equal(t, "All source accounts are exempt from fees.", exemption["message"])
}

// TestCalculateFee_CombinedExemption_AllAccountsExempt verifies that when
// multiple fees are present and both source and destination exemptions are triggered
// across different fees, the reason is combined to "all_accounts_exempt".
func TestCalculateFee_CombinedExemption_AllAccountsExempt(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	amount := decimal.NewFromInt(1000)
	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: amount,
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: "premium_alice",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "premium_bob",
						Amount:       &transaction.Amount{Asset: "BRL", Value: amount},
					}},
				},
			},
		},
	}

	// Fee1: non-deductible → source (premium_alice) is exempt → sets "all_source_accounts_exempt"
	fee1 := model.Fee{
		FeeLabel: "NonDeductibleFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "10",
			}},
		},
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "@fee_account1",
	}

	// Fee2: deductible → source (premium_alice) is also exempt → remains "all_source_accounts_exempt"
	// But destination (premium_bob) would also be exempt if source check didn't short-circuit.
	// Using a deductible fee where source is exempt triggers the source exemption again.
	fee2 := model.Fee{
		FeeLabel: "DeductibleFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "5",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         2,
		IsDeductibleFrom: func() *bool { b := true; return &b }(),
		CreditAccount:    "@fee_account2",
	}

	fees := map[string]model.Fee{"fee1": fee1, "fee2": fee2}
	waived := []string{"premium_alice", "premium_bob"}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &waived,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"premium_alice": {Asset: "BRL", Value: amount}},
		To:   map[string]transaction.Amount{"premium_bob": {Asset: "BRL", Value: amount}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	// No fees applied — value unchanged
	assert.Equal(t, amount, feeCalc.Transaction.Send.Value)

	// feeExemption metadata should be set
	assert.NotNil(t, feeCalc.Transaction.Metadata)
	exemption, ok := feeCalc.Transaction.Metadata["feeExemption"].(map[string]any)
	assert.True(t, ok, "feeExemption should be a map")
	assert.Equal(t, true, exemption["exempt"])
	// Both fees trigger source exemption, reason stays "all_source_accounts_exempt"
	// since both sides are covered by the same waived list
	assert.Contains(t, []string{"all_source_accounts_exempt", "all_accounts_exempt"}, exemption["reason"])
	// Message should match the reason
	if exemption["reason"] == "all_accounts_exempt" {
		assert.Equal(t, "All accounts (source and destination) are exempt from fees.", exemption["message"])
	} else {
		assert.Equal(t, "All source accounts are exempt from fees.", exemption["message"])
	}
}

// TestCalculateFee_IsDeductibleFrom_RoundingDistribution verifies proportional
// distribution of deductible fees across equal accounts with repeating decimals.
// 3 accounts with equal values (100 each) splitting a flat fee of 2:
// 100/300 = 0.3333... → fee per account ≈ 0.66 or 0.67 after rounding.
func TestCalculateFee_IsDeductibleFrom_RoundingDistribution(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	amount := decimal.NewFromInt(300)
	perAccount := decimal.NewFromInt(100)

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: amount,
				Source: transaction.Source{
					From: []transaction.FromTo{
						{Amount: &transaction.Amount{Asset: "BRL", Value: amount}},
					},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{
						{Amount: &transaction.Amount{Asset: "BRL", Value: perAccount}},
						{Amount: &transaction.Amount{Asset: "BRL", Value: perAccount}},
						{Amount: &transaction.Amount{Asset: "BRL", Value: perAccount}},
					},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "DeductibleRoundingFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "2",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := true; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"deductible_rounding": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{
			"@from_account": {Asset: "BRL", Value: amount},
		},
		To: map[string]transaction.Amount{
			"@to_account_1": {Asset: "BRL", Value: perAccount},
			"@to_account_2": {Asset: "BRL", Value: perAccount},
			"@to_account_3": {Asset: "BRL", Value: perAccount},
		},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	// Verify fee entries were created in resp.To
	feeTotal := decimal.Zero
	feeEntryCount := 0

	for key, amt := range resp.To {
		if strings.Contains(key, "fee") {
			feeTotal = feeTotal.Add(amt.Value)
			feeEntryCount++
		}
	}

	assert.Greater(t, feeEntryCount, 0, "Should have created fee entries in resp.To")
	assert.True(t, feeTotal.GreaterThan(decimal.Zero), "Fee total should be positive")
}

// TestCalculateFee_MultipleFees tests calculation with multiple fees
func TestCalculateFee_MultipleFees(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee1 := model.Fee{
		FeeLabel: "Fee1",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "50",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account1",
	}

	fee2 := model.Fee{
		FeeLabel: "Fee2",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "5",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         2,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account2",
	}

	fees := map[string]model.Fee{"fee1": fee1, "fee2": fee2}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)

	expectedValue := decimal.NewFromInt(1100)
	assert.True(t, feeCalc.Transaction.Send.Value.GreaterThanOrEqual(expectedValue))
}

// TestCalculateFee_WaivedAccountsNil tests when WaivedAccounts is nil
func TestCalculateFee_WaivedAccountsNil(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"test": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: nil,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	// Package should NOT be mutated - WaivedAccounts remains nil
	// This ensures cached packages are not modified
	assert.Nil(t, pkg.WaivedAccounts)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(decimal.NewFromInt(1100)),
		"expected Send.Value=1100, got %s", feeCalc.Transaction.Send.Value.String())
}

// TestCalculateFee_MaxBetweenTypes_InvalidValue tests error with invalid value in maxBetweenTypes
func TestCalculateFee_MaxBetweenTypes_InvalidValue(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "InvalidMaxFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: constant.FeeTypeFlat, Value: "invalid"},
				{Type: constant.FeeTypePercentage, Value: "5"},
			},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"invalid": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
}

// TestCalculateFee_MaxBetweenTypes_UnknownType tests error with unknown type in maxBetweenTypes
func TestCalculateFee_MaxBetweenTypes_UnknownType(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "UnknownTypeFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: "unknown", Value: "100"},
			},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"unknown": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
}

// TestFilterByTransactionRoute tests filter by transaction route
func TestFilterByTransactionRoute(t *testing.T) {
	route1 := "debitoted"
	route2 := "creditfrom"

	tests := []struct {
		name             string
		packages         []*pack.Package
		transactionRoute string
		expectedCount    int
	}{
		{
			name: "Filtra por rota específica",
			packages: []*pack.Package{
				{TransactionRoute: &route1},
				{TransactionRoute: &route2},
				{TransactionRoute: &route1},
			},
			transactionRoute: route1,
			expectedCount:    2,
		},
		{
			name: "Filtra por rota vazia quando TransactionRoute é nil",
			packages: []*pack.Package{
				{TransactionRoute: nil},
				{TransactionRoute: &route1},
			},
			transactionRoute: "",
			expectedCount:    1,
		},
		{
			name: "Retorna vazio quando nenhum pacote corresponde",
			packages: []*pack.Package{
				{TransactionRoute: &route1},
			},
			transactionRoute: route2,
			expectedCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByTransactionRoute(tt.packages, tt.transactionRoute)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

// TestFilterBySegmentID tests filter by segment ID
func TestFilterBySegmentID(t *testing.T) {
	segmentID1 := uuid.New()
	segmentID2 := uuid.New()

	tests := []struct {
		name          string
		packages      []*pack.Package
		segmentID     *uuid.UUID
		expectedCount int
	}{
		{
			name: "Filtra por segment ID específico",
			packages: []*pack.Package{
				{SegmentID: &segmentID1},
				{SegmentID: &segmentID2},
				{SegmentID: &segmentID1},
			},
			segmentID:     &segmentID1,
			expectedCount: 2,
		},
		{
			name: "Filtra quando segmentID é nil - retorna apenas pacotes sem segment",
			packages: []*pack.Package{
				{SegmentID: nil},
				{SegmentID: &segmentID1},
				{SegmentID: nil},
			},
			segmentID:     nil,
			expectedCount: 2,
		},
		{
			name: "Retorna vazio quando nenhum pacote corresponde",
			packages: []*pack.Package{
				{SegmentID: &segmentID1},
			},
			segmentID:     &segmentID2,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterBySegmentID(tt.packages, tt.segmentID)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

// TestFilterByAmount tests filter by amount
func TestFilterByAmount(t *testing.T) {
	tests := []struct {
		name          string
		packages      []*pack.Package
		amount        decimal.Decimal
		expectedCount int
	}{
		{
			name: "Filtra pacotes dentro do range",
			packages: []*pack.Package{
				{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
				{MinimumAmount: decimal.NewFromInt(600), MaximumAmount: decimal.NewFromInt(1000)},
				{MinimumAmount: decimal.NewFromInt(200), MaximumAmount: decimal.NewFromInt(400)},
			},
			amount:        decimal.NewFromInt(300),
			expectedCount: 2,
		},
		{
			name: "Retorna vazio quando valor está fora de todos os ranges",
			packages: []*pack.Package{
				{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
				{MinimumAmount: decimal.NewFromInt(600), MaximumAmount: decimal.NewFromInt(1000)},
			},
			amount:        decimal.NewFromInt(50),
			expectedCount: 0,
		},
		{
			name: "Inclui valor no limite mínimo",
			packages: []*pack.Package{
				{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
			},
			amount:        decimal.NewFromInt(100),
			expectedCount: 1,
		},
		{
			name: "Inclui valor no limite máximo",
			packages: []*pack.Package{
				{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(500)},
			},
			amount:        decimal.NewFromInt(500),
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByAmount(tt.packages, tt.amount)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

// TestSelectReferenceAmount tests reference amount selection
func TestSelectReferenceAmount(t *testing.T) {
	feeOriginal := model.Fee{
		ReferenceAmount: "originalAmount",
	}

	feeAfterFees := model.Fee{
		ReferenceAmount: constant.ReferenceAmountAfterFeesAmount,
	}

	currentValue := decimal.NewFromInt(1100)
	originalValue := decimal.NewFromInt(1000)

	tests := []struct {
		name           string
		fee            model.Fee
		currentValue   decimal.Decimal
		originalValue  decimal.Decimal
		expectedResult decimal.Decimal
	}{
		{
			name:           "Retorna originalValue quando ReferenceAmount é originalAmount",
			fee:            feeOriginal,
			currentValue:   currentValue,
			originalValue:  originalValue,
			expectedResult: originalValue,
		},
		{
			name:           "Retorna currentValue quando ReferenceAmount é afterFeesAmount",
			fee:            feeAfterFees,
			currentValue:   currentValue,
			originalValue:  originalValue,
			expectedResult: currentValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectReferenceAmount(tt.fee, tt.currentValue, tt.originalValue)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestIsTransactionValueBetweenMaxAndMinAmountPackage tests value verification between min and max
func TestIsTransactionValueBetweenMaxAndMinAmountPackage(t *testing.T) {
	tests := []struct {
		name     string
		pkg      pack.Package
		amount   decimal.Decimal
		expected bool
	}{
		{
			name:     "Valor dentro do range",
			pkg:      pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			amount:   decimal.NewFromInt(500),
			expected: true,
		},
		{
			name:     "Valor no limite mínimo",
			pkg:      pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			amount:   decimal.NewFromInt(100),
			expected: true,
		},
		{
			name:     "Valor no limite máximo",
			pkg:      pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			amount:   decimal.NewFromInt(1000),
			expected: true,
		},
		{
			name:     "Valor abaixo do mínimo",
			pkg:      pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			amount:   decimal.NewFromInt(50),
			expected: false,
		},
		{
			name:     "Valor acima do máximo",
			pkg:      pack.Package{MinimumAmount: decimal.NewFromInt(100), MaximumAmount: decimal.NewFromInt(1000)},
			amount:   decimal.NewFromInt(1500),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTransactionValueBetweenMaxAndMinAmountPackage(tt.pkg, tt.amount)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindPercentualOfValue tests percentage calculation
func TestFindPercentualOfValue(t *testing.T) {
	tests := []struct {
		name             string
		feeValue         decimal.Decimal
		transactionValue decimal.Decimal
		expectedValue    decimal.Decimal
	}{
		{
			name:             "10% de 1000 = 100",
			feeValue:         decimal.NewFromInt(10),
			transactionValue: decimal.NewFromInt(1000),
			expectedValue:    decimal.NewFromInt(100),
		},
		{
			name:             "5% de 2000 = 100",
			feeValue:         decimal.NewFromInt(5),
			transactionValue: decimal.NewFromInt(2000),
			expectedValue:    decimal.NewFromInt(100),
		},
		{
			name:             "25% de 400 = 100",
			feeValue:         decimal.NewFromInt(25),
			transactionValue: decimal.NewFromInt(400),
			expectedValue:    decimal.NewFromInt(100),
		},
		{
			name:             "0% de qualquer valor = 0",
			feeValue:         decimal.Zero,
			transactionValue: decimal.NewFromInt(1000),
			expectedValue:    decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findPercentualOfValue(tt.feeValue, tt.transactionValue, "BRL")
			assert.Equal(t, "BRL", result.Asset)
			assert.True(t, result.Value.Equal(tt.expectedValue), "expected %s, got %s", tt.expectedValue.String(), result.Value.String())
		})
	}
}

// TestIsAccountExempt tests exempt account verification
func TestIsAccountExempt(t *testing.T) {
	exemptAccounts := &[]string{"@account1", "@account2"}

	tests := []struct {
		name           string
		account        string
		exemptAccounts *[]string
		expected       bool
	}{
		{
			name:           "Conta está na lista de isentas",
			account:        "@account1",
			exemptAccounts: exemptAccounts,
			expected:       true,
		},
		{
			name:           "Conta não está na lista de isentas",
			account:        "@account3",
			exemptAccounts: exemptAccounts,
			expected:       false,
		},
		{
			name:           "exemptAccounts é nil",
			account:        "@account1",
			exemptAccounts: nil,
			expected:       false,
		},
		{
			name:           "exemptAccounts está vazio",
			account:        "@account1",
			exemptAccounts: &[]string{},
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAccountExempt(tt.account, tt.exemptAccounts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAccountExemptOrSegment_DecoratedKeys(t *testing.T) {
	exemptAccounts := &[]string{"@account1", "@creditAccount"}

	tests := []struct {
		name     string
		account  string
		expected bool
	}{
		{
			name:     "plain exempt alias",
			account:  "@account1",
			expected: true,
		},
		{
			name:     "fee-decorated key is canonicalized",
			account:  "@account1->fee0->routeX",
			expected: true,
		},
		{
			name:     "fee_source-decorated key is canonicalized",
			account:  "@creditAccount->fee_source0->@account1->routeY",
			expected: true,
		},
		{
			name:     "route-decorated key is canonicalized",
			account:  "@account1->routeZ",
			expected: true,
		},
		{
			name:     "non-exempt decorated key stays non-exempt",
			account:  "@account3->fee0->routeX",
			expected: false,
		},
		{
			name:     "plain non-exempt alias",
			account:  "@account3",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isAccountExemptOrSegment(tt.account, exemptAccounts, nil, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAccountExemptOrSegment_ErrorOnMissingSegmentContext(t *testing.T) {
	exemptAccounts := &[]string{"@account1"}
	segmentIDs := []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")}

	tests := []struct {
		name   string
		segCtx *SegmentContext
	}{
		{
			name:   "nil segCtx",
			segCtx: nil,
		},
		{
			name:   "segCtx with nil Resolver",
			segCtx: &SegmentContext{Resolver: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isAccountExemptOrSegment("@account1", exemptAccounts, segmentIDs, tt.segCtx)
			assert.Error(t, err)
			assert.False(t, result)
			assert.Contains(t, err.Error(), "internal configuration issue")
		})
	}
}

// TestTrimFeeSuffix tests fee suffix removal
func TestTrimFeeSuffix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove sufixo ->fee",
			input:    "@account->fee1",
			expected: "@account",
		},
		{
			name:     "Remove sufixo com múltiplos ->",
			input:    "@account->fee1->route",
			expected: "@account",
		},
		{
			name:     "Sem sufixo retorna original",
			input:    "@account",
			expected: "@account",
		},
		{
			name:     "String vazia",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimFeeSuffix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessAccount tests account processing
func TestProcessAccount(t *testing.T) {
	tests := []struct {
		name            string
		account         string
		expectedAccount string
		expectedSource  string
		hasMetadata     bool
	}{
		{
			name:            "Processa conta com fee_source",
			account:         "@credit->fee_source1->@source->route",
			expectedAccount: "@credit",
			expectedSource:  "@source",
			hasMetadata:     true,
		},
		{
			name:            "Conta sem fee_source retorna original",
			account:         "@account->route",
			expectedAccount: "@account->route",
			expectedSource:  "",
			hasMetadata:     false,
		},
		{
			name:            "Conta simples",
			account:         "@account",
			expectedAccount: "@account",
			expectedSource:  "",
			hasMetadata:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, metadata := processAccount(tt.account)
			assert.Equal(t, tt.expectedAccount, account)
			if tt.hasMetadata {
				assert.NotNil(t, metadata["source"])
				assert.Equal(t, tt.expectedSource, metadata["source"])
			} else {
				assert.Empty(t, metadata)
			}
		})
	}
}

// TestUpdatedAmountsFromFee tests fee values update
func TestUpdatedAmountsFromFee(t *testing.T) {
	tests := []struct {
		name     string
		amounts  map[string]transaction.Amount
		expected int
	}{
		{
			name: "Converte map para array FromTo",
			amounts: map[string]transaction.Amount{
				"@account1": {Asset: "BRL", Value: decimal.NewFromInt(100)},
				"@account2": {Asset: "BRL", Value: decimal.NewFromInt(200)},
			},
			expected: 2,
		},
		{
			name: "Processa conta com route",
			amounts: map[string]transaction.Amount{
				"@account->route": {Asset: "BRL", Value: decimal.NewFromInt(100)},
			},
			expected: 1,
		},
		{
			name: "Processa conta com fee_source",
			amounts: map[string]transaction.Amount{
				"@credit->fee_source1->@source->route": {Asset: "BRL", Value: decimal.NewFromInt(100)},
			},
			expected: 1,
		},
		{
			name:     "Map vazio retorna array vazio",
			amounts:  map[string]transaction.Amount{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := updatedAmountsFromFee(tt.amounts)
			assert.Len(t, result, tt.expected)

			if tt.expected > 0 {
				for _, fromTo := range result {
					assert.NotNil(t, fromTo.Amount)
					assert.NotEmpty(t, fromTo.AccountAlias)
				}
			}
		})
	}
}

// TestFindMaxAccount tests search for account with maximum value
func TestFindMaxAccount(t *testing.T) {
	tests := []struct {
		name           string
		amounts        map[string]transaction.Amount
		exemptAccounts *[]string
		expected       string
	}{
		{
			name: "Encontra conta com maior valor",
			amounts: map[string]transaction.Amount{
				"@account1": {Value: decimal.NewFromInt(100)},
				"@account2": {Value: decimal.NewFromInt(200)},
				"@account3": {Value: decimal.NewFromInt(150)},
			},
			exemptAccounts: nil,
			expected:       "@account2",
		},
		{
			name: "Ignora contas isentas",
			amounts: map[string]transaction.Amount{
				"@account1": {Value: decimal.NewFromInt(100)},
				"@account2": {Value: decimal.NewFromInt(200)},
				"@account3": {Value: decimal.NewFromInt(300)},
			},
			exemptAccounts: &[]string{"@account3"},
			expected:       "@account2",
		},
		{
			name: "Retorna string vazia quando todas as contas são isentas",
			amounts: map[string]transaction.Amount{
				"@account1": {Value: decimal.NewFromInt(100)},
			},
			exemptAccounts: &[]string{"@account1"},
			expected:       "",
		},
		{
			name:           "Retorna string vazia quando map está vazio",
			amounts:        map[string]transaction.Amount{},
			exemptAccounts: nil,
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findMaxAccount(tt.amounts, tt.exemptAccounts, nil, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCalculateFee_WithRouteFromAndRouteTo tests calculation with RouteFrom and RouteTo
func TestCalculateFee_WithRouteFromAndRouteTo(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	routeFrom := "route_from"
	routeTo := "route_to"

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "FeeWithRoutes",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
		RouteFrom:        &routeFrom,
		RouteTo:          &routeTo,
	}

	fees := map[string]model.Fee{"test": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	expectedValue := decimal.NewFromInt(1100)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedValue), "expected %s, got %s", expectedValue.String(), feeCalc.Transaction.Send.Value.String())
}

// TestCalculateFee_ProportionalFeeWithRepeatingDecimal tests proportional calculation with repeating decimal
func TestCalculateFee_ProportionalFeeWithRepeatingDecimal(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(333)},
					}, {
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(333)},
					}, {
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(334)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "ProportionalFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"test": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{
			"@from_account1": {Asset: "BRL", Value: decimal.NewFromInt(333)},
			"@from_account2": {Asset: "BRL", Value: decimal.NewFromInt(333)},
			"@from_account3": {Asset: "BRL", Value: decimal.NewFromInt(334)},
		},
		To: map[string]transaction.Amount{
			"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)},
		},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	assert.Greater(t, feeCalc.Transaction.Send.Value.IntPart(), int64(1000))
}

// TestCalculateFee_WithExemptAccounts tests calculation with exempt accounts
func TestCalculateFee_WithExemptAccounts(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}, {
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "FeeWithExempt",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"test": fee}
	exemptAccounts := []string{"@from_account1"}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &exemptAccounts,
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{
			"@from_account1": {Asset: "BRL", Value: decimal.NewFromInt(500)},
			"@from_account2": {Asset: "BRL", Value: decimal.NewFromInt(500)},
		},
		To: map[string]transaction.Amount{
			"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)},
		},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	assert.Greater(t, feeCalc.Transaction.Send.Value.IntPart(), int64(1000))
}

// TestCalculateFee_MaxBetweenTypes_FlatGreaterThanPercentage tests when flat is greater than percentage
func TestCalculateFee_MaxBetweenTypes_FlatGreaterThanPercentage(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "MaxBetweenFlat",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: constant.FeeTypeFlat, Value: "100"},
				{Type: constant.FeeTypePercentage, Value: "5"},
			},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"max": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	expectedValue := decimal.NewFromInt(1100)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedValue), "expected %s, got %s", expectedValue.String(), feeCalc.Transaction.Send.Value.String())
}

// TestCalculateFee_MaxBetweenTypes_PercentageGreaterThanFlat tests when percentage is greater than flat
func TestCalculateFee_MaxBetweenTypes_PercentageGreaterThanFlat(t *testing.T) {
	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "MaxBetweenPercentage",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: constant.FeeTypeFlat, Value: "50"},
				{Type: constant.FeeTypePercentage, Value: "10"},
			},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	fees := map[string]model.Fee{"max": fee}
	pkg := &pack.Package{
		ID:             uuid.New(),
		Fees:           fees,
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: decimal.NewFromInt(1000)}},
	}

	err := CalculateFee(logger, feeCalc, pkg, resp, "BRL", nil)
	assert.NoError(t, err)
	expectedValue := decimal.NewFromInt(1100)
	assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedValue), "expected %s, got %s", expectedValue.String(), feeCalc.Transaction.Send.Value.String())
}

// TestCalculateFee_Rounding verifies that fee values are rounded based on
// asset precision (Half Up) BEFORE applyDeductibleAndReferenceAmountRules.
func TestCalculateFee_Rounding(t *testing.T) {
	t.Parallel()

	boolFalse := false

	tests := []struct {
		name            string
		asset           string
		txValue         string
		applicationRule string
		calculations    []model.Calculation
		expectedFee     string
		description     string
	}{
		{
			name:            "percentual BRL 2% of 29.25 rounds 0.585 to 0.59",
			asset:           "BRL",
			txValue:         "29.25",
			applicationRule: constant.AppRulePercentual,
			calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "2",
			}},
			expectedFee: "0.59",
			description: "BRL has 2 decimal places; 29.25 * 0.02 = 0.585 rounds to 0.59 (Half Up)",
		},
		{
			name:            "percentual BTC 2% of 0.12345678 keeps 8 decimal places",
			asset:           "BTC",
			txValue:         "0.12345678",
			applicationRule: constant.AppRulePercentual,
			calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "2",
			}},
			expectedFee: "0.00246914",
			description: "BTC has 8 decimal places; 0.12345678 * 0.02 = 0.0024691356 rounds to 0.00246914",
		},
		{
			name:            "percentual JPY 7% of 999 rounds 69.93 to 70",
			asset:           "JPY",
			txValue:         "999",
			applicationRule: constant.AppRulePercentual,
			calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "7",
			}},
			expectedFee: "70",
			description: "JPY has 0 decimal places; 999 * 0.07 = 69.93 rounds to 70 (Half Up)",
		},
		{
			name:            "flatFee BRL fractional 0.585 rounds to 0.59",
			asset:           "BRL",
			txValue:         "100",
			applicationRule: constant.AppRuleFlatFee,
			calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "0.585",
			}},
			expectedFee: "0.59",
			description: "BRL has 2 decimal places; flat fee 0.585 rounds to 0.59 (Half Up)",
		},
		{
			name:            "maxBetweenTypes BRL selects percentual 0.585 and rounds to 0.59",
			asset:           "BRL",
			txValue:         "29.25",
			applicationRule: constant.AppRuleMaxBetweenTypes,
			calculations: []model.Calculation{
				{
					Type:  constant.FeeTypeFlat,
					Value: "0.50",
				},
				{
					Type:  constant.FeeTypePercentage,
					Value: "2",
				},
			},
			expectedFee: "0.59",
			description: "BRL has 2 decimal places; max(flat=0.50, 2% of 29.25=0.585) = 0.585 rounds to 0.59",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

			txValue, err := decimal.NewFromString(tt.txValue)
			assert.NoError(t, err, "failed to parse transaction value")

			feeCalc := &model.FeeCalculate{
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: tt.asset,
						Value: txValue,
						Source: transaction.Source{
							From: []transaction.FromTo{{
								AccountAlias: "@from_account",
								Amount:       &transaction.Amount{Asset: tt.asset, Value: txValue},
							}},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{{
								AccountAlias: "@to_account",
								Amount:       &transaction.Amount{Asset: tt.asset, Value: txValue},
							}},
						},
					},
				},
			}

			fee := model.Fee{
				FeeLabel: "RoundingFee",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: tt.applicationRule,
					Calculations:    tt.calculations,
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: &boolFalse,
				CreditAccount:    "@fee_account",
			}

			fees := map[string]model.Fee{"rounding_fee": fee}
			testPkg := &pack.Package{
				ID:             uuid.New(),
				Fees:           fees,
				WaivedAccounts: &[]string{},
			}

			resp := &transaction.Responses{
				From: map[string]transaction.Amount{
					"@from_account": {Asset: tt.asset, Value: txValue},
				},
				To: map[string]transaction.Amount{
					"@to_account": {Asset: tt.asset, Value: txValue},
				},
			}

			err = CalculateFee(logger, feeCalc, testPkg, resp, tt.asset, nil)
			assert.NoError(t, err, "CalculateFee should not return error")

			// Extract the fee amount from the response (fee entry in resp.From)
			expectedFeeValue, err := decimal.NewFromString(tt.expectedFee)
			assert.NoError(t, err, "failed to parse expected fee value")

			// Find the fee entry in resp.From (key contains "fee")
			var actualFeeValue decimal.Decimal
			feeFound := false

			for key, amount := range resp.From {
				if strings.Contains(key, "fee") {
					actualFeeValue = amount.Value
					feeFound = true

					break
				}
			}

			assert.True(t, feeFound, "fee entry not found in resp.From")

			// Assert the fee value is correctly rounded
			assert.True(t, actualFeeValue.Equal(expectedFeeValue),
				"[%s] fee value should be %s (rounded), got %s",
				tt.description, tt.expectedFee, actualFeeValue.String())

			// Assert total = original + fee (since IsDeductibleFrom=false, fee is added)
			expectedTotal := txValue.Add(expectedFeeValue)
			assert.True(t, feeCalc.Transaction.Send.Value.Equal(expectedTotal),
				"total (transaction + fee) should be %s, got %s",
				expectedTotal.String(), feeCalc.Transaction.Send.Value.String())
		})
	}
}

// TestCalculatePercentualFee_EmptyCalculations verifies that calculatePercentualFee
// returns an error when the calculations array is empty.
func TestCalculatePercentualFee_EmptyCalculations(t *testing.T) {
	t.Parallel()

	fee := model.Fee{
		FeeLabel: "EmptyPercentualFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations:    []model.Calculation{},
		},
	}

	result, err := calculatePercentualFee(fee, decimal.NewFromInt(1000), "BRL")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0023")
	assert.Equal(t, transaction.Amount{}, result)
}

// TestCalculatePercentualFee_InvalidPercentageValue verifies that calculatePercentualFee
// returns an error when the percentage value cannot be parsed as a decimal.
func TestCalculatePercentualFee_InvalidPercentageValue(t *testing.T) {
	t.Parallel()

	fee := model.Fee{
		FeeLabel: "InvalidPercentualFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypePercentage,
				Value: "not_a_number",
			}},
		},
	}

	result, err := calculatePercentualFee(fee, decimal.NewFromInt(1000), "BRL")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
	assert.Equal(t, transaction.Amount{}, result)
}

// TestCalculateFlatFee_EmptyCalculations verifies that calculateFlatFee
// returns an error when the calculations array is empty.
func TestCalculateFlatFee_EmptyCalculations(t *testing.T) {
	t.Parallel()

	fee := model.Fee{
		FeeLabel: "EmptyFlatFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations:    []model.Calculation{},
		},
	}

	result, err := calculateFlatFee(fee, "BRL")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0023")
	assert.Equal(t, transaction.Amount{}, result)
}

// TestCalculateFlatFee_InvalidDecimalValue verifies that calculateFlatFee
// returns an error when the flat fee value cannot be parsed as a decimal.
func TestCalculateFlatFee_InvalidDecimalValue(t *testing.T) {
	t.Parallel()

	fee := model.Fee{
		FeeLabel: "InvalidFlatFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations: []model.Calculation{{
				Type:  constant.FeeTypeFlat,
				Value: "abc_invalid",
			}},
		},
	}

	result, err := calculateFlatFee(fee, "BRL")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FEE-0044")
	assert.Equal(t, transaction.Amount{}, result)
}

// TestAllAccountsExempt_EmptyAccountsMap verifies that allAccountsExempt
// returns false when the accounts map is empty.
func TestAllAccountsExempt_EmptyAccountsMap(t *testing.T) {
	t.Parallel()

	emptyAccounts := map[string]transaction.Amount{}
	waived := []string{"@some_account"}

	result, err := allAccountsExempt(emptyAccounts, &waived, nil, nil)
	assert.NoError(t, err)
	assert.False(t, result, "empty accounts map should return false")
}

// TestAllAccountsExempt_ErrorPropagation_SegmentIDsWithNilSegCtx verifies that
// allAccountsExempt propagates the error from isAccountExemptOrSegment when
// segmentIDs is non-empty but segCtx is nil.
func TestAllAccountsExempt_ErrorPropagation_SegmentIDsWithNilSegCtx(t *testing.T) {
	t.Parallel()

	accounts := map[string]transaction.Amount{
		"@account1": {Asset: "BRL", Value: decimal.NewFromInt(500)},
	}
	waived := []string{"@other_account"}
	segmentIDs := []uuid.UUID{uuid.New()}

	result, err := allAccountsExempt(accounts, &waived, segmentIDs, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Segment-based waivers are configured but the resolution service is not available")
	assert.False(t, result)
}

// TestApplyFeeCorrection_ZeroDelta verifies that applyFeeCorrection does not
// modify any amounts when the delta between the expected fee and the total
// already distributed is zero (no correction needed).
func TestApplyFeeCorrection_ZeroDelta(t *testing.T) {
	t.Parallel()

	feeValue := transaction.Amount{
		Asset: "BRL",
		Value: decimal.NewFromInt(100),
	}
	// newFeeTotalPaying equals feeValue, so delta is zero
	newFeeTotalPaying := decimal.NewFromInt(100)

	updateAmount := map[string]transaction.Amount{
		"@account1":          {Asset: "BRL", Value: decimal.NewFromInt(500)},
		"@account1->fee0->r": {Asset: "BRL", Value: decimal.NewFromInt(100)},
	}
	updateAmountToStruct := map[string]transaction.Amount{
		"@fee_credit->fee_source0->@account1->r": {Asset: "BRL", Value: decimal.NewFromInt(100)},
	}

	origFrom := updateAmount["@account1->fee0->r"].Value
	origTo := updateAmountToStruct["@fee_credit->fee_source0->@account1->r"].Value

	target := feeCorrectionTarget{
		debitLegKey:  "@account1->fee0->r",
		creditLegKey: "@fee_credit->fee_source0->@account1->r",
		asset:        "BRL",
		found:        true,
	}

	applyFeeCorrection(updateAmount, updateAmountToStruct, feeValue, newFeeTotalPaying, false, target)

	// Values should remain unchanged since delta is zero
	assert.True(t, origFrom.Equal(updateAmount["@account1->fee0->r"].Value),
		"from fee entry should be unchanged when delta is zero")
	assert.True(t, origTo.Equal(updateAmountToStruct["@fee_credit->fee_source0->@account1->r"].Value),
		"to fee entry should be unchanged when delta is zero")
}

// TestApplyFeeCorrection_TargetNotFound verifies that applyFeeCorrection does
// not modify any amounts when no max-account leg was captured (target.found is
// false — e.g. every payer was exempt so no fee leg was created).
func TestApplyFeeCorrection_TargetNotFound(t *testing.T) {
	t.Parallel()

	feeValue := transaction.Amount{
		Asset: "BRL",
		Value: decimal.NewFromInt(100),
	}
	// newFeeTotalPaying differs from feeValue so delta would be non-zero
	newFeeTotalPaying := decimal.NewFromInt(99)

	updateAmount := map[string]transaction.Amount{
		"@account1": {Asset: "BRL", Value: decimal.NewFromInt(500)},
	}
	updateAmountToStruct := map[string]transaction.Amount{
		"@fee_credit->fee_source0->@account1->r": {Asset: "BRL", Value: decimal.NewFromInt(99)},
	}

	origAccount := updateAmount["@account1"].Value
	origTo := updateAmountToStruct["@fee_credit->fee_source0->@account1->r"].Value

	// found=false: no leg was captured, so nothing must change.
	applyFeeCorrection(updateAmount, updateAmountToStruct, feeValue, newFeeTotalPaying, false, feeCorrectionTarget{})

	assert.True(t, origAccount.Equal(updateAmount["@account1"].Value),
		"account value should be unchanged when no target leg was captured")
	assert.True(t, origTo.Equal(updateAmountToStruct["@fee_credit->fee_source0->@account1->r"].Value),
		"to struct value should be unchanged when no target leg was captured")
}

// TestApplyFeeCorrection_Deductible verifies the deductible-path reconciliation:
// the residual delta is added to the captured fee_source leg AND subtracted from
// the max account's reduced balance, preserving sum(legs)==sum(deductions).
func TestApplyFeeCorrection_Deductible(t *testing.T) {
	t.Parallel()

	feeValue := transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)}
	// Distributed 99.97; residual delta = 0.03 must land on the max leg + payer.
	newFeeTotalPaying := decimal.RequireFromString("99.97")

	const legKey = "@fee_credit->fee_source0->@to_a->r"

	const payerKey = "@to_a"

	updateAmount := map[string]transaction.Amount{
		legKey:   {Asset: "BRL", Value: decimal.RequireFromString("14.29")},
		payerKey: {Asset: "BRL", Value: decimal.RequireFromString("128.567")},
	}

	target := feeCorrectionTarget{
		debitLegKey: legKey,
		payerKey:    payerKey,
		asset:       "BRL",
		found:       true,
	}

	applyFeeCorrection(updateAmount, nil, feeValue, newFeeTotalPaying, true, target)

	// delta = 100 - 99.97 = 0.03 added to the leg, subtracted from the payer.
	assert.True(t, decimal.RequireFromString("14.32").Equal(updateAmount[legKey].Value),
		"fee_source leg should absorb the +0.03 residual, got %s", updateAmount[legKey].Value)
	assert.True(t, decimal.RequireFromString("128.537").Equal(updateAmount[payerKey].Value),
		"payer balance should drop by 0.03 to keep debit==credit, got %s", updateAmount[payerKey].Value)
}
