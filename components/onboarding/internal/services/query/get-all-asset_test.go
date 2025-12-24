package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
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
		name              string
		setupMocks        func()
		expectBusinessErr bool
		expectInternalErr bool
		expectedAssets    []*mmodel.Asset
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
			expectBusinessErr: false,
			expectInternalErr: false,
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
			expectBusinessErr: true,
			expectInternalErr: false,
			expectedAssets:    nil,
		},
		{
			name: "failure - repository error retrieving assets",
			setupMocks: func() {
				mockAssetRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
					Return(nil, errors.New("failed to retrieve assets")).
					Times(1)
			},
			expectBusinessErr: false,
			expectInternalErr: true,
			expectedAssets:    nil,
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
					Return(nil, errors.New("failed to retrieve metadata")).
					Times(1)
			},
			expectBusinessErr: true,
			expectInternalErr: false,
			expectedAssets:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllAssets(ctx, organizationID, ledgerID, filter)

			if tt.expectInternalErr {
				assert.Error(t, err)
				var internalErr pkg.InternalServerError
				assert.True(t, errors.As(err, &internalErr), "expected InternalServerError type")
				assert.Nil(t, result)
			} else if tt.expectBusinessErr {
				assert.Error(t, err)
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
