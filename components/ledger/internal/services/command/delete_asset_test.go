// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAssetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		assetID        uuid.UUID
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Cascade soft-delete of all external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				// Asset has: 1 canonical external + 2 custom externals.
				// All 3 must be soft-deleted; the regular account is not returned
				// by ListExternalAccountsByAssetCode and must not be deleted.
				externals := []*mmodel.Account{
					{ID: uuid.New().String(), Type: constant.ExternalAccountType},
					{ID: uuid.New().String(), Type: constant.ExternalAccountType},
					{ID: uuid.New().String(), Type: constant.ExternalAccountType},
				}
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return(externals, nil)
				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(3)
				mockAssetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Success - No external accounts to cascade",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return([]*mmodel.Account{}, nil)
				mockAssetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Asset not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to list external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return(nil, errors.New("error listing accounts"))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to delete an external account",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return([]*mmodel.Account{{ID: uuid.New().String(), Type: constant.ExternalAccountType}}, nil)
				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("error deleting account"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := uc.DeleteAssetByID(ctx, tt.organizationID, tt.ledgerID, tt.assetID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
