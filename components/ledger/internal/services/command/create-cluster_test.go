package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/cluster"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestCreateCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := cluster.NewMockRepository(ctrl)

	uc := &UseCase{
		ClusterRepo: mockRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		input          *mmodel.CreateClusterInput
		mockSetup      func()
		expectErr      bool
		expectedProd   *mmodel.Cluster
	}{
		{
			name:           "Success with all fields",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateClusterInput{
				Name: "Test Cluster",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Test Cluster").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{
						ID:             "123",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Name:           "Test Cluster",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil) // Produto criado com sucesso
			},
			expectErr: false,
			expectedProd: &mmodel.Cluster{
				Name:   "Test Cluster",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
		{
			name:           "Error when FindByName fails",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateClusterInput{
				Name: "Failing Cluster",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Failing Cluster").
					Return(false, errors.New("repository error"))
			},
			expectErr:    true,
			expectedProd: nil,
		},
		{
			name:           "Success with default status",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateClusterInput{
				Name:     "Default Status Cluster",
				Status:   mmodel.Status{}, // Empty status
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Default Status Cluster").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Cluster{
						ID:             "124",
						OrganizationID: "org124",
						LedgerID:       "ledger124",
						Name:           "Default Status Cluster",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil)
			},
			expectErr: false,
			expectedProd: &mmodel.Cluster{
				Name:   "Default Status Cluster",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateCluster(ctx, tt.organizationID, tt.ledgerID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedProd.Name, result.Name)
				assert.Equal(t, tt.expectedProd.Status.Code, result.Status.Code)
			}
		})
	}
}
