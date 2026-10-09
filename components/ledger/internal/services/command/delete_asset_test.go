// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupDeleteAssetUseCase(t *testing.T) (*UseCase, *asset.MockRepository, *account.MockRepository, *onbMongo.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)

	return &UseCase{
		AssetRepo:              mockAssetRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}, mockAssetRepo, mockAccountRepo, mockMetadataRepo
}

func TestDeleteAssetByID(t *testing.T) {
	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		assetID        uuid.UUID
		mockSetup      func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository)
		expectErr      bool
	}{
		{
			name:           "Success - Cascade soft-delete of all external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
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
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Return(nil).
					Times(1)
				// Only the asset's metadata is soft-deleted; the external
				// accounts never carry a metadata document.
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, gomock.Any()).
					Times(0)
			},
			expectErr: false,
		},
		{
			name:           "Success - No external accounts to cascade",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return([]*mmodel.Account{}, nil)
				mockAssetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name:           "Error - Asset not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Times(0)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to list external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return(nil, errors.New("error listing accounts"))
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Times(0)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to delete an external account",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return([]*mmodel.Account{{ID: uuid.New().String(), Type: constant.ExternalAccountType}}, nil)
				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("error deleting account"))
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Times(0)
			},
			expectErr: true,
		},
		{
			name:           "Success - metadata soft delete failure does not fail the delete",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func(org, ledger, assetID uuid.UUID, mockAssetRepo *asset.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *onbMongo.MockRepository) {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: assetID.String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), "asset123").
					Return([]*mmodel.Account{}, nil)
				mockAssetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAsset, assetID.String()).
					Return(errors.New("mongo unavailable")).
					Times(3)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, mockAssetRepo, mockAccountRepo, mockMetadataRepo := setupDeleteAssetUseCase(t)
			tt.mockSetup(tt.organizationID, tt.ledgerID, tt.assetID, mockAssetRepo, mockAccountRepo, mockMetadataRepo)

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
