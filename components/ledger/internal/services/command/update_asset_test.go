// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestUpdateAssetByID(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	assetID := uuid.New()

	nameConflict := pkg.ValidateBusinessError(constant.ErrAssetNameOrCodeDuplicate, constant.EntityAsset)

	tests := []struct {
		name      string
		input     *mmodel.UpdateAssetInput
		mockSetup func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository)
		wantErr   error
		wantCode  string
	}{
		{
			name: "Success - Asset updated with metadata",
			input: &mmodel.UpdateAssetInput{
				Name:     "Updated Asset",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "Updated Asset", assetID).
					Return(false, nil)
				assetRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, assetID, gomock.Any()).
					Return(&mmodel.Asset{ID: "123", Name: "Updated Asset", Status: mmodel.Status{Code: "active"}}, nil)
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				metadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
		},
		{
			name: "Success - Empty name skips the name lookup",
			input: &mmodel.UpdateAssetInput{
				Status: mmodel.Status{Code: "inactive"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
				assetRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, assetID, gomock.Any()).
					Return(&mmodel.Asset{ID: "123", Status: mmodel.Status{Code: "inactive"}}, nil)
				metadataRepo.EXPECT().
					Update(gomock.Any(), constant.EntityAsset, assetID.String(), gomock.Any()).
					Return(nil)
			},
		},
		{
			name: "Error - Name held by another asset is a conflict and nothing is updated",
			input: &mmodel.UpdateAssetInput{
				Name:   "US Dollar",
				Status: mmodel.Status{Code: "active"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, _ *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "US Dollar", assetID).
					Return(true, nameConflict)
				assetRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			wantErr:  nameConflict,
			wantCode: constant.ErrAssetNameOrCodeDuplicate.Error(),
		},
		{
			name: "Error - Technical failure on the name lookup is returned and nothing is updated",
			input: &mmodel.UpdateAssetInput{
				Name: "US Dollar",
			},
			mockSetup: func(assetRepo *asset.MockRepository, _ *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "US Dollar", assetID).
					Return(false, errors.New("connection refused"))
				assetRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			wantErr: errors.New("connection refused"),
		},
		{
			name: "Error - Asset not found",
			input: &mmodel.UpdateAssetInput{
				Name:   "Nonexistent Asset",
				Status: mmodel.Status{Code: "inactive"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, _ *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "Nonexistent Asset", assetID).
					Return(false, nil)
				assetRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			wantErr:  pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset),
			wantCode: constant.ErrAssetIDNotFound.Error(),
		},
		{
			name: "Error - Failed to update metadata",
			input: &mmodel.UpdateAssetInput{
				Name:     "Asset with Metadata Error",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "Asset with Metadata Error", assetID).
					Return(false, nil)
				assetRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: "123", Name: "Asset with Metadata Error", Status: mmodel.Status{Code: "active"}}, nil)
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				metadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			wantErr: errors.New("metadata update error"),
		},
		{
			name: "Error - Failure to update asset",
			input: &mmodel.UpdateAssetInput{
				Name:   "Update Failure Asset",
				Status: mmodel.Status{Code: "inactive"},
			},
			mockSetup: func(assetRepo *asset.MockRepository, _ *mongodb.MockRepository) {
				assetRepo.EXPECT().
					FindByNameExcludingID(gomock.Any(), organizationID, ledgerID, "Update Failure Asset", assetID).
					Return(false, nil)
				assetRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update error"))
			},
			wantErr: errors.New("update error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.mockSetup(mockAssetRepo, mockMetadataRepo)

			uc := &UseCase{
				AssetRepo:              mockAssetRepo,
				OnboardingMetadataRepo: mockMetadataRepo,
			}

			result, err := uc.UpdateAssetByID(context.Background(), organizationID, ledgerID, assetID, tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, tt.wantErr.Error(), err.Error())

				if tt.wantCode != "" {
					assert.Equal(t, tt.wantCode, assetUpdateErrorCode(t, err))
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.input.Name, result.Name)
			assert.Equal(t, tt.input.Status, result.Status)
		})
	}
}

// assetUpdateErrorCode extracts the registry code from the conflict or
// not-found business error UpdateAssetByID returns.
func assetUpdateErrorCode(t *testing.T, err error) string {
	t.Helper()

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		return notFound.Code
	}

	t.Fatalf("error %T is not a conflict or not-found business error", err)

	return ""
}
