// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	mongoPack "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestGetPackageByID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	enableFlag := true
	packID := uuid.New()
	segmentID := uuid.New()
	orgId := uuid.New()
	resultEntity := &mongoPack.Package{
		ID:             packID,
		FeeGroupLabel:  "teste group label",
		Description:    nil,
		SegmentID:      &segmentID,
		LedgerID:       uuid.New(),
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(1000),
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Enable:         &enableFlag,
	}

	tests := []struct {
		name           string
		packageId      uuid.UUID
		orgId          uuid.UUID
		mockSetup      func()
		expectErr      bool
		errContains    string
		expectedResult *mongoPack.Package
	}{
		{
			name:      "Success - Get a package by id",
			packageId: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(resultEntity, nil)
			},
			expectErr: false,
			expectedResult: &mongoPack.Package{
				ID:             packID,
				FeeGroupLabel:  "teste group label",
				Description:    nil,
				SegmentID:      &segmentID,
				LedgerID:       uuid.New(),
				MinimumAmount:  decimal.NewFromInt(100),
				MaximumAmount:  decimal.NewFromInt(1000),
				WaivedAccounts: &[]string{"acc01", "acc02"},
				Enable:         &enableFlag,
			},
		},
		{
			name:      "Error Bad Request - Get a package by id",
			packageId: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrBadRequest)
			},
			expectErr:      true,
			errContains:    "FEE-0003",
			expectedResult: nil,
		},
		{
			name:      "Error Document Not Found - Get a package by id",
			packageId: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, mongo.ErrNoDocuments)
			},
			expectErr:      true,
			errContains:    "No Package entity was found",
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := packSvc.GetPackageByID(ctx, tt.packageId, tt.orgId)

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
				assert.Equal(t, tt.expectedResult.MinimumAmount, result.MinimumAmount)
				assert.Equal(t, tt.expectedResult.MaximumAmount, result.MaximumAmount)
			}
		})
	}
}
