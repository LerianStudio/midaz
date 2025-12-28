package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	t.Run("success - delete balance with zero funds", func(t *testing.T) {
		balanceEntity := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.Zero,
			OnHold:         decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceEntity, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.NoError(t, err)
	})

	t.Run("success - delete balance when balance is nil (not found)", func(t *testing.T) {
		newBalanceID := libCommons.GenerateUUIDv7()

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, newBalanceID).
			Return(nil, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, newBalanceID).
			Return(nil).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, newBalanceID)

		require.NoError(t, err)
	})

	t.Run("error - balance has available funds", func(t *testing.T) {
		balanceWithFunds := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceWithFunds, nil).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var validationErr pkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
	})

	t.Run("error - balance has on hold funds", func(t *testing.T) {
		balanceWithOnHold := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.Zero,
			OnHold:         decimal.NewFromInt(500),
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceWithOnHold, nil).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var validationErr pkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
	})

	t.Run("error - balance has both available and on hold funds", func(t *testing.T) {
		balanceWithBothFunds := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(500),
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceWithBothFunds, nil).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var validationErr pkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
	})

	t.Run("error - find balance fails", func(t *testing.T) {
		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, errors.New("database connection error")).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("error - delete balance fails", func(t *testing.T) {
		balanceEntity := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.Zero,
			OnHold:         decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceEntity, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(errors.New("delete operation failed")).
			Times(1)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("error - delete balance with negative available (edge case)", func(t *testing.T) {
		// Some systems might allow negative balances
		balanceWithNegative := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(-100), // Negative balance
			OnHold:         decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceWithNegative, nil).
			Times(1)

		// Negative values are not zero, so should be blocked
		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var validationErr pkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
	})

	t.Run("error - delete balance with very small decimal amounts", func(t *testing.T) {
		// Test with very small decimal amounts that should be considered non-zero
		smallAmount, _ := decimal.NewFromString("0.00000001")
		balanceWithSmallAmount := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			AssetCode:      "BTC",
			Available:      smallAmount,
			OnHold:         decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(balanceWithSmallAmount, nil).
			Times(1)

		// Even very small amounts should block deletion
		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		require.Error(t, err)
		var validationErr pkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
	})
}

// TestDeleteBalanceSuccess is responsible to test DeleteBalanceSuccess with success
func TestDeleteBalanceSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(nil).
		Times(1)
	err := uc.BalanceRepo.Delete(context.TODO(), organizationID, ledgerID, balanceID)

	assert.Nil(t, err)
}

// TestDeleteBalanceError is responsible to test DeleteBalanceError with error
func TestDeleteBalanceError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(errors.New(errMSG))
	err := uc.BalanceRepo.Delete(context.TODO(), organizationID, ledgerID, balanceID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
