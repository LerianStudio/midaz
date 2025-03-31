package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
	"time"
)

func TestGetAssetRateByID(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           100,
		Scale:          libPointers.Float64(2),
		Source:         libPointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByExternalID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), orgID, ledgerID, exID)

	assert.Equal(t, assetRate, res)
	assert.Nil(t, err)
}

// TestGetAssetRateByIDError is responsible to test GetAssetRateByExternalID with error
func TestGetAssetRateByIDError(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByExternalID(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

func TestGetAssetRateByExternalID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           100,
		Scale:          libPointers.Float64(2),
		Source:         libPointers.String("External System"),
		TTL:            3600,
	}

	// Create an ObjectID for the metadata
	objectID, _ := primitive.ObjectIDFromHex("507f1f77bcf86cd799439011")
	
	metadata := &mongodb.Metadata{
		ID:         objectID,
		EntityID:   id.String(),
		EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
		Data: mongodb.JSON{
			"custom_field": "custom_value",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create mocks
	assetRateRepo := assetrate.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	// Create use case with mocks
	uc := UseCase{
		AssetRateRepo: assetRateRepo,
		MetadataRepo:  metadataRepo,
	}

	// Test cases
	tests := []struct {
		name           string
		setupMocks     func()
		expectedResult *assetrate.AssetRate
		expectedError  error
	}{
		{
			name: "success_with_metadata",
			setupMocks: func() {
				// Setup AssetRateRepo mock
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(assetRate, nil)

				// Setup MetadataRepo mock
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(assetrate.AssetRate{}).Name(), id.String()).
					Return(metadata, nil)
			},
			expectedResult: &assetrate.AssetRate{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				ExternalID:     exID.String(),
				From:           "USD",
				To:             "BRL",
				Rate:           100,
				Scale:          libPointers.Float64(2),
				Source:         libPointers.String("External System"),
				TTL:            3600,
				Metadata: map[string]interface{}{
					"custom_field": "custom_value",
				},
			},
			expectedError: nil,
		},
		{
			name: "success_without_metadata",
			setupMocks: func() {
				// Setup AssetRateRepo mock
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(assetRate, nil)

				// Setup MetadataRepo mock
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(assetrate.AssetRate{}).Name(), id.String()).
					Return(nil, nil)
			},
			expectedResult: assetRate,
			expectedError:  nil,
		},
		{
			name: "error_finding_asset_rate",
			setupMocks: func() {
				// Setup AssetRateRepo mock with error
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(nil, errors.New("database error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"),
		},
		{
			name: "error_finding_metadata",
			setupMocks: func() {
				// Setup AssetRateRepo mock
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(assetRate, nil)

				// Setup MetadataRepo mock with error
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(assetrate.AssetRate{}).Name(), id.String()).
					Return(nil, errors.New("metadata error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("metadata error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks for this test case
			tc.setupMocks()

			// Call the method being tested
			result, err := uc.GetAssetRateByExternalID(context.Background(), orgID, ledgerID, exID)

			// Assert results
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
