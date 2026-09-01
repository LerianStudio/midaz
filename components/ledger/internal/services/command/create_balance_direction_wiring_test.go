// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountBalanceDirectionWiring covers Task 1.3.2 for the INITIAL
// balance path (CreateAccount -> CreateDefaultBalance): the account type's
// DefaultDirection is resolved once (best-effort) and flows through the
// resolver into the default balance's Direction.
func TestCreateAccountBalanceDirectionWiring(t *testing.T) {
	t.Parallel()

	setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *account.MockRepository, *mongodb.MockRepository, *accounttype.MockRepository, *balance.MockRepository, *ledger.MockRepository) {
		mockAssetRepo := asset.NewMockRepository(ctrl)
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRepo:              mockAssetRepo,
			PortfolioRepo:          mockPortfolioRepo,
			AccountRepo:            mockAccountRepo,
			OnboardingMetadataRepo: mockMetadataRepo,
			AccountTypeRepo:        mockAccountTypeRepo,
			BalanceRepo:            mockBalanceRepo,
			LedgerRepo:             mockLedgerRepo,
		}

		return uc, mockAssetRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceRepo, mockLedgerRepo
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	externalAlias := "@external/USD"

	tests := []struct {
		name              string
		input             *mmodel.CreateAccountInput
		typeLookup        func(mockAccountTypeRepo *accounttype.MockRepository)
		expectedDirection string
	}{
		{
			name: "type default debit -> initial balance debit",
			input: &mmodel.CreateAccountInput{
				Name:      "Debit Account",
				Type:      "loan",
				AssetCode: "USD",
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "loan").
					Return(&mmodel.AccountType{KeyValue: "loan", DefaultDirection: constant.DirectionDebit}, nil).
					Times(1)
			},
			expectedDirection: constant.DirectionDebit,
		},
		{
			name: "type default credit -> initial balance credit",
			input: &mmodel.CreateAccountInput{
				Name:      "Credit Account",
				Type:      "deposit",
				AssetCode: "USD",
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(&mmodel.AccountType{KeyValue: "deposit", DefaultDirection: constant.DirectionCredit}, nil).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name: "type with no default -> non-external falls back to credit",
			input: &mmodel.CreateAccountInput{
				Name:      "No-Default Account",
				Type:      "savings",
				AssetCode: "USD",
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "savings").
					Return(&mmodel.AccountType{KeyValue: "savings", DefaultDirection: ""}, nil).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name: "type lookup not-found -> graceful fallback to credit, no failure",
			input: &mmodel.CreateAccountInput{
				Name:      "Missing-Type Account",
				Type:      "unregistered",
				AssetCode: "USD",
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "unregistered").
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name: "type lookup technical error -> graceful fallback to credit, no failure",
			input: &mmodel.CreateAccountInput{
				Name:      "Errored-Type Account",
				Type:      "flaky",
				AssetCode: "USD",
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "flaky").
					Return(nil, errors.New("database connection error")).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name: "external account -> no type lookup, debit via bypass",
			input: &mmodel.CreateAccountInput{
				Name:      "External Account",
				Type:      "external",
				AssetCode: "USD",
				Alias:     &externalAlias,
			},
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			expectedDirection: constant.DirectionDebit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockAssetRepo, mockAccountRepo, mockMetadataRepo, mockAccountTypeRepo, mockBalanceRepo, mockLedgerRepo := setupTest(ctrl)

			// Validation is OFF: the type default must still be resolved.
			mockLedgerRepo.EXPECT().
				GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, nil).AnyTimes()

			tt.typeLookup(mockAccountTypeRepo)

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
				DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
					assert.Equal(t, tt.expectedDirection, b.Direction, "default balance direction")
					return b, nil
				}).
				Times(1)

			acc, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, "Bearer test-token", HolderOnV2)

			assert.NoError(t, err)
			assert.NotNil(t, acc)
		})
	}
}

// TestCreateAdditionalBalanceDirectionWiring covers Task 1.3.2 for the
// ADDITIONAL balance path: the caller override wins (RF-03); without an
// override the account type's DefaultDirection is inherited (RF-02); a
// non-external type with no default falls back to credit; a type-lookup
// miss/error degrades gracefully to credit without failing the request.
func TestCreateAdditionalBalanceDirectionWiring(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	setupTest := func(ctrl *gomock.Controller) (*UseCase, *balance.MockRepository, *accounttype.MockRepository) {
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			AccountTypeRepo: mockAccountTypeRepo,
		}

		return uc, mockBalanceRepo, mockAccountTypeRepo
	}

	defaultBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          "test-alias",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	tests := []struct {
		name              string
		direction         *string
		typeLookup        func(mockAccountTypeRepo *accounttype.MockRepository)
		expectedDirection string
	}{
		{
			name:      "explicit override wins over type default",
			direction: testutils.Ptr(constant.DirectionDebit),
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				// An explicit override wins without inheriting the type default,
				// so the lookup must be skipped entirely.
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:      "no override inherits type default debit",
			direction: nil,
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(&mmodel.AccountType{KeyValue: "deposit", DefaultDirection: constant.DirectionDebit}, nil).
					Times(1)
			},
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:      "no override, type has no default -> credit fallback",
			direction: nil,
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(&mmodel.AccountType{KeyValue: "deposit", DefaultDirection: ""}, nil).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:      "no override, type lookup miss -> credit fallback, no failure",
			direction: nil,
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:      "no override, type lookup technical error -> credit fallback, no failure",
			direction: nil,
			typeLookup: func(mockAccountTypeRepo *accounttype.MockRepository) {
				mockAccountTypeRepo.EXPECT().
					FindByKey(gomock.Any(), organizationID, ledgerID, "deposit").
					Return(nil, errors.New("database connection error")).
					Times(1)
			},
			expectedDirection: constant.DirectionCredit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockBalanceRepo, mockAccountTypeRepo := setupTest(ctrl)

			cbi := &mmodel.CreateAdditionalBalance{
				Key:       "asset-freeze",
				Direction: tt.direction,
			}

			mockBalanceRepo.EXPECT().
				FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "asset-freeze").
				Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
				Times(1)

			mockBalanceRepo.EXPECT().
				FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
				Return(defaultBalance, nil).
				Times(1)

			tt.typeLookup(mockAccountTypeRepo)

			mockBalanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
					assert.Equal(t, tt.expectedDirection, b.Direction, "additional balance direction")
					return b, nil
				}).
				Times(1)

			result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
