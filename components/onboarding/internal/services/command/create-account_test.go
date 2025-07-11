package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountScenarios tests various scenarios for the CreateAccount function
func TestCreateAccountScenarios(t *testing.T) {
	// Helper function to create a new UseCase with mocks
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *rabbitmq.MockProducerRepository, *mongodb.MockRepository, *settings.MockRepository, *accounttype.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockSettingsRepo := settings.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			RabbitMQRepo:    mockRabbitMQ,
			MetadataRepo:    mockMetadataRepo,
			SettingsRepo:    mockSettingsRepo,
			AccountTypeRepo: mockAccountTypeRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	customAlias := "custom-alias"
	existingAlias := "existing-alias"

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository)
		expectedErr  string
		expectedName string
		expectError  bool
	}{
		{
			name: "success with all fields - accounting validation disabled",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "success with accounting validation enabled",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(&mmodel.AccountType{
						KeyValue: "deposit",
						Name:     "Deposit Account",
					}, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "asset not found",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "XYZ",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()
			},
			expectedErr:  "asset code does not exist",
			expectedName: "",
			expectError:  true,
		},
		{
			name: "invalid account type with validation enabled",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "invalid_type",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "invalid_type").
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:  "The provided 'type' is not valid",
			expectedName: "",
			expectError:  true,
		},
		{
			name: "error creating account",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to create account")).AnyTimes()
			},
			expectedErr:  "failed to create account",
			expectedName: "",
			expectError:  true,
		},
		{
			name: "auto-generate name when not provided",
			input: &mmodel.CreateAccountInput{
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "USD deposit account",
			expectError:  false,
		},
		{
			name: "custom alias - success",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
				Alias:     &customAlias,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "alias already exists",
			input: &mmodel.CreateAccountInput{
				Name:      "Duplicate Alias",
				Type:      "deposit",
				AssetCode: "USD",
				Alias:     &existingAlias,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				// When FindByAlias returns true, it means the alias already exists
				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, pkg.ValidateBusinessError(constant.ErrAliasUnavailability, "Account")).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "alias",
			expectedName: "",
		},
	}

	// Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset controller for each test to avoid interference
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, account)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, account)
				assert.Equal(t, tt.expectedName, account.Name)
			}
		})
	}
}

// TestCreateAccountEdgeCases tests edge cases for the CreateAccount function
func TestCreateAccountEdgeCases(t *testing.T) {
	// Helper function to create a new UseCase with mocks
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *rabbitmq.MockProducerRepository, *mongodb.MockRepository, *settings.MockRepository, *accounttype.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockSettingsRepo := settings.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			RabbitMQRepo:    mockRabbitMQ,
			MetadataRepo:    mockMetadataRepo,
			SettingsRepo:    mockSettingsRepo,
			AccountTypeRepo: mockAccountTypeRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create reusable IDs as strings
	portfolioIDStr := uuid.New().String()
	parentAccountIDStr := uuid.New().String()

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository)
		expectedErr  string
		expectedName string
		expectError  bool
	}{
		{
			name: "lookup portfolio when EntityID is nil but PortfolioID is provided",
			input: &mmodel.CreateAccountInput{
				Name:        "Test Account",
				Type:        "deposit",
				AssetCode:   "USD",
				PortfolioID: &portfolioIDStr,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockPortfolioRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Portfolio{
						ID:       portfolioIDStr,
						EntityID: "entity123",
					}, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "portfolio not found",
			input: &mmodel.CreateAccountInput{
				Name:        "Test Account",
				Type:        "deposit",
				AssetCode:   "USD",
				PortfolioID: &portfolioIDStr,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockPortfolioRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("portfolio not found")).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "portfolio not found",
			expectedName: "",
		},
		{
			name: "parent account check - success",
			input: &mmodel.CreateAccountInput{
				Name:            "Test Account",
				Type:            "deposit",
				AssetCode:       "USD",
				ParentAccountID: &parentAccountIDStr,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{
						ID:        parentAccountIDStr,
						AssetCode: "USD",
					}, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "parent account not found",
			input: &mmodel.CreateAccountInput{
				Name:            "Test Account",
				Type:            "deposit",
				AssetCode:       "USD",
				ParentAccountID: &parentAccountIDStr,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("parent account not found")).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "parent account",
			expectedName: "",
		},
		{
			name: "mismatched asset code",
			input: &mmodel.CreateAccountInput{
				Name:            "Test Account",
				Type:            "deposit",
				AssetCode:       "USD",
				ParentAccountID: &parentAccountIDStr,
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{
						ID:        parentAccountIDStr,
						AssetCode: "EUR", // Different from the input's USD
					}, nil).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "asset code",
			expectedName: "",
		},
		{
			name: "metadata creation error",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
				Metadata: map[string]interface{}{
					"key1": "value1",
				},
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata creation error")).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "metadata creation error",
			expectedName: "",
		},
		{
			name: "rabbitmq error",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("rabbitmq error")).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false, // RabbitMQ errors don't fail the account creation
		},
		{
			name: "with metadata",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
				Metadata: map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings not found (validation disabled)
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
	}

	// Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset controller for each test to avoid interference
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, account)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, account)
				assert.Equal(t, tt.expectedName, account.Name)
			}
		})
	}
}

// TestCreateAccountValidationEdgeCases tests edge cases for accounting validation in CreateAccount
func TestCreateAccountValidationEdgeCases(t *testing.T) {
	// Helper function to create a new UseCase with mocks
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *rabbitmq.MockProducerRepository, *mongodb.MockRepository, *settings.MockRepository, *accounttype.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockSettingsRepo := settings.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			RabbitMQRepo:    mockRabbitMQ,
			MetadataRepo:    mockMetadataRepo,
			SettingsRepo:    mockSettingsRepo,
			AccountTypeRepo: mockAccountTypeRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository)
		expectedErr  string
		expectedName string
		expectError  bool
	}{
		{
			name: "settings found but active=false (validation disabled)",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found but inactive
				activeFalse := false
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeFalse,
					}, nil).
					Times(1)

				// Should not call AccountTypeRepo since validation is disabled
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "settings found but active=nil (validation disabled)",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found but active is nil
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: nil,
					}, nil).
					Times(1)

				// Should not call AccountTypeRepo since validation is disabled
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "case sensitivity - uppercase input finds lowercase database entry",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "DEPOSIT", // Uppercase input
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				// Mock account type found - repository handles case insensitivity
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "DEPOSIT").
					Return(&mmodel.AccountType{
						KeyValue: "deposit", // Lowercase in database
						Name:     "Deposit Account",
					}, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "settings query returns nil entity but no error",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings query returns nil but no error
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, nil).
					Times(1)

				// Should not call AccountTypeRepo since validation is disabled
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "account type query returns nil entity but no error",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				// Mock account type query returns nil but no error - validation passes since no error
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(nil, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "settings query error (not ErrDatabaseItemNotFound)",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings query returns generic error
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(nil, errors.New("database connection error")).
					Times(1)

				// Should not call AccountTypeRepo since settings query failed
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				// Asset repo expectation to handle any unexpected calls
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()
			},
			expectedErr:  "database connection error",
			expectedName: "",
			expectError:  true,
		},
		{
			name: "account type query error (not ErrDatabaseItemNotFound)",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(nil, errors.New("database connection error")).
					Times(1)

				// Asset repo expectation to handle any unexpected calls
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()
			},
			expectedErr:  "database connection error",
			expectedName: "",
			expectError:  true,
		},
		{
			name: "mixed case input validation",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "DePoSiT", // Mixed case input
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository, mockSettingsRepo *settings.MockRepository, mockAccountTypeRepo *accounttype.MockRepository) {
				mockRabbitMQ.EXPECT().
					CheckRabbitMQHealth().
					Return(true).
					Times(1)

				// Mock accounting validation - settings found and active
				activeTrue := true
				mockSettingsRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, constant.AccountingValidationEnabledKey).
					Return(&mmodel.Settings{
						Key:    constant.AccountingValidationEnabledKey,
						Active: &activeTrue,
					}, nil).
					Times(1)

				// Mock account type found - repository handles case insensitivity
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "DePoSiT").
					Return(&mmodel.AccountType{
						KeyValue: "deposit", // Lowercase in database
						Name:     "Deposit Account",
					}, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
						account.ID = uuid.New().String()
						return account, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo, mockSettingsRepo, mockAccountTypeRepo)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, account)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, account)
				assert.Equal(t, tt.expectedName, account.Name)
			}
		})
	}
}
