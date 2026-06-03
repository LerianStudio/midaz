// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	pkg "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreatePackage_InvalidDecimalMinAmount(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := pkg.NewMockMidazResolver(ctrl)
	enableFlag := true

	packSvc := &UseCase{
		packageRepo: mockPackRepo,
		resolver:    mockResolver,
	}

	tests := []struct {
		name      string
		minAmount string
		maxAmount string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Error - invalid minAmount with comma separator",
			minAmount: "1000,50",
			maxAmount: "2000",
			wantErr:   true,
			errMsg:    "FEE-0042",
		},
		{
			name:      "Error - invalid minAmount with letters",
			minAmount: "abc",
			maxAmount: "2000",
			wantErr:   true,
			errMsg:    "FEE-0042",
		},
		{
			name:      "Error - invalid maxAmount with comma separator",
			minAmount: "1000",
			maxAmount: "2000,50",
			wantErr:   true,
			errMsg:    "FEE-0042",
		},
		{
			name:      "Error - invalid maxAmount with letters",
			minAmount: "1000",
			maxAmount: "xyz",
			wantErr:   true,
			errMsg:    "FEE-0042",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			segIDString := uuid.New().String()
			ledgerIDString := uuid.New().String()

			feeModel := model.Fee{
				FeeLabel:         "Teste",
				CalculationModel: nil,
				ReferenceAmount:  "afterFeesAmount",
				CreditAccount:    "teste",
			}
			fees := make(map[string]model.Fee, 1)
			fees["teste"] = feeModel

			cpi := &model.CreatePackageInput{
				FeeGroupLabel: "teste group label",
				SegmentID:     &segIDString,
				LedgerID:      ledgerIDString,
				MinAmount:     tt.minAmount,
				MaxAmount:     tt.maxAmount,
				Fee:           fees,
				Enable:        &enableFlag,
			}

			// Midaz validation passes, range validation passes (mocked)
			mockResolver.EXPECT().
				AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil).
				AnyTimes()

			mockPackRepo.EXPECT().
				FindList(gomock.Any(), gomock.Any()).
				Return([]*pack.Package{}, nil).
				AnyTimes()

			ctx := context.Background()
			result, err := packSvc.CreatePackage(ctx, cpi, uuid.New(), uuid.New(), uuid.New())

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value %q/%q", tt.minAmount, tt.maxAmount)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.errMsg, "Expected FEE-0042 error code")
			}
		})
	}
}
