package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteBalance(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	t.Run("find balance error", func(t *testing.T) {
		uc, mockBalanceRepo := setupDeleteBalanceUseCase(t)
		expectedErr := errors.New("database connection error")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, expectedErr)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("balance with funds cannot be deleted", func(t *testing.T) {
		cases := []struct {
			name      string
			available decimal.Decimal
			onHold    decimal.Decimal
		}{
			{"available only", decimal.NewFromInt(100), decimal.Zero},
			{"on-hold only", decimal.Zero, decimal.NewFromInt(50)},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				uc, mockBalanceRepo := setupDeleteBalanceUseCase(t)
				bal := &mmodel.Balance{
					ID:        balanceID.String(),
					Available: tc.available,
					OnHold:    tc.onHold,
				}

				mockBalanceRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, balanceID).
					Return(bal, nil)

				err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

				var validationErr midazpkg.ValidationError
				assert.True(t, errors.As(err, &validationErr))
				assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
			})
		}
	})

	t.Run("delete error", func(t *testing.T) {
		uc, mockBalanceRepo := setupDeleteBalanceUseCase(t)
		zeroBalance := &mmodel.Balance{
			ID:        balanceID.String(),
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		expectedErr := errors.New("delete failed")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(zeroBalance, nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(expectedErr)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("nil balance proceeds to delete", func(t *testing.T) {
		uc, mockBalanceRepo := setupDeleteBalanceUseCase(t)

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.NoError(t, err)
	})

	t.Run("deletes balance with zero funds", func(t *testing.T) {
		uc, mockBalanceRepo := setupDeleteBalanceUseCase(t)
		zeroBalance := &mmodel.Balance{
			ID:        balanceID.String(),
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(zeroBalance, nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.NoError(t, err)
	})
}

func setupDeleteBalanceUseCase(t *testing.T) (*UseCase, *balance.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	return &UseCase{
		BalanceRepo: mockBalanceRepo,
	}, mockBalanceRepo
}
