// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestValidateMinAmountUpdate_InvalidDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		min     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Error - invalid min amount with comma",
			min:     "100,50",
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Error - invalid min amount with letters",
			min:     "abc",
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Success - valid min amount below max",
			min:     "100.50",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MinAmount: &tt.min,
			}
			err := up.ValidateMinAmountUpdate(decimal.NewFromInt(1000))

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q", tt.min)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMaxAmountUpdate_InvalidDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		max     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Error - invalid max amount with comma",
			max:     "1000,50",
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Error - invalid max amount with letters",
			max:     "xyz",
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Success - valid max amount above min",
			max:     "1000.50",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := &UpdatePackageInput{
				MaxAmount: &tt.max,
			}
			err := up.ValidateMaxAmountUpdate(decimal.NewFromInt(100))

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q", tt.max)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDeductibleCalculation_InvalidDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		calc    Calculation
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Error - invalid decimal value with comma",
			calc:    Calculation{Type: "percentage", Value: "10,50"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Error - invalid decimal value with letters",
			calc:    Calculation{Type: "flat", Value: "abc"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Success - valid percentage value",
			calc:    Calculation{Type: "percentage", Value: "50"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				IsDeductibleFrom: func() *bool { b := true; return &b }(),
			}
			err := f.validateDeductibleCalculation(tt.calc, decimal.NewFromInt(1000), "testFee")

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q", tt.calc.Value)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculationValue_InvalidDecimal(t *testing.T) {
	t.Parallel()

	existingFees := map[string]Fee{
		"testFee": {
			IsDeductibleFrom: func() *bool { b := true; return &b }(),
		},
	}
	deductible := true

	tests := []struct {
		name    string
		calc    Calculation
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Error - invalid decimal value with comma",
			calc:    Calculation{Type: "percentage", Value: "10,50"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Error - invalid decimal value with letters",
			calc:    Calculation{Type: "flat", Value: "not_a_number"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{}
			err := f.validateCalculationValue(tt.calc, existingFees, &deductible, "testFee", decimal.NewFromInt(1000))

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q", tt.calc.Value)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCalculation_InvalidDecimalReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		calc    Calculation
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Error - invalid calculation value with comma",
			calc:    Calculation{Type: "percentage", Value: "10,50"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name:    "Error - invalid calculation value with letters",
			calc:    Calculation{Type: "flat", Value: "abc"},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fee{
				IsDeductibleFrom: func() *bool { b := false; return &b }(),
			}
			err := f.validateCalculation(tt.calc, "testFee", decimal.NewFromInt(1000))

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q", tt.calc.Value)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
