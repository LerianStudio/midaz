// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

func TestGetAllAssets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:    mockAssetRepo,
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name           string
		setupMocks     func()
		expectedErr    error
		expectedAssets []*mmodel.Asset
	}{
		{
			name: "success - assets retrieved with metadata",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
					Return([]*mmodel.Asset{
						{ID: "asset1"},
						{ID: "asset2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Asset", []string{"asset1", "asset2"}).
					Return([]*mongodb.Metadata{
						{EntityID: "asset1", Data: map[string]any{"key1": "value1"}},
						{EntityID: "asset2", Data: map[string]any{"key2": "value2"}},
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedAssets: []*mmodel.Asset{
				{ID: "asset1", Metadata: map[string]any{"key1": "value1"}},
				{ID: "asset2", Metadata: map[string]any{"key2": "value2"}},
			},
		},
		{
			name: "failure - assets not found",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:    errNoAssetsFound,
			expectedAssets: nil,
		},
		{
			name: "failure - repository error retrieving assets",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
					Return(nil, errFailedToRetrieveAssets).
					Times(1)
			},
			expectedErr:    errFailedToRetrieveAssets,
			expectedAssets: nil,
		},
		{
			name: "failure - metadata retrieval error",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
					Return([]*mmodel.Asset{
						{ID: "asset1"},
						{ID: "asset2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Asset", []string{"asset1", "asset2"}).
					Return(nil, errFailedToRetrieveMetadata).
					Times(1)
			},
			expectedErr:    errNoAssetsFound,
			expectedAssets: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllAssets(ctx, organizationID, ledgerID, filter)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, len(tt.expectedAssets))

				for i, asset := range result {
					assert.Equal(t, tt.expectedAssets[i].ID, asset.ID)
					assert.Equal(t, tt.expectedAssets[i].Metadata, asset.Metadata)
				}
			}
		})
	}
}
