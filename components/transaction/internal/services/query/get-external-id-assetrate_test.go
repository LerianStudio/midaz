// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
)

func TestGetAssetRateByID(t *testing.T) {
	t.Parallel()

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

	uc.AssetRateRepo.(*assetrate.MockRepository). //nolint:forcetypeassert

							EXPECT().
							FindByExternalID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
							Return(assetRate, nil).
							Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), orgID, ledgerID, exID)

	assert.Equal(t, assetRate, res)
	require.NoError(t, err)
}

// TestGetAssetRateByIDError is responsible to test GetAssetRateByExternalID with error.
func TestGetAssetRateByIDError(t *testing.T) {
	t.Parallel()

	id := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository). //nolint:forcetypeassert

							EXPECT().
							FindByExternalID(gomock.Any(), organizationID, ledgerID, id).
							Return(nil, errors.New(errMSG)). //nolint:err113
							Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), organizationID, ledgerID, id)

	require.Error(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

func TestGetAssetRateByExternalID(t *testing.T) { //nolint:funlen
	t.Parallel()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	ar := &assetrate.AssetRate{
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

	// Test cases
	tests := []struct {
		name           string
		setupMocks     func(arRepo *assetrate.MockRepository, metaRepo *mongodb.MockRepository)
		expectedResult *assetrate.AssetRate
		expectedError  error
	}{
		{
			name: "success_with_metadata",
			setupMocks: func(arRepo *assetrate.MockRepository, metaRepo *mongodb.MockRepository) {
				arRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(ar, nil)

				metaRepo.EXPECT().
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
				Metadata: map[string]any{
					"custom_field": "custom_value",
				},
			},
			expectedError: nil,
		},
		{
			name: "success_without_metadata",
			setupMocks: func(arRepo *assetrate.MockRepository, metaRepo *mongodb.MockRepository) {
				arRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(ar, nil)

				metaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(assetrate.AssetRate{}).Name(), id.String()).
					Return(nil, nil)
			},
			expectedResult: ar,
			expectedError:  nil,
		},
		{
			name: "error_finding_asset_rate",
			setupMocks: func(arRepo *assetrate.MockRepository, metaRepo *mongodb.MockRepository) {
				arRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(nil, errors.New("database error")) //nolint:err113
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"), //nolint:err113
		},
		{
			name: "error_finding_metadata",
			setupMocks: func(arRepo *assetrate.MockRepository, metaRepo *mongodb.MockRepository) {
				arRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, exID).
					Return(ar, nil)

				metaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(assetrate.AssetRate{}).Name(), id.String()).
					Return(nil, errors.New("metadata error")) //nolint:err113
			},
			expectedResult: nil,
			expectedError:  errors.New("metadata error"), //nolint:err113
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			arRepo := assetrate.NewMockRepository(ctrl)
			metaRepo := mongodb.NewMockRepository(ctrl)
			uc := UseCase{AssetRateRepo: arRepo, MetadataRepo: metaRepo}

			// Setup mocks for this test case
			tc.setupMocks(arRepo, metaRepo)

			// Call the method being tested
			result, err := uc.GetAssetRateByExternalID(context.Background(), orgID, ledgerID, exID)

			// Assert results
			if tc.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
