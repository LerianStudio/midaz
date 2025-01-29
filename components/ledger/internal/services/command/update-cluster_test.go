package command

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

func TestUpdateClusterByID(t *testing.T) {
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
		input          *mmodel.UpdateClusterInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Cluster updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			input: &mmodel.UpdateClusterInput{
				Name:     "Updated Cluster",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockClusterRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{ID: "123", Name: "Updated Cluster", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Cluster not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			input: &mmodel.UpdateClusterInput{
				Name:     "Nonexistent Cluster",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockClusterRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			input: &mmodel.UpdateClusterInput{
				Name:     "Cluster with Metadata Error",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockClusterRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{ID: "123", Name: "Cluster with Metadata Error", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to update cluster",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			clusterID:      uuid.New(),
			input: &mmodel.UpdateClusterInput{
				Name:     "Update Failure Cluster",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockClusterRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateClusterByID(ctx, tt.organizationID, tt.ledgerID, tt.clusterID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}
