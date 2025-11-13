package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAllBalancesByAccountIDListAllError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return(nil, errors.New("list error")).
		Times(1)

	uc := UseCase{
		RedisRepo: mockRedis,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.Error(t, err)
}

func TestDeleteAllBalancesByAccountIDBalanceWithFunds(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{
			{
				ID:        balanceID.String(),
				Alias:     "alias",
				Key:       "key",
				Available: decimal.NewFromInt(1),
			},
		}, nil).
		Times(1)

	uc := UseCase{
		RedisRepo: mockRedis,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.Error(t, err)

	var validationErr pkg.ValidationError

	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
	assert.Equal(t, "DeleteAllBalancesByAccountID", validationErr.EntityType)
}

func TestDeleteAllBalancesByAccountIDUpdateBalanceTransferPermissionsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{
			{
				ID:        balanceID.String(),
				Alias:     "alias",
				Key:       "key",
				Available: decimal.Zero,
				OnHold:    decimal.Zero,
			},
		}, nil).
		Times(1)

	updateErr := errors.New("update error")

	mockBalance := balance.NewMockRepository(ctrl)
	firstCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.NotNil(t, update.AllowReceiving)
			assert.NotNil(t, update.AllowSending)
			assert.False(t, *update.AllowReceiving)
			assert.False(t, *update.AllowSending)

			return updateErr
		})

	secondCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.NotNil(t, update.AllowReceiving)
			assert.NotNil(t, update.AllowSending)
			assert.True(t, *update.AllowReceiving)
			assert.True(t, *update.AllowSending)

			return nil
		})

	gomock.InOrder(firstCall, secondCall)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.ErrorIs(t, err, updateErr)
}

func TestDeleteAllBalancesByAccountIDRedisDeleteError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, "alias#key")
	delErr := errors.New("del error")

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{
			{
				ID:        balanceID.String(),
				Alias:     "alias",
				Key:       "key",
				Available: decimal.Zero,
				OnHold:    decimal.Zero,
			},
		}, nil).
		Times(1)
	mockRedis.
		EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(delErr).
		Times(1)

	mockBalance := balance.NewMockRepository(ctrl)
	firstCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.False(t, *update.AllowReceiving)
			assert.False(t, *update.AllowSending)

			return nil
		})

	secondCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.True(t, *update.AllowReceiving)
			assert.True(t, *update.AllowSending)

			return nil
		})

	gomock.InOrder(firstCall, secondCall)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.ErrorIs(t, err, delErr)
}

func TestDeleteAllBalancesByAccountIDBalanceDeleteError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, "alias#key")
	deleteErr := errors.New("delete error")

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{
			{
				ID:        balanceID.String(),
				Alias:     "alias",
				Key:       "key",
				Available: decimal.Zero,
				OnHold:    decimal.Zero,
			},
		}, nil).
		Times(1)
	mockRedis.
		EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(nil).
		Times(1)

	mockBalance := balance.NewMockRepository(ctrl)
	firstCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.False(t, *update.AllowReceiving)
			assert.False(t, *update.AllowSending)

			return nil
		})
	mockBalance.
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(deleteErr).
		Times(1)

	secondCall := mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.True(t, *update.AllowReceiving)
			assert.True(t, *update.AllowSending)

			return nil
		})

	gomock.InOrder(firstCall, secondCall)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.ErrorIs(t, err, deleteErr)
}

func TestDeleteAllBalancesByAccountIDSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, "alias#key")

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{
			{
				ID:        balanceID.String(),
				Alias:     "alias",
				Key:       "key",
				Available: decimal.Zero,
				OnHold:    decimal.Zero,
			},
		}, nil).
		Times(1)
	mockRedis.
		EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(nil).
		Times(1)

	mockBalance := balance.NewMockRepository(ctrl)
	mockBalance.
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, update mmodel.UpdateBalance) error {
			assert.False(t, *update.AllowReceiving)
			assert.False(t, *update.AllowSending)

			return nil
		}).
		Times(1)
	mockBalance.
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(nil).
		Times(1)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID)

	assert.NoError(t, err)
}
