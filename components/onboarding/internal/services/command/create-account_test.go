package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/rabbitmq"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
)

// TestCreateAccountSuccess is responsible to test CreateAccount with success
func TestCreateAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		AssetRepo:     mockAssetRepo,
		PortfolioRepo: mockPortfolioRepo,
		AccountRepo:   mockAccountRepo,
		RabbitMQRepo:  mockRabbitMQ,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	createAccountInput := &mmodel.CreateAccountInput{
		Name:      "Test Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	t.Run("success", func(t *testing.T) {
		mockAssetRepo.EXPECT().
			FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "", createAccountInput.AssetCode).
			Return(true, nil).
			Times(1)

		mockAccountRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(&mmodel.Account{
				ID:        uuid.New().String(),
				AssetCode: createAccountInput.AssetCode,
				Name:      createAccountInput.Name,
				Type:      createAccountInput.Type,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil).
			Times(1)

		mockRabbitMQ.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			Times(1)

		acc, err := uc.CreateAccount(ctx, organizationID, ledgerID, createAccountInput)
		assert.NoError(t, err)
		assert.NotNil(t, acc)
		assert.Equal(t, createAccountInput.AssetCode, acc.AssetCode)
	})
}

func TestCreateAccount2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		AssetRepo:     mockAssetRepo,
		PortfolioRepo: mockPortfolioRepo,
		AccountRepo:   mockAccountRepo,
		RabbitMQRepo:  mockRabbitMQ,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func()
		expectedErr  error
		expectedName string
	}{
		{
			name: "success",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "", "USD").
					Return(true, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{
						ID:        uuid.New().String(),
						AssetCode: "USD",
						Name:      "Test Account",
						Type:      "deposit",
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:  nil,
			expectedName: "Test Account",
		},
		{
			name: "asset not found",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "XYZ",
			},
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "", "XYZ").
					Return(false, nil).
					Times(1)
			},
			expectedErr:  errors.New("The provided asset code does not exist in our records. Please verify the asset code and try again."),
			expectedName: "",
		},
		{
			name: "invalid account type",
			input: &mmodel.CreateAccountInput{
				Name:      "Invalid Account",
				Type:      "invalidType",
				AssetCode: "USD",
			},
			mockSetup:    func() {},
			expectedErr:  errors.New("0066 - The provided 'type' is not valid. Accepted types are: deposit, savings, loans, marketplace, creditCard or external. Please provide a valid type."),
			expectedName: "",
		},
		{
			name: "error creating account",
			input: &mmodel.CreateAccountInput{
				Name:      "Error Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "", "USD").
					Return(true, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to create account")).
					Times(1)
			},
			expectedErr:  errors.New("failed to create account"),
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, account)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, account)
				assert.Equal(t, tt.expectedName, account.Name)
			}
		})
	}
}
