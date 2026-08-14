// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupDeleteAssetUseCase(t *testing.T) (*UseCase, *asset.MockRepository, *account.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	return &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
	}, mockAssetRepo, mockAccountRepo
}

func TestDeleteAssetByID(t *testing.T) {
	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		assetID        uuid.UUID
		mockSetup      func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository)
		expectErr      bool
	}{
		{
			name:           "Success - Cascade soft-delete of all external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)

				// Three distinct externals are returned; each must be
				// deleted by its specific account ID (not just three deletes).
				extID1 := uuid.New()
				extID2 := uuid.New()
				extID3 := uuid.New()
				externals := []*mmodel.Account{
					{ID: extID1.String(), Type: constant.ExternalAccountType},
					{ID: extID2.String(), Type: constant.ExternalAccountType},
					{ID: extID3.String(), Type: constant.ExternalAccountType},
				}
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return(externals, nil)

				mockAccountRepo.EXPECT().Delete(gomock.Any(), org, ledger, nil, extID1).Return(nil)
				mockAccountRepo.EXPECT().Delete(gomock.Any(), org, ledger, nil, extID2).Return(nil)
				mockAccountRepo.EXPECT().Delete(gomock.Any(), org, ledger, nil, extID3).Return(nil)

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
			mockSetup: func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository) {
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
			mockSetup: func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository) {
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
			mockSetup: func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository) {
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
			mockSetup: func(org, ledger uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository) {
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
			uc, mockAssetRepo, mockAccountRepo := setupDeleteAssetUseCase(t)
			tt.mockSetup(tt.organizationID, tt.ledgerID, mockAssetRepo, mockAccountRepo)

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
