package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestCreateProduct(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := product.NewMockRepository(ctrl)

	uc := &UseCase{
		ProductRepo: mockRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		input          *mmodel.CreateProductInput
		mockSetup      func()
		expectErr      bool
		expectedProd   *mmodel.Product
	}{
		{
			name:           "Success with all fields",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateProductInput{
				Name: "Test Product",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Test Product").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Product{
						ID:             "123",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Name:           "Test Product",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil) // Produto criado com sucesso
			},
			expectErr: false,
			expectedProd: &mmodel.Product{
				Name:   "Test Product",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
		{
			name:           "Error when FindByName fails",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateProductInput{
				Name: "Failing Product",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Failing Product").
					Return(false, errors.New("repository error"))
			},
			expectErr:    true,
			expectedProd: nil,
		},
		{
			name:           "Success with default status",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateProductInput{
				Name:     "Default Status Product",
				Status:   mmodel.Status{}, // Empty status
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Default Status Product").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Product{
						ID:             "124",
						OrganizationID: "org124",
						LedgerID:       "ledger124",
						Name:           "Default Status Product",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil)
			},
			expectErr: false,
			expectedProd: &mmodel.Product{
				Name:   "Default Status Product",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateProduct(ctx, tt.organizationID, tt.ledgerID, tt.input)

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
