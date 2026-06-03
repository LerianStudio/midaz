// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	mongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllPackages(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	enableFlag := true
	packID := uuid.New()
	segmentID := uuid.New()
	orgId := uuid.New()
	resultEntity := []*mongo.Package{
		{
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
	}

	mockPackRepo := pack.NewMockRepository(ctrl)

	filter := http.QueryHeader{
		Limit:          10,
		Page:           1,
		OrganizationID: orgId,
	}

	packSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	tests := []struct {
		name           string
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		errContains    string
		expectedResult []*mongo.Package
	}{
		{
			name:   "Success - Get all packages",
			filter: filter,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindList(gomock.Any(), filter).
					Return(resultEntity, nil)
			},
			expectErr: false,
			expectedResult: []*mongo.Package{
				{
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
		},
		{
			name:   "Error - Get all packages",
			filter: filter,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindList(gomock.Any(), filter).
					Return(nil, constant.ErrBadRequest)
			},
			expectErr:      true,
			errContains:    constant.ErrBadRequest.Error(),
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := packSvc.GetAllPackages(ctx, tt.filter, orgId)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, len(tt.expectedResult))
				assert.Equal(t, tt.expectedResult[0].ID, result[0].ID)
				assert.Equal(t, tt.expectedResult[0].FeeGroupLabel, result[0].FeeGroupLabel)
			}
		})
	}
}
