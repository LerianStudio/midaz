// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountScenarios tests various scenarios for the CreateAccount function
func TestCreateAccountScenarios(t *testing.T) {
	// Helper function to create a new UseCase with mocks
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *balance.MockRepository, *ledger.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockLedgerRepo := ledger.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:              mockAssetRepo,
			PortfolioRepo:          mockPortfolioRepo,
			AccountRepo:            mockAccountRepo,
			OnboardingMetadataRepo: mockMetadataRepo,
			AccountTypeRepo:        mockAccountTypeRepo,
			BalanceRepo:            mockBalanceRepo,
			LedgerRepo:             mockLedgerRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceRepo, mockLedgerRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	customAlias := "custom-alias"
	existingAlias := "existing-alias"

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository)
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo)

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
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *balance.MockRepository, *ledger.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockLedgerRepo := ledger.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:              mockAssetRepo,
			PortfolioRepo:          mockPortfolioRepo,
			AccountRepo:            mockAccountRepo,
			OnboardingMetadataRepo: mockMetadataRepo,
			AccountTypeRepo:        mockAccountTypeRepo,
			BalanceRepo:            mockBalanceRepo,
			LedgerRepo:             mockLedgerRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceRepo, mockLedgerRepo
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
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository)
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
			},
			expectError:  true,
			expectedErr:  "metadata creation error",
			expectedName: "",
		},
		{
			name: "error - balance creation fails",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("create balance error")).Times(1)

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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo)

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
	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *balance.MockRepository, *ledger.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockLedgerRepo := ledger.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:              mockAssetRepo,
			PortfolioRepo:          mockPortfolioRepo,
			AccountRepo:            mockAccountRepo,
			OnboardingMetadataRepo: mockMetadataRepo,
			AccountTypeRepo:        mockAccountTypeRepo,
			BalanceRepo:            mockBalanceRepo,
			LedgerRepo:             mockLedgerRepo,
		}

		return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceRepo, mockLedgerRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name         string
		input        *mmodel.CreateAccountInput
		mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository)
		expectedErr  string
		expectedName string
		expectError  bool
	}{
		{
			name: "validation disabled - ledger settings not configured",
			input: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).AnyTimes()

				mockBalance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, nil).AnyTimes()
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
			mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockMetadataRepo *mongodb.MockRepository, mockAccountTypeRepo *accounttype.MockRepository, mockBalance *balance.MockRepository, mockLedgerRepo *ledger.MockRepository) {
				mockLedgerRepo.EXPECT().
					GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]any{"accounting": map[string]any{"validateAccountType": true}}, nil).AnyTimes()

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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			token := "Bearer test-token"
			uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo := setupTest(ctrl)

			tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalance, mockLedgerRepo)

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

// TestCreateAccountWithOptionsPendingCRMLink validates the internal saga-mode
// (PendingCRMLink=true) entry point: account must land in PENDING_CRM_LINK
// with blocked=true and a default balance that disallows both send and
// receive. This path is not exposed on any public HTTP route.
func TestCreateAccountWithOptionsPendingCRMLink(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		PortfolioRepo:          mockPortfolioRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		AccountTypeRepo:        mockAccountTypeRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	// Capture the persisted account to assert its derived fields.
	var capturedAccount *mmodel.Account
	mockAccountRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			out.ID = uuid.New().String()
			capturedAccount = &out
			return &out, nil
		}).Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	// Capture the balance to assert its AllowSending / AllowReceiving flags.
	var capturedBalance *mmodel.Balance
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Balance) (*mmodel.Balance, error) {
			out := *in
			capturedBalance = &out
			return &out, nil
		}).Times(1)

	input := &mmodel.CreateAccountInput{
		Name:      "Pending CRM Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.createAccountWithOptions(ctx, organizationID, ledgerID, input, "Bearer test-token", accountCreateOptions{PendingCRMLink: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if acc == nil {
		t.Fatalf("expected account, got nil")
	}

	if got := acc.Status.Code; got != constant.AccountStatusPendingCRMLink {
		t.Errorf("account.Status.Code: got %q, want %q", got, constant.AccountStatusPendingCRMLink)
	}

	if acc.Blocked == nil || !*acc.Blocked {
		t.Errorf("account.Blocked: got %v, want true", acc.Blocked)
	}

	if capturedAccount == nil {
		t.Fatal("account.Create was not invoked")
	}
	if capturedAccount.Status.Code != constant.AccountStatusPendingCRMLink {
		t.Errorf("persisted account.Status.Code: got %q, want %q", capturedAccount.Status.Code, constant.AccountStatusPendingCRMLink)
	}
	if capturedAccount.Blocked == nil || !*capturedAccount.Blocked {
		t.Errorf("persisted account.Blocked: got %v, want true", capturedAccount.Blocked)
	}

	if capturedBalance == nil {
		t.Fatal("balance.Create was not invoked")
	}
	if capturedBalance.AllowSending {
		t.Error("default balance.AllowSending: got true, want false for pending-CRM-link account")
	}
	if capturedBalance.AllowReceiving {
		t.Error("default balance.AllowReceiving: got true, want false for pending-CRM-link account")
	}
}

// TestCreateAccountDefaultPathRegressionGuard ensures the public CreateAccount
// entry point (PendingCRMLink=false, the only externally exposed path) still
// produces ACTIVE + blocked=false + AllowSending=true + AllowReceiving=true by
// default — i.e., no regression from the Phase 2 refactor.
func TestCreateAccountDefaultPathRegressionGuard(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		PortfolioRepo:          mockPortfolioRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		AccountTypeRepo:        mockAccountTypeRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	var capturedAccount *mmodel.Account
	mockAccountRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			out.ID = uuid.New().String()
			capturedAccount = &out
			return &out, nil
		}).Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	var capturedBalance *mmodel.Balance
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Balance) (*mmodel.Balance, error) {
			out := *in
			capturedBalance = &out
			return &out, nil
		}).Times(1)

	input := &mmodel.CreateAccountInput{
		Name:      "Default Path Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.CreateAccount(ctx, organizationID, ledgerID, input, "Bearer test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if acc == nil {
		t.Fatalf("expected account, got nil")
	}

	if got := acc.Status.Code; got != constant.AccountStatusActive {
		t.Errorf("account.Status.Code: got %q, want %q (regression: default path must land ACTIVE)", got, constant.AccountStatusActive)
	}

	if acc.Blocked == nil {
		t.Fatalf("account.Blocked: got nil, want non-nil pointer to false")
	}
	if *acc.Blocked {
		t.Errorf("account.Blocked: got true, want false (regression: default path must not block)")
	}

	if capturedAccount == nil {
		t.Fatal("account.Create was not invoked")
	}
	if capturedAccount.Status.Code != constant.AccountStatusActive {
		t.Errorf("persisted account.Status.Code: got %q, want %q", capturedAccount.Status.Code, constant.AccountStatusActive)
	}

	if capturedBalance == nil {
		t.Fatal("balance.Create was not invoked")
	}
	if !capturedBalance.AllowSending {
		t.Error("default balance.AllowSending: got false, want true (regression: default path must allow sending)")
	}
	if !capturedBalance.AllowReceiving {
		t.Error("default balance.AllowReceiving: got false, want true (regression: default path must allow receiving)")
	}
}

// TestCreateAccountBlockedFlag ensures the blocked flag is persisted when provided
func TestCreateAccountBlockedFlag(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		PortfolioRepo:          mockPortfolioRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		AccountTypeRepo:        mockAccountTypeRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

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

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

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
