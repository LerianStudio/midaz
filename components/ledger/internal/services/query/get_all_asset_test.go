// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllAssets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	errMetadata := errors.New("failed to retrieve metadata")

	tests := []struct {
		name           string
		setupMocks     func()
		expectedErr    error
		technicalErr   bool
		expectedAssets []*mmodel.Asset
	}{
		{
			name: "success - assets retrieved with metadata",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter).
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
					FindAll(gomock.Any(), organizationID, ledgerID, filter).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:    errors.New("No assets were found in the search. Please review the search criteria and try again."),
			expectedAssets: nil,
		},
		{
			name: "failure - repository error retrieving assets",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter).
					Return(nil, errors.New("failed to retrieve assets")).
					Times(1)
			},
			expectedErr:    errors.New("failed to retrieve assets"),
			expectedAssets: nil,
		},
		{
			name: "failure - metadata failure is returned as a technical error",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter).
					Return([]*mmodel.Asset{
						{ID: "asset1"},
						{ID: "asset2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Asset", []string{"asset1", "asset2"}).
					Return(nil, errMetadata).
					Times(1)
			},
			expectedErr:    errMetadata,
			technicalErr:   true,
			expectedAssets: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllAssets(ctx, organizationID, ledgerID, filter)

			if tt.technicalErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)

				var nf pkg.EntityNotFoundError
				assert.False(t, errors.As(err, &nf))
				assert.Nil(t, result)
			} else if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedAssets), len(result))
				for i, asset := range result {
					assert.Equal(t, tt.expectedAssets[i].ID, asset.ID)
					assert.Equal(t, tt.expectedAssets[i].Metadata, asset.Metadata)
				}
			}
		})
	}
}
