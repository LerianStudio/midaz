// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

func TestCreatePackageInput_GetTransactionRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		transactionRoute *string
		expected         string
	}{
		{
			name:             "With transaction route",
			transactionRoute: stringPtr("debitoted"),
			expected:         "debitoted",
		},
		{
			name:             "With nil transaction route",
			transactionRoute: nil,
			expected:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &CreatePackageInput{
				TransactionRoute: tt.transactionRoute,
			}
			result := cp.GetTransactionRoute()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreatePackageInput_ValidateMinAndMaxAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		minAmount string
		maxAmount string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Valid amounts",
			minAmount: "100.00",
			maxAmount: "1000.00",
			wantErr:   false,
		},
		{
			name:      "Min greater than max",
			minAmount: "1000.00",
			maxAmount: "100.00",
			wantErr:   true,
			errCode:   constant.ErrMinAmountGreaterThanMaxAmount.Error(),
		},
		{
			name:      "Invalid min amount format",
			minAmount: "invalid",
			maxAmount: "1000.00",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Invalid max amount format",
			minAmount: "100.00",
			maxAmount: "invalid",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Min amount with comma",
			minAmount: "100,00",
			maxAmount: "1000.00",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Max amount with comma",
			minAmount: "100.00",
			maxAmount: "1000,00",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &CreatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := cp.ValidateMinAndMaxAmount()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreatePackageInput_ValidateFees(t *testing.T) {
	tests := []struct {
		name      string
		fees      map[string]Fee
		minAmount string
		wantErr   bool
		errCode   string
	}{
		{
			name: "Valid fees",
			fees: map[string]Fee{
				"fee1": {
					Priority:         1,
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(true),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "10.00"},
						},
					},
				},
			},
			minAmount: "100.00",
			wantErr:   false,
		},
		{
			name: "Priority 1 without originalAmount",
			fees: map[string]Fee{
				"fee1": {
					Priority:        1,
					ReferenceAmount: "afterFeesAmount",
				},
			},
			wantErr: true,
			errCode: constant.ErrPriorityOne.Error(),
		},
		{
			name: "IsDeductibleFrom true without originalAmount",
			fees: map[string]Fee{
				"fee1": {
					IsDeductibleFrom: boolPtr(true),
					ReferenceAmount:  "afterFeesAmount",
				},
			},
			wantErr: true,
			errCode: constant.ErrIsDeductibleFrom.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &CreatePackageInput{
				Fee:       tt.fees,
				MinAmount: tt.minAmount,
			}
			err := cp.ValidateFees()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_GetMinimumAmount(t *testing.T) {
	tests := []struct {
		name      string
		minAmount *string
		expected  string
	}{
		{
			name:      "With minimum amount",
			minAmount: stringPtr("100.00"),
			expected:  "100.00",
		},
		{
			name:      "With nil minimum amount",
			minAmount: nil,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
			}
			result := up.GetMinimumAmount()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdatePackageInput_GetMaximumAmount(t *testing.T) {
	tests := []struct {
		name      string
		maxAmount *string
		expected  string
	}{
		{
			name:      "With maximum amount",
			maxAmount: stringPtr("1000.00"),
			expected:  "1000.00",
		},
		{
			name:      "With nil maximum amount",
			maxAmount: nil,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MaxAmount: tt.maxAmount,
			}
			result := up.GetMaximumAmount()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdatePackageInput_ValidateMinAndMaxAmount(t *testing.T) {
	tests := []struct {
		name      string
		minAmount *string
		maxAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Valid amounts",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("1000.00"),
			wantErr:   false,
		},
		{
			name:      "Min greater than max",
			minAmount: stringPtr("1000.00"),
			maxAmount: stringPtr("100.00"),
			wantErr:   true,
			errCode:   constant.ErrMinAmountGreaterThanMaxAmount.Error(),
		},
		{
			name:      "Invalid min amount format",
			minAmount: stringPtr("invalid"),
			maxAmount: stringPtr("1000.00"),
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Only min amount",
			minAmount: stringPtr("100.00"),
			maxAmount: nil,
			wantErr:   false,
		},
		{
			name:      "Only max amount",
			minAmount: nil,
			maxAmount: stringPtr("1000.00"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := up.ValidateMinAndMaxAmount()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_ValidateMinAndMaxAmountValue(t *testing.T) {
	tests := []struct {
		name      string
		minAmount *string
		maxAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Valid amounts",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("1000.00"),
			wantErr:   false,
		},
		{
			name:      "Max amount with comma",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("1000,00"),
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Nil amounts",
			minAmount: nil,
			maxAmount: nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := up.ValidateMinAndMaxAmountValue()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_ValidateMinAmountUpdate(t *testing.T) {
	maxAmountData := decimal.NewFromInt(1000)

	tests := []struct {
		name      string
		minAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Valid min amount",
			minAmount: stringPtr("100.00"),
			wantErr:   false,
		},
		{
			name:      "Min greater than max",
			minAmount: stringPtr("2000.00"),
			wantErr:   true,
			errCode:   constant.ErrMinAmountGreaterThanMaxAmount.Error(),
		},
		{
			name:      "Min equal to max",
			minAmount: stringPtr("1000.00"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
			}
			err := up.ValidateMinAmountUpdate(maxAmountData)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_ValidateMaxAmountUpdate(t *testing.T) {
	minAmountData := decimal.NewFromInt(100)

	tests := []struct {
		name      string
		maxAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Valid max amount",
			maxAmount: stringPtr("1000.00"),
			wantErr:   false,
		},
		{
			name:      "Max less than min",
			maxAmount: stringPtr("50.00"),
			wantErr:   true,
			errCode:   constant.ErrMaxAmountLessThanMinAmount.Error(),
		},
		{
			name:      "Max equal to min",
			maxAmount: stringPtr("100.00"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MaxAmount: tt.maxAmount,
			}
			err := up.ValidateMaxAmountUpdate(minAmountData)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFee_GetIsDeductibleFrom(t *testing.T) {
	tests := []struct {
		name             string
		isDeductibleFrom *bool
		expected         bool
	}{
		{
			name:             "True value",
			isDeductibleFrom: boolPtr(true),
			expected:         true,
		},
		{
			name:             "False value",
			isDeductibleFrom: boolPtr(false),
			expected:         false,
		},
		{
			name:             "Nil value",
			isDeductibleFrom: nil,
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				IsDeductibleFrom: tt.isDeductibleFrom,
			}
			result := f.GetIsDeductibleFrom()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_GetRouteFrom(t *testing.T) {
	tests := []struct {
		name      string
		routeFrom *string
		expected  string
	}{
		{
			name:      "With route from",
			routeFrom: stringPtr("route_from_value"),
			expected:  "route_from_value",
		},
		{
			name:      "With nil route from",
			routeFrom: nil,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				RouteFrom: tt.routeFrom,
			}
			result := f.GetRouteFrom()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_GetRouteTo(t *testing.T) {
	tests := []struct {
		name     string
		routeTo  *string
		expected string
	}{
		{
			name:     "With route to",
			routeTo:  stringPtr("route_to_value"),
			expected: "route_to_value",
		},
		{
			name:     "With nil route to",
			routeTo:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				RouteTo: tt.routeTo,
			}
			result := f.GetRouteTo()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_ValidateIfFeeIsNil(t *testing.T) {
	tests := []struct {
		name     string
		fee      Fee
		expected bool
	}{
		{
			name:     "Nil fee",
			fee:      Fee{},
			expected: true,
		},
		{
			name: "Fee with FeeLabel",
			fee: Fee{
				FeeLabel: "Test Fee",
			},
			expected: false,
		},
		{
			name: "Fee with CalculationModel",
			fee: Fee{
				CalculationModel: &CalculationModel{},
			},
			expected: false,
		},
		{
			name: "Fee with ReferenceAmount",
			fee: Fee{
				ReferenceAmount: "originalAmount",
			},
			expected: false,
		},
		{
			name: "Fee with Priority",
			fee: Fee{
				Priority: 1,
			},
			expected: false,
		},
		{
			name: "Fee with IsDeductibleFrom",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
			},
			expected: false,
		},
		{
			name: "Fee with CreditAccount",
			fee: Fee{
				CreditAccount: "account123",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fee.ValidateIfFeeIsNil()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_validateReferenceAmountIsInvalid(t *testing.T) {
	tests := []struct {
		name            string
		referenceAmount string
		expected        bool
	}{
		{
			name:            "Valid originalAmount",
			referenceAmount: "originalAmount",
			expected:        false,
		},
		{
			name:            "Valid afterFeesAmount",
			referenceAmount: "afterFeesAmount",
			expected:        false,
		},
		{
			name:            "Invalid value",
			referenceAmount: "invalid",
			expected:        true,
		},
		{
			name:            "Empty string",
			referenceAmount: "",
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				ReferenceAmount: tt.referenceAmount,
			}
			result := f.validateReferenceAmountIsInvalid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_validateAppRuleIsInvalid(t *testing.T) {
	tests := []struct {
		name            string
		applicationRule string
		expected        bool
	}{
		{
			name:            "Valid maxBetweenTypes",
			applicationRule: "maxBetweenTypes",
			expected:        false,
		},
		{
			name:            "Valid flatFee",
			applicationRule: "flatFee",
			expected:        false,
		},
		{
			name:            "Valid percentual",
			applicationRule: "percentual",
			expected:        false,
		},
		{
			name:            "Invalid value",
			applicationRule: "invalid",
			expected:        true,
		},
		{
			name:            "Empty string",
			applicationRule: "",
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: tt.applicationRule,
				},
			}
			result := f.validateAppRuleIsInvalid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_formatCalculationFieldName(t *testing.T) {
	calc := Calculation{
		Type:  "percentage",
		Value: "10.50",
	}

	f := &Fee{}
	result := f.formatCalculationFieldName(calc)

	assert.Equal(t, "percentage", result["type"])
	assert.Equal(t, "10.50", result["value"])
}

func TestAmountData_GetTransactionRoute(t *testing.T) {
	tests := []struct {
		name             string
		transactionRoute *string
		expected         string
	}{
		{
			name:             "With transaction route",
			transactionRoute: stringPtr("debitoted"),
			expected:         "debitoted",
		},
		{
			name:             "With nil transaction route",
			transactionRoute: nil,
			expected:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AmountData{
				TransactionRoute: tt.transactionRoute,
			}
			result := a.GetTransactionRoute()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFee_updateFeeLabel(t *testing.T) {
	tests := []struct {
		name     string
		feeLabel string
		expected bool
	}{
		{
			name:     "With fee label",
			feeLabel: "Test Fee",
			expected: true,
		},
		{
			name:     "Empty fee label",
			feeLabel: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				FeeLabel: tt.feeLabel,
			}
			upFields := bson.M{}
			result := f.updateFeeLabel("fee1", upFields)

			assert.Equal(t, tt.expected, result)
			if tt.expected {
				assert.Equal(t, tt.feeLabel, upFields["fees.fee1.fee_label"])
			}
		})
	}
}

func TestFee_updatePriority(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		expected bool
	}{
		{
			name:     "With priority",
			priority: 1,
			expected: true,
		},
		{
			name:     "Zero priority",
			priority: 0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				Priority: tt.priority,
			}
			upFields := bson.M{}
			result := f.updatePriority("fee1", upFields)

			assert.Equal(t, tt.expected, result)
			if tt.expected {
				assert.Equal(t, tt.priority, upFields["fees.fee1.priority"])
			}
		})
	}
}

func TestFee_updateRouteFrom(t *testing.T) {
	tests := []struct {
		name      string
		routeFrom *string
		expected  bool
	}{
		{
			name:      "With route from",
			routeFrom: stringPtr("route_from"),
			expected:  true,
		},
		{
			name:      "Nil route from",
			routeFrom: nil,
			expected:  false,
		},
		{
			name:      "Empty route from",
			routeFrom: stringPtr(""),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				RouteFrom: tt.routeFrom,
			}
			upFields := bson.M{}
			result := f.updateRouteFrom("fee1", upFields)

			assert.Equal(t, tt.expected, result)
			if tt.expected {
				if routeFromVal, ok := upFields["fees.fee1.route_from"].(*string); ok {
					assert.Equal(t, *tt.routeFrom, *routeFromVal)
				} else {
					assert.Equal(t, *tt.routeFrom, upFields["fees.fee1.route_from"])
				}
			}
		})
	}
}

func TestFee_updateRouteTo(t *testing.T) {
	tests := []struct {
		name     string
		routeTo  *string
		expected bool
	}{
		{
			name:     "With route to",
			routeTo:  stringPtr("route_to"),
			expected: true,
		},
		{
			name:     "Nil route to",
			routeTo:  nil,
			expected: false,
		},
		{
			name:     "Empty route to",
			routeTo:  stringPtr(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				RouteTo: tt.routeTo,
			}
			upFields := bson.M{}
			result := f.updateRouteTo("fee1", upFields)

			assert.Equal(t, tt.expected, result)
			if tt.expected {
				if routeToVal, ok := upFields["fees.fee1.route_to"].(*string); ok {
					assert.Equal(t, *tt.routeTo, *routeToVal)
				} else {
					assert.Equal(t, *tt.routeTo, upFields["fees.fee1.route_to"])
				}
			}
		})
	}
}

func TestFee_hasNoCalculationModelUpdates(t *testing.T) {
	tests := []struct {
		name             string
		calculationModel *CalculationModel
		expected         bool
	}{
		{
			name: "No updates - empty rule and calculations",
			calculationModel: &CalculationModel{
				ApplicationRule: "",
				Calculations:    []Calculation{},
			},
			expected: true,
		},
		{
			name: "Has updates - with application rule",
			calculationModel: &CalculationModel{
				ApplicationRule: "flatFee",
				Calculations:    []Calculation{},
			},
			expected: false,
		},
		{
			name: "Has updates - with calculations",
			calculationModel: &CalculationModel{
				ApplicationRule: "",
				Calculations:    []Calculation{{Type: "flat", Value: "10.00"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				CalculationModel: tt.calculationModel,
			}
			result := f.hasNoCalculationModelUpdates()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdatePackageInput_ValidateFees(t *testing.T) {
	tests := []struct {
		name      string
		fees      map[string]Fee
		minAmount string
		wantErr   bool
		errCode   string
	}{
		{
			name: "Valid fees - nil fee (skipped)",
			fees: map[string]Fee{
				"fee1": {}, // Nil fee should be skipped
			},
			minAmount: "100.00",
			wantErr:   false,
		},
		{
			name: "Valid fees with calculation model",
			fees: map[string]Fee{
				"fee1": {
					Priority:         1,
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(true),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "10.00"},
						},
					},
				},
			},
			minAmount: "100.00",
			wantErr:   false,
		},
		{
			name: "Priority 1 without originalAmount",
			fees: map[string]Fee{
				"fee1": {
					Priority:        1,
					ReferenceAmount: "afterFeesAmount",
					FeeLabel:        "Test Fee", // Not nil
				},
			},
			wantErr: true,
			errCode: constant.ErrPriorityOne.Error(),
		},
		{
			name: "IsDeductibleFrom true without originalAmount",
			fees: map[string]Fee{
				"fee1": {
					IsDeductibleFrom: boolPtr(true),
					ReferenceAmount:  "afterFeesAmount",
					FeeLabel:         "Test Fee", // Not nil
				},
			},
			wantErr: true,
			errCode: constant.ErrIsDeductibleFrom.Error(),
		},
		{
			name: "Priority 0 with afterFeesAmount (should pass)",
			fees: map[string]Fee{
				"fee1": {
					Priority:        0,
					ReferenceAmount: "afterFeesAmount",
					FeeLabel:        "Test Fee",
				},
			},
			wantErr: false,
		},
		{
			name: "ReferenceAmount empty with IsDeductibleFrom nil (should pass)",
			fees: map[string]Fee{
				"fee1": {
					ReferenceAmount:  "",
					IsDeductibleFrom: nil,
					FeeLabel:         "Test Fee",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				Fee:       tt.fees,
				MinAmount: stringPtr(tt.minAmount),
			}
			err := up.ValidateFees()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreatePackageInput_ValidateFees_MoreCases(t *testing.T) {
	tests := []struct {
		name      string
		fees      map[string]Fee
		minAmount string
		wantErr   bool
		errCode   string
	}{
		{
			name: "Multiple fees - all valid",
			fees: map[string]Fee{
				"fee1": {
					Priority:         1,
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(true),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "10.00"},
						},
					},
				},
				"fee2": {
					Priority:         2,
					ReferenceAmount:  "afterFeesAmount",
					IsDeductibleFrom: boolPtr(false),
					CalculationModel: &CalculationModel{
						ApplicationRule: Percentual,
						Calculations: []Calculation{
							{Type: Percentage, Value: "5.00"},
						},
					},
				},
			},
			minAmount: "100.00",
			wantErr:   false,
		},
		{
			name: "Fee with nil CalculationModel",
			fees: map[string]Fee{
				"fee1": {
					Priority:         1,
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(true),
					CalculationModel: nil,
				},
			},
			minAmount: "100.00",
			wantErr:   true,
			errCode:   constant.ErrCalculationRequired.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &CreatePackageInput{
				Fee:       tt.fees,
				MinAmount: tt.minAmount,
			}
			err := cp.ValidateFees()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculationCountForRule(t *testing.T) {
	tests := []struct {
		name            string
		applicationRule string
		calculations    []Calculation
		wantErr         bool
		errCode         string
	}{
		{
			name:            "FlatFee with 1 calculation - valid",
			applicationRule: FlatFee,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: false,
		},
		{
			name:            "FlatFee with 2 calculations - invalid",
			applicationRule: FlatFee,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
				{Type: Flat, Value: "20.00"},
			},
			wantErr: true,
			errCode: constant.ErrAppRuleFlatFeeAndPercentual.Error(),
		},
		{
			name:            "FlatFee with 0 calculations - invalid",
			applicationRule: FlatFee,
			calculations:    []Calculation{},
			wantErr:         true,
			errCode:         constant.ErrAppRuleFlatFeeAndPercentual.Error(),
		},
		{
			name:            "Percentual with 1 calculation - valid",
			applicationRule: Percentual,
			calculations: []Calculation{
				{Type: Percentage, Value: "10.00"},
			},
			wantErr: false,
		},
		{
			name:            "Percentual with 2 calculations - invalid",
			applicationRule: Percentual,
			calculations: []Calculation{
				{Type: Percentage, Value: "10.00"},
				{Type: Percentage, Value: "20.00"},
			},
			wantErr: true,
			errCode: constant.ErrAppRuleFlatFeeAndPercentual.Error(),
		},
		{
			name:            "MaxBetween with 2 calculations - valid",
			applicationRule: MaxBetween,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
				{Type: Percentage, Value: "5.00"},
			},
			wantErr: false,
		},
		{
			name:            "MaxBetween with 1 calculation - invalid",
			applicationRule: MaxBetween,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: true,
			errCode: constant.ErrAppRuleMaxBetweenTypes.Error(),
		},
		{
			name:            "MaxBetween with 0 calculations - invalid",
			applicationRule: MaxBetween,
			calculations:    []Calculation{},
			wantErr:         true,
			errCode:         constant.ErrAppRuleMaxBetweenTypes.Error(),
		},
		{
			name:            "Unknown rule - should pass (no validation)",
			applicationRule: "unknown",
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &CalculationModel{
				ApplicationRule: tt.applicationRule,
				Calculations:    tt.calculations,
			}
			err := validateCalculationCountForRule(model, "fee1")

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculationRuleAndTypes(t *testing.T) {
	tests := []struct {
		name            string
		applicationRule string
		calculations    []Calculation
		wantErr         bool
		errCode         string
	}{
		{
			name:            "FlatFee with flat type - valid",
			applicationRule: FlatFee,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: false,
		},
		{
			name:            "FlatFee with percentage type - invalid",
			applicationRule: FlatFee,
			calculations: []Calculation{
				{Type: Percentage, Value: "10.00"},
			},
			wantErr: true,
			errCode: constant.ErrCalculationTypeFlatFee.Error(),
		},
		{
			name:            "Percentual with percentage type - valid",
			applicationRule: Percentual,
			calculations: []Calculation{
				{Type: Percentage, Value: "10.00"},
			},
			wantErr: false,
		},
		{
			name:            "Percentual with flat type - invalid",
			applicationRule: Percentual,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: true,
			errCode: constant.ErrCalculationTypePercentual.Error(),
		},
		{
			name:            "MaxBetween - should pass (no validation)",
			applicationRule: MaxBetween,
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
				{Type: Percentage, Value: "5.00"},
			},
			wantErr: false,
		},
		{
			name:            "Unknown rule - should pass",
			applicationRule: "unknown",
			calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &CalculationModel{
				ApplicationRule: tt.applicationRule,
				Calculations:    tt.calculations,
			}
			err := validateCalculationRuleAndTypes(model, "fee1")

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculationValues(t *testing.T) {
	tests := []struct {
		name         string
		model        *CalculationModel
		minAmount    string
		isDeductible bool
		wantErr      bool
		errCode      string
	}{
		{
			name: "Valid calculation - not deductible",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "1000.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      false,
		},
		{
			name: "Valid percentage - deductible, under 100%",
			model: &CalculationModel{
				ApplicationRule: Percentual,
				Calculations: []Calculation{
					{Type: Percentage, Value: "50.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      false,
		},
		{
			name: "Invalid percentage - deductible, over 100%",
			model: &CalculationModel{
				ApplicationRule: Percentual,
				Calculations: []Calculation{
					{Type: Percentage, Value: "150.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      true,
			errCode:      constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name: "Valid flat - deductible, under min amount",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "50.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      false,
		},
		{
			name: "Invalid flat - deductible, over min amount",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "150.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      true,
			errCode:      constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name: "Invalid decimal value",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "invalid"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrConvertToDecimal.Error(),
		},
		{
			name: "Multiple calculations - all valid",
			model: &CalculationModel{
				ApplicationRule: MaxBetween,
				Calculations: []Calculation{
					{Type: Flat, Value: "50.00"},
					{Type: Percentage, Value: "5.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      false,
		},
		{
			name: "Multiple calculations - one invalid",
			model: &CalculationModel{
				ApplicationRule: MaxBetween,
				Calculations: []Calculation{
					{Type: Flat, Value: "50.00"},
					{Type: Percentage, Value: "150.00"}, // Over 100%
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      true,
			errCode:      constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name: "Empty minAmount - should skip deductible validation",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "1000.00"},
				},
			},
			minAmount:    "",
			isDeductible: true,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCalculationValues(tt.model, tt.minAmount, "fee1", tt.isDeductible)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculationModel(t *testing.T) {
	tests := []struct {
		name         string
		model        *CalculationModel
		minAmount    string
		isDeductible bool
		wantErr      bool
		errCode      string
	}{
		{
			name:         "Nil model",
			model:        nil,
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrCalculationRequired.Error(),
		},
		{
			name: "Valid model - FlatFee",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "10.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      false,
		},
		{
			name: "Valid model - Percentual",
			model: &CalculationModel{
				ApplicationRule: Percentual,
				Calculations: []Calculation{
					{Type: Percentage, Value: "5.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      false,
		},
		{
			name: "Valid model - MaxBetween",
			model: &CalculationModel{
				ApplicationRule: MaxBetween,
				Calculations: []Calculation{
					{Type: Flat, Value: "10.00"},
					{Type: Percentage, Value: "5.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      false,
		},
		{
			name: "Invalid - FlatFee with wrong count",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "10.00"},
					{Type: Flat, Value: "20.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrAppRuleFlatFeeAndPercentual.Error(),
		},
		{
			name: "Invalid - FlatFee with wrong type",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Percentage, Value: "10.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrCalculationTypeFlatFee.Error(),
		},
		{
			name: "Invalid - Percentual with wrong type",
			model: &CalculationModel{
				ApplicationRule: Percentual,
				Calculations: []Calculation{
					{Type: Flat, Value: "10.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrCalculationTypePercentual.Error(),
		},
		{
			name: "Invalid - MaxBetween with 1 calculation",
			model: &CalculationModel{
				ApplicationRule: MaxBetween,
				Calculations: []Calculation{
					{Type: Flat, Value: "10.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: false,
			wantErr:      true,
			errCode:      constant.ErrAppRuleMaxBetweenTypes.Error(),
		},
		{
			name: "Invalid - deductible percentage over 100%",
			model: &CalculationModel{
				ApplicationRule: Percentual,
				Calculations: []Calculation{
					{Type: Percentage, Value: "150.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      true,
			errCode:      constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name: "Invalid - deductible flat over min amount",
			model: &CalculationModel{
				ApplicationRule: FlatFee,
				Calculations: []Calculation{
					{Type: Flat, Value: "150.00"},
				},
			},
			minAmount:    "100.00",
			isDeductible: true,
			wantErr:      true,
			errCode:      constant.ErrCalculationValueFlatFee.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCalculationModel(tt.model, tt.minAmount, "fee1", tt.isDeductible)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFee_ValidateNewFee(t *testing.T) {
	tests := []struct {
		name      string
		fee       Fee
		minAmount decimal.Decimal
		wantErr   bool
		errCode   string
	}{
		{
			name: "Valid new fee",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   false,
		},
		{
			name: "Missing FeeLabel",
			fee: Fee{
				FeeLabel:         "",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Missing CalculationModel ApplicationRule",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: "", // Empty rule
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Missing Calculations",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations:    []Calculation{}, // Empty calculations
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Missing ReferenceAmount",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  "",
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Priority 0",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         0,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Missing CreditAccount",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Nil IsDeductibleFrom",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: nil,
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name: "Invalid calculation - empty type",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: "", Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrCalculationFieldOfFeeRequired.Error(),
		},
		{
			name: "Invalid calculation - zero value",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "0"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrCalculationFieldOfFeeRequired.Error(),
		},
		{
			name: "Invalid application rule - MaxBetween with 1 calculation",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: MaxBetween,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrAppRuleMaxBetweenTypes.Error(),
		},
		{
			name: "Invalid deductible percentage over 100%",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: Percentual,
					Calculations: []Calculation{
						{Type: Percentage, Value: "150.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrDeductibleCalculationValuePercentage.Error(),
		},
		{
			name: "Invalid deductible flat over min amount",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "150.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   true,
			errCode:   constant.ErrDeductibleCalculationValueFlatFee.Error(),
		},
		{
			name: "Valid deductible percentage exactly 100%",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: Percentual,
					Calculations: []Calculation{
						{Type: Percentage, Value: "100.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   false,
		},
		{
			name: "Valid deductible flat exactly min amount",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(true),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "100.00"},
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   false,
		},
		{
			name: "Valid non-deductible fee - no validation",
			fee: Fee{
				FeeLabel:         "Test Fee",
				ReferenceAmount:  OriginalAmount,
				Priority:         1,
				CreditAccount:    "credit_account",
				IsDeductibleFrom: boolPtr(false),
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "1000.00"}, // Over min, but not deductible
					},
				},
			},
			minAmount: decimal.NewFromInt(100),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fee.ValidateNewFee("fee1", tt.minAmount)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFee_SetAndValidateHasFieldsToUpdate(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	minAmount := decimal.NewFromInt(100)
	feeKey := "fee1"

	existingFee := Fee{
		FeeLabel:         "Existing Fee",
		ReferenceAmount:  OriginalAmount,
		Priority:         1,
		CreditAccount:    "existing_account",
		IsDeductibleFrom: boolPtr(true),
		CalculationModel: &CalculationModel{
			ApplicationRule: FlatFee,
			Calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
		},
	}

	existingFees := map[string]Fee{
		feeKey: existingFee,
	}

	tests := []struct {
		name                 string
		fee                  Fee
		existingFees         map[string]Fee
		updateDeductibleFrom *bool
		mockMidazSetup       func(*pkg.MockMidazResolver)
		wantErr              bool
		errCode              string
		expectUpdate         bool
	}{
		{
			name:         "Update all fields",
			existingFees: existingFees,
			fee: Fee{
				FeeLabel:         "Updated Fee",
				ReferenceAmount:  "afterFeesAmount",
				Priority:         2,
				CreditAccount:    "new_account",
				RouteFrom:        stringPtr("route_from"),
				RouteTo:          stringPtr("route_to"),
				IsDeductibleFrom: boolPtr(false),
				CalculationModel: &CalculationModel{
					ApplicationRule: Percentual,
					Calculations: []Calculation{
						{Type: Percentage, Value: "5.00"},
					},
				},
			},
			updateDeductibleFrom: boolPtr(false),
			mockMidazSetup: func(m *pkg.MockMidazResolver) {
				m.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "new_account").
					Return(nil)
			},
			wantErr:      false,
			expectUpdate: true,
		},
		{
			name:         "Update only FeeLabel",
			existingFees: existingFees,
			fee: Fee{
				FeeLabel: "Updated Label",
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
			expectUpdate:         true,
		},
		{
			name:         "Update only Priority",
			existingFees: existingFees,
			fee: Fee{
				Priority: 3,
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
			expectUpdate:         true,
		},
		{
			name:         "Update ReferenceAmount - invalid value",
			existingFees: existingFees,
			fee: Fee{
				ReferenceAmount: "invalid",
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrReferenceAmountInvalid.Error(),
			expectUpdate:         false,
		},
		{
			name:         "Update ReferenceAmount - afterFeesAmount with existing deductible true",
			existingFees: existingFees,
			fee: Fee{
				ReferenceAmount:  "afterFeesAmount",
				IsDeductibleFrom: nil, // Not updating
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrIsDeductibleFrom.Error(),
			expectUpdate:         false,
		},
		{
			name: "Update IsDeductibleFrom - invalid reference amount (existing afterFeesAmount)",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
				ReferenceAmount:  "", // Empty, will check existing
			},
			existingFees: map[string]Fee{
				feeKey: {
					ReferenceAmount:  "afterFeesAmount",
					IsDeductibleFrom: boolPtr(false),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "10.00"},
						},
					},
				},
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrIsDeductibleFrom.Error(),
			expectUpdate:         false,
		},
		{
			name:         "Update CreditAccount - Midaz error",
			existingFees: existingFees,
			fee: Fee{
				CreditAccount: "invalid_account",
			},
			updateDeductibleFrom: nil,
			mockMidazSetup: func(m *pkg.MockMidazResolver) {
				m.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "invalid_account").
					Return(pkg.ValidateBusinessError(constant.ErrFindAccountOnMidaz, "", "invalid_account"))
			},
			wantErr:      true,
			errCode:      constant.ErrFindAccountOnMidaz.Error(),
			expectUpdate: false,
		},
		{
			name:         "Update CalculationModel - invalid app rule",
			existingFees: existingFees,
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: "invalid",
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
					},
				},
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrAppRuleInvalid.Error(),
			expectUpdate:         false,
		},
		{
			name:         "Update CalculationModel - invalid calculation type",
			existingFees: existingFees,
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: "invalid", Value: "10.00"},
					},
				},
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrCalculationTypeInvalid.Error(),
			expectUpdate:         false,
		},
		{
			name:                 "No updates - all fields empty/nil",
			fee:                  Fee{},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
			expectUpdate:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMidaz := pkg.NewMockMidazResolver(ctrl)
			tt.mockMidazSetup(mockMidaz)

			upFields := bson.M{}
			testExistingFees := tt.existingFees
			if testExistingFees == nil {
				testExistingFees = existingFees
			}
			hasUpdate, err := tt.fee.SetAndValidateHasFieldsToUpdate(
				ctx,
				tt.updateDeductibleFrom,
				minAmount,
				testExistingFees,
				feeKey,
				orgID,
				ledgerID,
				upFields,
				mockMidaz,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if httpErr, ok := err.(*pkg.HTTPError); ok {
						assert.Contains(t, httpErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectUpdate, hasUpdate)
			}
		})
	}
}

func TestFee_SetAndValidateHasFieldsToUpdate_CalculationModelCases(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	minAmount := decimal.NewFromInt(100)
	feeKey := "fee1"

	existingFee := Fee{
		FeeLabel:         "Existing Fee",
		ReferenceAmount:  OriginalAmount,
		Priority:         1,
		CreditAccount:    "existing_account",
		IsDeductibleFrom: boolPtr(true),
		CalculationModel: &CalculationModel{
			ApplicationRule: FlatFee,
			Calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
		},
	}

	existingFees := map[string]Fee{
		feeKey: existingFee,
	}

	tests := []struct {
		name                 string
		fee                  Fee
		updateDeductibleFrom *bool
		mockMidazSetup       func(*pkg.MockMidazResolver)
		wantErr              bool
		errCode              string
	}{
		{
			name: "Update ApplicationRule only",
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: Percentual,
					Calculations:    []Calculation{},
				},
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
		},
		{
			name: "Update Calculations only",
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: "",
					Calculations: []Calculation{
						{Type: Percentage, Value: "5.00"},
					},
				},
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
		},
		{
			name: "Update both ApplicationRule and Calculations",
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: MaxBetween,
					Calculations: []Calculation{
						{Type: Flat, Value: "10.00"},
						{Type: Percentage, Value: "5.00"},
					},
				},
			},
			updateDeductibleFrom: nil,
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
		},
		{
			name: "Update CalculationModel - deductible validation fails",
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: Percentual,
					Calculations: []Calculation{
						{Type: Percentage, Value: "150.00"}, // Over 100%
					},
				},
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrDeductibleCalculationValuePercentage.Error(),
		},
		{
			name: "Update CalculationModel - existing deductible, validation fails",
			fee: Fee{
				CalculationModel: &CalculationModel{
					ApplicationRule: FlatFee,
					Calculations: []Calculation{
						{Type: Flat, Value: "150.00"}, // Over min
					},
				},
			},
			updateDeductibleFrom: nil, // Existing is deductible
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrDeductibleCalculationValueFlatFee.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMidaz := pkg.NewMockMidazResolver(ctrl)
			tt.mockMidazSetup(mockMidaz)

			upFields := bson.M{}
			_, err := tt.fee.SetAndValidateHasFieldsToUpdate(
				ctx,
				tt.updateDeductibleFrom,
				minAmount,
				existingFees,
				feeKey,
				orgID,
				ledgerID,
				upFields,
				mockMidaz,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFee_SetAndValidateHasFieldsToUpdate_IsDeductibleFromCases(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	minAmount := decimal.NewFromInt(100)
	feeKey := "fee1"

	existingFeeWithAfterFees := Fee{
		FeeLabel:         "Existing Fee",
		ReferenceAmount:  "afterFeesAmount",
		Priority:         1,
		CreditAccount:    "existing_account",
		IsDeductibleFrom: boolPtr(false),
		CalculationModel: &CalculationModel{
			ApplicationRule: FlatFee,
			Calculations: []Calculation{
				{Type: Flat, Value: "10.00"},
			},
		},
	}

	existingFeeWithHighPercentage := Fee{
		FeeLabel:         "Existing Fee",
		ReferenceAmount:  OriginalAmount,
		Priority:         1,
		CreditAccount:    "existing_account",
		IsDeductibleFrom: boolPtr(false),
		CalculationModel: &CalculationModel{
			ApplicationRule: Percentual,
			Calculations: []Calculation{
				{Type: Percentage, Value: "150.00"}, // Over 100%
			},
		},
	}

	existingFeeWithHighFlat := Fee{
		FeeLabel:         "Existing Fee",
		ReferenceAmount:  OriginalAmount,
		Priority:         1,
		CreditAccount:    "existing_account",
		IsDeductibleFrom: boolPtr(false),
		CalculationModel: &CalculationModel{
			ApplicationRule: FlatFee,
			Calculations: []Calculation{
				{Type: Flat, Value: "150.00"}, // Over min
			},
		},
	}

	tests := []struct {
		name                 string
		fee                  Fee
		existingFees         map[string]Fee
		updateDeductibleFrom *bool
		mockMidazSetup       func(*pkg.MockMidazResolver)
		wantErr              bool
		errCode              string
	}{
		{
			name: "Update IsDeductibleFrom to true - valid",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
			},
			existingFees: map[string]Fee{
				feeKey: {
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(false),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "10.00"},
						},
					},
				},
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
		},
		{
			name: "Update IsDeductibleFrom to true - invalid reference amount",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
				ReferenceAmount:  "", // Empty, will check existing
			},
			existingFees: map[string]Fee{
				feeKey: existingFeeWithAfterFees,
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrIsDeductibleFrom.Error(),
		},
		{
			name: "Update IsDeductibleFrom to true - invalid percentage calculation",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
			},
			existingFees: map[string]Fee{
				feeKey: existingFeeWithHighPercentage,
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrDeductibleCalculationValuePercentage.Error(),
		},
		{
			name: "Update IsDeductibleFrom to true - invalid flat calculation",
			fee: Fee{
				IsDeductibleFrom: boolPtr(true),
			},
			existingFees: map[string]Fee{
				feeKey: existingFeeWithHighFlat,
			},
			updateDeductibleFrom: boolPtr(true),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              true,
			errCode:              constant.ErrDeductibleCalculationValueFlatFee.Error(),
		},
		{
			name: "Update IsDeductibleFrom to false - should pass",
			fee: Fee{
				IsDeductibleFrom: boolPtr(false),
			},
			existingFees: map[string]Fee{
				feeKey: {
					ReferenceAmount:  OriginalAmount,
					IsDeductibleFrom: boolPtr(true),
					CalculationModel: &CalculationModel{
						ApplicationRule: FlatFee,
						Calculations: []Calculation{
							{Type: Flat, Value: "150.00"}, // Over min, but will be false
						},
					},
				},
			},
			updateDeductibleFrom: boolPtr(false),
			mockMidazSetup:       func(m *pkg.MockMidazResolver) {},
			wantErr:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMidaz := pkg.NewMockMidazResolver(ctrl)
			tt.mockMidazSetup(mockMidaz)

			upFields := bson.M{}
			_, err := tt.fee.SetAndValidateHasFieldsToUpdate(
				ctx,
				tt.updateDeductibleFrom,
				minAmount,
				tt.existingFees,
				feeKey,
				orgID,
				ledgerID,
				upFields,
				mockMidaz,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreatePackageInput_ValidateMinAndMaxAmount_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		minAmount string
		maxAmount string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Min equal to max",
			minAmount: "100.00",
			maxAmount: "100.00",
			wantErr:   false,
		},
		{
			name:      "Min with comma in middle",
			minAmount: "1,000.00",
			maxAmount: "2000.00",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Max with comma in middle",
			minAmount: "100.00",
			maxAmount: "2,000.00",
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Very large numbers",
			minAmount: "999999999.99",
			maxAmount: "9999999999.99",
			wantErr:   false,
		},
		{
			name:      "Very small numbers",
			minAmount: "0.01",
			maxAmount: "0.02",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &CreatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := cp.ValidateMinAndMaxAmount()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_ValidateMinAndMaxAmount_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		minAmount *string
		maxAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Min equal to max",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("100.00"),
			wantErr:   false,
		},
		{
			name:      "Both nil",
			minAmount: nil,
			maxAmount: nil,
			wantErr:   false,
		},
		{
			name:      "Invalid max format with nil min",
			minAmount: nil,
			maxAmount: stringPtr("invalid"),
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := up.ValidateMinAndMaxAmount()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageInput_ValidateMinAndMaxAmountValue_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		minAmount *string
		maxAmount *string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "Max amount with comma",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("1000,00"),
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
		{
			name:      "Both max with comma (function only checks max)",
			minAmount: stringPtr("100.00"),
			maxAmount: stringPtr("1000,00"),
			wantErr:   true,
			errCode:   constant.ErrConvertToDecimal.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: tt.minAmount,
				MaxAmount: tt.maxAmount,
			}
			err := up.ValidateMinAndMaxAmountValue()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					} else if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
