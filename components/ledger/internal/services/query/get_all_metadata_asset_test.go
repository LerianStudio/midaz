// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestGetAllMetadataAssets(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	matchedID := uuid.New()
	filter := http.QueryHeader{Limit: 5, Page: 1, UseMetadata: true}

	// PostgreSQL reads each batch of matched ids on one page sized to it.
	pagedByMatchedIDs := gomock.Cond(func(q http.QueryHeader) bool {
		return len(q.EntityIDs) == 1 && q.EntityIDs[0] == matchedID && q.Limit == 1 && q.Page == 1
	})

	// A created_at range reaches the metadata store unchanged and PostgreSQL applies it to the batch.
	combinedFilter := filter
	combinedFilter.StartDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	combinedFilter.EndDate = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	batchWithDateRange := gomock.Cond(func(q http.QueryHeader) bool {
		return len(q.EntityIDs) == 1 && q.EntityIDs[0] == matchedID && q.Limit == 1 && q.Page == 1 && !q.StartDate.IsZero()
	})

	tests := []struct {
		name          string
		filter        http.QueryHeader
		mockSetup     func(metadataRepo *mongodb.MockRepository, assetRepo *asset.MockRepository)
		expectErr     bool
		expectedCount int
		expectMeta    bool
	}{
		{
			name: "matched assets are paged by entity ids and carry their metadata",
			mockSetup: func(metadataRepo *mongodb.MockRepository, assetRepo *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, filter, "", metadataListBatchSize).
					Return([]string{matchedID.String()}, nil)
				assetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, pagedByMatchedIDs).
					Return([]*mmodel.Asset{{ID: matchedID.String(), Name: "Test Asset"}}, nil)
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAsset, []string{matchedID.String()}).
					Return([]*mongodb.Metadata{{EntityID: matchedID.String(), Data: map[string]any{"key": "value"}}}, nil)
			},
			expectedCount: 1,
			expectMeta:    true,
		},
		{
			name: "no metadata match yields an empty page without querying postgres",
			mockSetup: func(metadataRepo *mongodb.MockRepository, _ *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, filter, "", metadataListBatchSize).
					Return([]string{}, nil)
			},
			expectedCount: 0,
		},
		{
			name: "a matched id without a live row yields an empty page",
			mockSetup: func(metadataRepo *mongodb.MockRepository, assetRepo *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, filter, "", metadataListBatchSize).
					Return([]string{matchedID.String()}, nil)
				assetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, pagedByMatchedIDs).
					Return(nil, nil)
			},
			expectedCount: 0,
		},
		{
			name: "metadata repository failure is returned",
			mockSetup: func(metadataRepo *mongodb.MockRepository, _ *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, filter, "", metadataListBatchSize).
					Return(nil, errors.New("mongo error"))
			},
			expectErr: true,
		},
		{
			name: "asset repository failure is returned",
			mockSetup: func(metadataRepo *mongodb.MockRepository, assetRepo *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, filter, "", metadataListBatchSize).
					Return([]string{matchedID.String()}, nil)
				assetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, pagedByMatchedIDs).
					Return(nil, errors.New("database error"))
			},
			expectErr: true,
		},
		{
			name:   "a created_at range is applied by postgres to the batch",
			filter: combinedFilter,
			mockSetup: func(metadataRepo *mongodb.MockRepository, assetRepo *asset.MockRepository) {
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), constant.EntityAsset, combinedFilter, "", metadataListBatchSize).
					Return([]string{matchedID.String()}, nil)
				assetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, batchWithDateRange).
					Return([]*mmodel.Asset{{ID: matchedID.String()}}, nil)
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAsset, []string{matchedID.String()}).
					Return([]*mongodb.Metadata{{EntityID: matchedID.String(), Data: map[string]any{"key": "value"}}}, nil)
			},
			expectedCount: 1,
			expectMeta:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.mockSetup(mockMetadataRepo, mockAssetRepo)

			uc := &UseCase{AssetRepo: mockAssetRepo, OnboardingMetadataRepo: mockMetadataRepo}

			requestFilter := filter
			if tt.filter.UseMetadata {
				requestFilter = tt.filter
			}

			result, err := uc.GetAllMetadataAssets(context.Background(), organizationID, ledgerID, requestFilter)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result, "an unmatched filter must yield an empty, non-nil page")
			assert.Len(t, result, tt.expectedCount)

			if tt.expectMeta {
				assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
			}
		})
	}
}
