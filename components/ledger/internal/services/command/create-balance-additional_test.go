package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateAdditionalBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "test-alias"

	// Create default balance for reference
	defaultBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          alias,
		Key:            "default",
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	t.Run("success", func(t *testing.T) {
		allowSending := true
		allowReceiving := false
		key := "asset-freeze"

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// First lookup for existing additional balance should return EntityNotFound to continue flow
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "asset-freeze").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
			Times(1)

		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(defaultBalance, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, alias, b.Alias)
				assert.Equal(t, "asset-freeze", b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "USD", b.AssetCode)
				assert.Equal(t, "deposit", b.AccountType)
				assert.True(t, b.AllowSending)
				assert.False(t, b.AllowReceiving)
				return nil
			}).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, alias, result.Alias)
		assert.Equal(t, "asset-freeze", result.Key)
		assert.True(t, result.AllowSending)
		assert.False(t, result.AllowReceiving)
	})

	t.Run("failed to get default balance", func(t *testing.T) {
		allowSending := true
		allowReceiving := true
		key := "test-key"

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// First lookup for existing additional balance should return EntityNotFound to continue flow
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "test-key").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
			Times(1)

		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(nil, errors.New("default balance not found")).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, "default balance not found", err.Error())
	})

	t.Run("additional balance already exists", func(t *testing.T) {
		allowSending := false
		allowReceiving := true
		key := "duplicate-key"

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// Simulate that the additional balance already exists
		existingAdditional := &mmodel.Balance{Key: key}
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "duplicate-key").
			Return(existingAdditional, nil).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "An account alias with the specified key value already exists")
	})

	t.Run("error creating additional balance", func(t *testing.T) {
		allowSending := true
		allowReceiving := true
		key := "test-key"

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// First lookup for existing additional balance should return EntityNotFound to continue flow
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "test-key").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
			Times(1)

		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(defaultBalance, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(errors.New("database error")).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, "database error", err.Error())
	})

	t.Run("key is converted to lowercase", func(t *testing.T) {
		allowSending := true
		allowReceiving := true
		key := "UPPER-CASE-KEY"

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// First lookup for existing additional balance should return EntityNotFound to continue flow
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "upper-case-key").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
			Times(1)

		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(defaultBalance, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, "upper-case-key", b.Key)
				return nil
			}).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "upper-case-key", result.Key)
	})

	t.Run("external account type not allowed", func(t *testing.T) {
		allowSending := true
		allowReceiving := true
		key := "test-key"

		externalBalance := &mmodel.Balance{
			ID:             uuid.New().String(),
			Alias:          alias,
			Key:            constant.DefaultBalanceKey,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			AssetCode:      "USD",
			AccountType:    constant.ExternalAccountType,
			AllowSending:   true,
			AllowReceiving: true,
		}

		cbi := &mmodel.CreateAdditionalBalance{
			Key:            key,
			AllowSending:   &allowSending,
			AllowReceiving: &allowReceiving,
		}

		// First lookup for existing additional balance should return EntityNotFound to continue flow
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "test-key").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
			Times(1)

		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(externalBalance, nil).
			Times(1)

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, cbi)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Additional balances are not allowed for external account type")
	})
}
