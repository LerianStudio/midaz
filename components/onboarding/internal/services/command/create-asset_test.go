package command

import (
	"context"
	"errors"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestCreateAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		AssetRepo:    mockAssetRepo,
		AccountRepo:  mockAccountRepo,
		RabbitMQRepo: mockRabbitMQ,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	createAccountInput := &mmodel.CreateAccountInput{
		Name:      "Test Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	tests := []struct {
		name        string
		input       *mmodel.CreateAssetInput
		mockSetup   func()
		expectedErr error
		expectedRes *mmodel.Asset
	}{
		{
			name: "success - asset created",
			input: &mmodel.CreateAssetInput{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: libPointers.String("Active asset"),
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "USD Dollar", "USD").
					Return(false, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{
						ID:        uuid.New().String(),
						Name:      "USD Dollar",
						Type:      "currency",
						Code:      "USD",
						Status:    mmodel.Status{Code: "ACTIVE"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Return(nil, nil).
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
			},
			expectedErr: nil,
			expectedRes: &mmodel.Asset{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
		},
		{
			name: "failure - invalid type",
			input: &mmodel.CreateAssetInput{
				Name: "Invalid Asset",
				Type: "invalidType",
				Code: "INV",
			},
			mockSetup: func() {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)
			},

			expectedErr: errors.New("0040 - The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type."),
			expectedRes: nil,
		},
		{
			name: "failure - error creating asset",
			input: &mmodel.CreateAssetInput{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
			},
			mockSetup: func() {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "USD Dollar", "USD").
					Return(false, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to create asset")).
					Times(1)
			},
			expectedErr: errors.New("failed to create asset"),
			expectedRes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateAsset(ctx, organizationID, ledgerID, tt.input)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedRes.Name, result.Name)
				assert.Equal(t, tt.expectedRes.Code, result.Code)
				assert.Equal(t, tt.expectedRes.Type, result.Type)
			}
		})
	}
}
