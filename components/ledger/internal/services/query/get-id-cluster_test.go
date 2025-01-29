package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/cluster"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetClusterByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClusterRepo := cluster.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		ClusterRepo:  mockClusterRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		clusterID      uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Cluster
	}{
		{
			name:           "Success - Retrieve cluster with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			mockSetup: func() {
				clusterID := uuid.New()
				mockClusterRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{ID: clusterID.String(), Name: "Test Cluster", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Cluster{
				ID:       "valid-uuid",
				Name:     "Test Cluster",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
		},
		{
			name:           "Error - Cluster not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			mockSetup: func() {
				mockClusterRepo.EXPECT().
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
			clusterID:      uuid.New(),
			mockSetup: func() {
				clusterID := uuid.New()
				mockClusterRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{ID: clusterID.String(), Name: "Test Cluster", Status: mmodel.Status{Code: "active"}}, nil)
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
			result, err := uc.GetClusterByID(ctx, tt.organizationID, tt.ledgerID, tt.clusterID)

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
