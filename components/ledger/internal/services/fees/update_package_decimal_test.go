// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/stretchr/testify/assert"
)

func TestConvertFeeToMongoFormat_InvalidDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fee     model.Fee
		wantErr bool
		errMsg  string
	}{
		{
			name: "Error - invalid calculation value with comma",
			fee: model.Fee{
				FeeLabel: "test",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations: []model.Calculation{
						{Type: "flat", Value: "100,50"},
					},
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: boolPtr(false),
				CreditAccount:    "account",
			},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name: "Error - invalid calculation value with letters",
			fee: model.Fee{
				FeeLabel: "test",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations: []model.Calculation{
						{Type: "flat", Value: "abc"},
					},
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: boolPtr(false),
				CreditAccount:    "account",
			},
			wantErr: true,
			errMsg:  "Remember to use dot",
		},
		{
			name: "Success - valid calculation value",
			fee: model.Fee{
				FeeLabel: "test",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations: []model.Calculation{
						{Type: "flat", Value: "100.50"},
					},
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: boolPtr(false),
				CreditAccount:    "account",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := &UseCase{}
			result, err := uc.convertFeeToMongoFormat(tt.fee)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value")
				assert.Equal(t, pack.Fee{}, result)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, pack.Fee{}, result)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
