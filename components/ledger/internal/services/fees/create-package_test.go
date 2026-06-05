// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	mongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkg "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	mongodb "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestCreatePackage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := pkg.NewMockMidazResolver(ctrl)
	enableFlag := true
	packID := uuid.New()
	ledgerID := uuid.New()
	packId := uuid.New()
	segId := uuid.New()
	orgId := uuid.New()

	packSvc := &UseCase{
		packageRepo: mockPackRepo,
		resolver:    mockResolver,
	}

	feeModel := model.Fee{
		FeeLabel:         "Teste",
		CalculationModel: nil,
		ReferenceAmount:  "afterFeesAmount",
		Priority:         0,
		IsDeductibleFrom: nil,
		CreditAccount:    "teste",
	}
	fees := make(map[string]model.Fee, 1)
	fees["teste"] = feeModel

	segIDString := segId.String()
	ledgerIDString := segId.String()
	createPackInput := &model.CreatePackageInput{
		FeeGroupLabel:  "teste group label",
		Description:    nil,
		SegmentID:      &segIDString,
		LedgerID:       ledgerIDString,
		MinAmount:      "2000",
		MaxAmount:      "3000",
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Fee:            fees,
		Enable:         &enableFlag,
	}

	resultEntity := &mongo.Package{
		ID:             packId,
		FeeGroupLabel:  "teste group label",
		Description:    nil,
		SegmentID:      &segId,
		LedgerID:       uuid.New(),
		MinimumAmount:  decimal.NewFromInt(2000),
		MaximumAmount:  decimal.NewFromInt(3000),
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Enable:         &enableFlag,
	}

	packList := []*mongo.Package{
		{
			ID:             packID,
			FeeGroupLabel:  "teste group label",
			SegmentID:      &segId,
			LedgerID:       ledgerID,
			MinimumAmount:  decimal.NewFromInt(100),
			MaximumAmount:  decimal.NewFromInt(1000),
			WaivedAccounts: &[]string{"acc01", "acc02"},
			Enable:         &enableFlag,
		},
	}

	packListTest2 := []*mongo.Package{
		{
			ID:             packID,
			FeeGroupLabel:  "teste group label",
			SegmentID:      nil,
			LedgerID:       ledgerID,
			MinimumAmount:  decimal.NewFromInt(2500),
			MaximumAmount:  decimal.NewFromInt(2600),
			WaivedAccounts: &[]string{"acc01", "acc02"},
			Enable:         &enableFlag,
		},
	}

	packListExistent := []*mongo.Package{
		{
			ID:             packID,
			FeeGroupLabel:  "teste group label",
			SegmentID:      nil,
			LedgerID:       ledgerID,
			MinimumAmount:  decimal.NewFromInt(2000),
			MaximumAmount:  decimal.NewFromInt(3000),
			WaivedAccounts: &[]string{"acc01", "acc02"},
			Enable:         &enableFlag,
		},
	}

	filter := http.QueryHeader{
		OrganizationID: orgId,
		LedgerID:       ledgerID,
		SegmentID:      segId,
	}

	tests := []struct {
		name           string
		packInput      *model.CreatePackageInput
		filter         http.QueryHeader
		orgId          uuid.UUID
		segId          uuid.UUID
		ledgerId       uuid.UUID
		mockSetup      func()
		expectErr      bool
		errContains    string
		expectedResult *mongo.Package
	}{
		{
			name:      "Success - Create a package",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			segId:     segId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packList, nil)

				mockPackRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(resultEntity, nil)
			},
			expectErr: false,
			expectedResult: &mongo.Package{
				ID:             packId,
				FeeGroupLabel:  "teste group label",
				Description:    nil,
				SegmentID:      &segId,
				LedgerID:       uuid.New(),
				MinimumAmount:  decimal.NewFromInt(100),
				MaximumAmount:  decimal.NewFromInt(1000),
				WaivedAccounts: &[]string{"acc01", "acc02"},
				Enable:         &enableFlag,
			},
		},
		{
			name:      "Error - Package already exist",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packListExistent, nil)
			},
			expectErr:      true,
			errContains:    "FEE-0018",
			expectedResult: nil,
		},
		{
			name:      "Error - New package is in range of max and min amount",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packListTest2, nil)
			},
			expectErr:      true,
			errContains:    "FEE-0035",
			expectedResult: nil,
		},
		{
			name:      "Error - Get midaz Account",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			segId:     segId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(constant.ErrFindAccountOnMidaz)
			},
			expectErr:      true,
			errContains:    "FEE-0014",
			expectedResult: nil,
		},
		{
			name:      "Error - Create a package",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			segId:     segId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packList, nil)

				mockPackRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrBadRequest)
			},
			expectErr:      true,
			errContains:    "FEE-0003",
			expectedResult: nil,
		},
		{
			name:      "Error - Create a package duplicate key",
			packInput: createPackInput,
			filter:    filter,
			orgId:     orgId,
			segId:     segId,
			ledgerId:  ledgerID,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packList, nil)

				mockPackRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, mongodb.WriteException{
						WriteConcernError: nil,
						WriteErrors: []mongodb.WriteError{
							{
								Index:   0,
								Code:    11000,
								Message: "duplicate key",
								Details: nil,
								Raw:     nil,
							},
						},
						Labels: nil,
						Raw:    nil,
					})
			},
			expectErr:      true,
			errContains:    "FEE-0018",
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := packSvc.CreatePackage(ctx, tt.packInput, tt.orgId, tt.ledgerId, tt.segId)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.ID, result.ID)
				assert.Equal(t, tt.expectedResult.FeeGroupLabel, result.FeeGroupLabel)
			}
		})
	}
}
