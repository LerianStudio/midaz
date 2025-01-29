package command

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/cluster"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestDeleteClusterByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClusterRepo := cluster.NewMockRepository(ctrl)

	uc := &UseCase{
		ClusterRepo: mockClusterRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	clusterID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - cluster deleted",
			setupMocks: func() {
				mockClusterRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, clusterID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - cluster not found",
			setupMocks: func() {
				mockClusterRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, clusterID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errors.New("The provided cluster ID does not exist in our records. Please verify the cluster ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockClusterRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, clusterID).
					Return(errors.New("failed to delete cluster")).
					Times(1)
			},
			expectedErr: errors.New("failed to delete cluster"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteClusterByID(ctx, organizationID, ledgerID, clusterID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
