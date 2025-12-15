package command

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountScenarios tests various scenarios for the CreateAccount function
func TestCreateAccountScenarios(t *testing.T) {
	// Helper function to create a new UseCase with mocks
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *mbootstrap.MockBalancePort) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceGRPC := mbootstrap.NewMockBalancePort(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			MetadataRepo:    mockMetadataRepo,
			AccountTypeRepo: mockAccountTypeRepo,
			BalancePort: mockBalanceGRPC,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceGRPC
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	customAlias := "custom-alias"
	existingAlias := "existing-alias"

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		envVar       string
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort)
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
			envVar: "", // Empty means validation disabled
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
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
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "success with external account type - skip validation",
			input: &mmodel.CreateAccountInput{
				Name:      "External Account",
				Type:      "external",
				AssetCode: "USD",
			},
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				// AccountTypeRepo should not be called for external type
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "External Account",
			expectError:  false,
		},
		{
			name: "success with EXTERNAL account type - case insensitive",
			input: &mmodel.CreateAccountInput{
				Name:      "External Account",
				Type:      "EXTERNAL",
				AssetCode: "USD",
			},
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				// AccountTypeRepo should not be called for external type (case insensitive)
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "External Account",
			expectError:  false,
		},
		{
			name: "asset not found",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "XYZ",
			},
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			name: "auto-generate name when not provided",
			input: &mmodel.CreateAccountInput{
				Type:      "deposit",
				AssetCode: "USD",
			},
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "error creating account",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			name: "alias already exists",
			input: &mmodel.CreateAccountInput{
				Name:      "Duplicate Alias",
				Type:      "deposit",
				AssetCode: "USD",
				Alias:     &existingAlias,
			},
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			// Set up environment variable
			originalEnv := os.Getenv("ACCOUNT_TYPE_VALIDATION")
			envValue := tt.envVar
			if envValue == "{{organizationID}}:{{ledgerID}},other-org:other-ledger" {
				envValue = organizationID.String() + ":" + ledgerID.String() + ",other-org:other-ledger"
			}
			os.Setenv("ACCOUNT_TYPE_VALIDATION", envValue)
			defer func() {
				os.Setenv("ACCOUNT_TYPE_VALIDATION", originalEnv)
			}()

			// Reset controller for each test to avoid interference
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, token)

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
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *mbootstrap.MockBalancePort) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceGRPC := mbootstrap.NewMockBalancePort(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			MetadataRepo:    mockMetadataRepo,
			AccountTypeRepo: mockAccountTypeRepo,
			BalancePort: mockBalanceGRPC,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceGRPC
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
		envVar       string
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort)
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata creation error")).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectError:  true,
			expectedErr:  "metadata creation error",
			expectedName: "",
		},
		{
			name: "grpc error - balance creation fails",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("grpc create balance error")).Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).Times(1)
			},
			expectedErr:  "default balance could not be created",
			expectedName: "",
			expectError:  true,
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
			envVar: "",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
	}

	// Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			originalEnv := os.Getenv("ACCOUNT_TYPE_VALIDATION")
			os.Setenv("ACCOUNT_TYPE_VALIDATION", tt.envVar)
			defer func() {
				os.Setenv("ACCOUNT_TYPE_VALIDATION", originalEnv)
			}()

			// Reset controller for each test to avoid interference
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, token)

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
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *mbootstrap.MockBalancePort) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceGRPC := mbootstrap.NewMockBalancePort(ctrl)

		uc := &UseCase{
			AssetRepo:       mockAssetRepo,
			PortfolioRepo:   mockPortfolioRepo,
			AccountRepo:     mockAccountRepo,
			MetadataRepo:    mockMetadataRepo,
			AccountTypeRepo: mockAccountTypeRepo,
			BalancePort: mockBalanceGRPC,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceGRPC
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		envVar       string
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort)
		expectedErr  string
		expectedName string
		expectError  bool
	}{
		{
			name: "validation disabled - organization:ledger not in env var",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			envVar: "other-org:other-ledger,another-org:another-ledger",
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "mixed case input validation",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "DePoSiT", // Mixed case input
				AssetCode: "USD",
			},
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in
						out.ID = uuid.New().String()
						return &out, nil
					}).AnyTimes()

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				mockBalance.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(1)
			},
			expectedErr:  "",
			expectedName: "Test Account",
			expectError:  false,
		},
		{
			name: "account type query error (not ErrDatabaseItemNotFound)",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			envVar: "{{organizationID}}:{{ledgerID}},other-org:other-ledger", // Will be replaced with actual IDs
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *mbootstrap.MockBalancePort) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			originalEnv := os.Getenv("ACCOUNT_TYPE_VALIDATION")
			envValue := tt.envVar
			if envValue == "{{organizationID}}:{{ledgerID}},other-org:other-ledger" {
				envValue = organizationID.String() + ":" + ledgerID.String() + ",other-org:other-ledger"
			}
			os.Setenv("ACCOUNT_TYPE_VALIDATION", envValue)
			defer func() {
				os.Setenv("ACCOUNT_TYPE_VALIDATION", originalEnv)
			}()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance)

			account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, token)

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

// TestCreateAccountBlockedFlag ensures the blocked flag is persisted when provided
func TestCreateAccountBlockedFlag(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	originalEnv := os.Getenv("ACCOUNT_TYPE_VALIDATION")
	os.Setenv("ACCOUNT_TYPE_VALIDATION", "")
	defer func() {
		os.Setenv("ACCOUNT_TYPE_VALIDATION", originalEnv)
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceGRPC := mbootstrap.NewMockBalancePort(ctrl)

	uc := &UseCase{
		AssetRepo:       mockAssetRepo,
		PortfolioRepo:   mockPortfolioRepo,
		AccountRepo:     mockAccountRepo,
		MetadataRepo:    mockMetadataRepo,
		AccountTypeRepo: mockAccountTypeRepo,
		BalancePort: mockBalanceGRPC,
	}

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			out.ID = uuid.New().String()
			return &out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBalanceGRPC.EXPECT().
		CreateBalanceSync(gomock.Any(), gomock.Any()).
		Return(nil, nil).Times(1)

	// Input with blocked=true
	blocked := true
	input := &mmodel.CreateAccountInput{
		Name:      "Blocked Account",
		Type:      "deposit",
		AssetCode: "USD",
		Blocked:   &blocked,
	}

	token := "Bearer test-token"
	acc, err := uc.CreateAccount(ctx, organizationID, ledgerID, input, token)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if acc == nil {
		t.Fatalf("expected account, got nil")
	}

	if acc.Blocked == nil || !*acc.Blocked {
		t.Fatalf("expected account.Blocked to be true, got false")
	}
}
