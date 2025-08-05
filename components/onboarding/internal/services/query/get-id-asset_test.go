package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetAssetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:    mockAssetRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		assetID        uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Asset
	}{
		{
			name:           "Success - Retrieve asset with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				assetID := uuid.New()
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: assetID.String(), Name: "Test Asset", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Asset{
				ID:       "valid-uuid",
				Name:     "Test Asset",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
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
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				assetID := uuid.New()
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: assetID.String(), Name: "Test Asset", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAssetByID(ctx, tt.organizationID, tt.ledgerID, tt.assetID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
