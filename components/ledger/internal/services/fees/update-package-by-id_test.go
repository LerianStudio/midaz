// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	mongoPack "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	pkg "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

func TestUpdatePackage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	flagTrue := true

	mockPackageRepo := pack.NewMockRepository(ctrl)
	mockResolver := pkg.NewMockMidazResolver(ctrl)

	orgId := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
	enableFlag := true
	calculation := make([]model.Calculation, 1)
	calculation[0] = model.Calculation{
		Type:  "percentage",
		Value: "12",
	}

	feeRemove := make(map[string]model.Fee)
	feeRemove["fees"] = model.Fee{}

	calculationEntity := make([]mongoPack.Calculation, 1)
	calculationEntity[0] = mongoPack.Calculation{
		Type:  "percentage",
		Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(160)},
	}

	feeEntity := make(map[string]mongoPack.Fee)
	feeEntity["teste"] = mongoPack.Fee{
		FeeLabel: "atualizado",
		CalculationModel: mongoPack.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculationEntity,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	fee := make(map[string]model.Fee)
	fee["fees"] = model.Fee{
		FeeLabel: "atualizado",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculation,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         3,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	feeAmountData := make(map[string]model.Fee)
	feeAmountData["fees"] = model.Fee{
		FeeLabel: "atualizado",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculation,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         2,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      feeAmountData,
		LedgerID:  ledgerID,
		SegmentID: nil,
	}

	minAmount := "900"
	maxAmount := "1000"
	packToUpdate := &model.UpdatePackageInput{
		FeeGroupLabel:  "atualiza",
		Description:    "atualiza description",
		MinAmount:      &minAmount,
		MaxAmount:      &maxAmount,
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Fee:            fee,
		EnablePackage:  &flagTrue,
	}

	packSvc := &UseCase{
		packageRepo: mockPackageRepo,
		resolver:    mockResolver,
	}

	filter := http.QueryHeader{
		OrganizationID: orgId,
		LedgerID:       ledgerID,
	}

	packEntity := []*mongoPack.Package{
		{
			ID:             packID,
			FeeGroupLabel:  "teste group label",
			Description:    nil,
			SegmentID:      nil,
			LedgerID:       ledgerID,
			MinimumAmount:  decimal.NewFromInt(100),
			MaximumAmount:  decimal.NewFromInt(1000),
			WaivedAccounts: &[]string{"acc01", "acc02"},
			Enable:         &enableFlag,
		},
	}

	tests := []struct {
		name        string
		packId      uuid.UUID
		orgId       uuid.UUID
		filter      http.QueryHeader
		packInput   *model.UpdatePackageInput
		mockSetup   func()
		expectErr   bool
		errContains string
	}{
		{
			name:      "Success - Update package by id",
			packId:    packID,
			packInput: &model.UpdatePackageInput{Fee: feeRemove},
			mockSetup: func() {
				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr: false,
		},
		{
			name:      "Success - Update package by id ",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr: false,
		},
		{
			name:      "Error - Update package by id not found",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(ErrDatabaseItemNotFound)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "No Package entity was found",
		},
		{
			name:      "Error - Update package by id",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(constant.ErrBadRequest)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "FEE-0003",
		},
		{
			name:      "Error - No fields to update package by id",
			packId:    uuid.New(),
			orgId:     orgId,
			packInput: &model.UpdatePackageInput{},
			mockSetup: func() {
				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "FEE-0017",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := packSvc.UpdatePackageByID(ctx, tt.packId, tt.orgId, tt.packInput)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageByID_UpdatedAtFieldSet(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackageRepo := pack.NewMockRepository(ctrl)
	mockResolver := pkg.NewMockMidazResolver(ctrl)

	orgId := uuid.New()
	packID := uuid.New()
	feeGroupLabel := "new label"
	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      map[string]model.Fee{},
		LedgerID:  uuid.New(),
		SegmentID: nil,
	}

	packSvc := &UseCase{
		packageRepo: mockPackageRepo,
		resolver:    mockResolver,
	}

	input := &model.UpdatePackageInput{
		FeeGroupLabel: feeGroupLabel,
	}

	var capturedUpdateFields interface{}

	mockPackageRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(amountData, nil)

	mockPackageRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, updateFields interface{}) error {
			capturedUpdateFields = updateFields
			return nil
		})

	err := packSvc.UpdatePackageByID(context.Background(), packID, orgId, input)
	assert.NoError(t, err)

	// Assert that updated_at is set in the updateFields
	updateMap, ok := capturedUpdateFields.(*bson.M)
	if !ok {
		updateMap2, ok2 := capturedUpdateFields.(bson.M)
		if !ok2 {
			t.Fatalf("updateFields is not a bson.M: %T", capturedUpdateFields)
		}
		updateMap = &updateMap2
	}
	setFields, ok := (*updateMap)["$set"].(bson.M)
	assert.True(t, ok, "expected $set in updateFields")
	_, hasUpdatedAt := setFields["updated_at"]
	assert.True(t, hasUpdatedAt, "expected updated_at in $set fields")
}
