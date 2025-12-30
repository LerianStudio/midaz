package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAllBalancesByAccountID(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := libCommons.GenerateUUIDv7()

	t.Run("list balances error", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("list balances error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return(nil, expectedErr)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("no balances returns nil", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{}, nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})

	t.Run("redis lookup error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("redis error")
		balanceItem := newTestBalance(decimal.NewFromInt(1), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, expectedErr)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("redis balance present prevents deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.NewFromInt(1), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var validationErr midazpkg.ValidationError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
	})

	t.Run("balances with funds remaining prevent deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.NewFromInt(10), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var validationErr midazpkg.ValidationError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
	})

	t.Run("toggle balance transfers error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("update permissions error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		firstCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return expectedErr
			})
		secondCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})
		gomock.InOrder(firstCall, secondCall)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("delete balances error rolls back transfers", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("delete balances error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return nil
			})
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(expectedErr)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("successfully deletes balances", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedID := uuid.MustParse(balanceItem.ID)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return nil
			})
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) error {
				assert.Len(t, ids, 1)
				assert.Equal(t, expectedID, ids[0])
				return nil
			})

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})
}

func TestExtractBalanceIDs_NilElementInSlice_Panics(t *testing.T) {
	uc := &UseCase{}

	balances := []*mmodel.Balance{
		{ID: uuid.New().String()},
		nil,
		{ID: uuid.New().String()},
	}

	assert.Panics(t, func() {
		_ = uc.extractBalanceIDs(balances)
	}, "expected panic when balance slice contains nil element")
}

func setupDeleteAllBalancesUseCase(t *testing.T) (*UseCase, *balance.MockRepository, *redis.MockRedisRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	return &UseCase{
		BalanceRepo: mockBalanceRepo,
		RedisRepo:   mockRedisRepo,
	}, mockBalanceRepo, mockRedisRepo
}

func newTestBalance(available, onHold decimal.Decimal) *mmodel.Balance {
	return &mmodel.Balance{
		ID:        uuid.New().String(),
		Alias:     "alias",
		Key:       "key",
		Available: available,
		OnHold:    onHold,
	}
}

func balanceRedisKey(b *mmodel.Balance) string {
	return fmt.Sprintf("%s#%s", b.Alias, b.Key)
}

func TestToggleBalanceTransfers(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("successfully toggles transfers", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})

		err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true)
		assert.NoError(t, err)
	})

	t.Run("error triggers rollback with opposite permissions", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("update permissions error")

		firstCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return expectedErr
			})
		secondCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})
		gomock.InOrder(firstCall, secondCall)

		err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestUpdateBalanceTransferPermissions(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("successfully updates permissions", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		allow := boolPtr(true)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.Equal(t, allow, update.AllowReceiving)
				assert.Equal(t, allow, update.AllowSending)
				return nil
			})

		err := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allow)
		assert.NoError(t, err)
	})

	t.Run("returns error from repository", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		allow := boolPtr(false)
		expectedErr := errors.New("update permissions error")

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(expectedErr)

		err := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allow)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func boolPtr(v bool) *bool {
	return &v
}
