package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAllBalancesByAccountID(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := UseCase{
			RedisRepo:   mockRedisRepo,
			BalanceRepo: mockBalanceRepo,
		}

		targetBalance := &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: accountID.String(),
			Alias:     "@alias",
			Key:       "default",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, fmt.Sprintf("%s#%s", targetBalance.Alias, targetBalance.Key))

		listCall := mockRedisRepo.
			EXPECT().
			ListAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{targetBalance}, nil).
			Times(1)

		getCall := mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), cacheKey).
			Return("", nil).
			After(listCall).
			Times(1)

		updateDisableCall := mockBalanceRepo.
			EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, targetBalance.IDtoUUID(), gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
			DoAndReturn(func(ctx context.Context, orgID, ledID, balID uuid.UUID, payload mmodel.UpdateBalance) error {
				assert.NotNil(t, payload.AllowReceiving)
				assert.False(t, *payload.AllowReceiving)
				assert.NotNil(t, payload.AllowSending)
				assert.False(t, *payload.AllowSending)
				return nil
			}).
			After(getCall).
			Times(1)

		delCall := mockRedisRepo.
			EXPECT().
			Del(gomock.Any(), cacheKey).
			Return(nil).
			After(updateDisableCall).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, []uuid.UUID{targetBalance.IDtoUUID()}).
			Return(nil).
			After(delCall).
			Times(1)

		err := uc.DeleteAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID)

		assert.NoError(t, err)
	})

	t.Run("ListAllBalancesError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := UseCase{
			RedisRepo:   mockRedisRepo,
			BalanceRepo: mockBalanceRepo,
		}

		expectedErr := errors.New("redis unavailable")

		mockRedisRepo.
			EXPECT().
			ListAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return(nil, expectedErr).
			Times(1)

		err := uc.DeleteAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("BalanceWithFundsReturnsError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := UseCase{
			RedisRepo:   mockRedisRepo,
			BalanceRepo: mockBalanceRepo,
		}

		targetBalance := &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: accountID.String(),
			Alias:     "@alias",
			Key:       "default",
			Available: decimal.NewFromInt(1),
			OnHold:    decimal.Zero,
		}

		mockRedisRepo.
			EXPECT().
			ListAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{targetBalance}, nil).
			Times(1)

		err := uc.DeleteAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrBalancesCantBeDeleted.Error())
	})

	t.Run("RedisGetError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := UseCase{
			RedisRepo:   mockRedisRepo,
			BalanceRepo: mockBalanceRepo,
		}

		targetBalance := &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: accountID.String(),
			Alias:     "@alias",
			Key:       "default",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, fmt.Sprintf("%s#%s", targetBalance.Alias, targetBalance.Key))
		expectedErr := errors.New("redis get failure")

		listCall := mockRedisRepo.
			EXPECT().
			ListAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{targetBalance}, nil).
			Times(1)

		mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), cacheKey).
			Return("", expectedErr).
			After(listCall).
			Times(1)

		err := uc.DeleteAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("RedisDeleteErrorRestoresCacheAndReenablesTransfers", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := UseCase{
			RedisRepo:   mockRedisRepo,
			BalanceRepo: mockBalanceRepo,
		}

		balanceA := &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: accountID.String(),
			Alias:     "@a",
			Key:       "k1",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		balanceB := &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: accountID.String(),
			Alias:     "@b",
			Key:       "k2",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		keyA := utils.BalanceInternalKey(organizationID, ledgerID, fmt.Sprintf("%s#%s", balanceA.Alias, balanceA.Key))
		keyB := utils.BalanceInternalKey(organizationID, ledgerID, fmt.Sprintf("%s#%s", balanceB.Alias, balanceB.Key))
		delErr := errors.New("redis delete failure")

		listCall := mockRedisRepo.
			EXPECT().
			ListAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceA, balanceB}, nil).
			Times(1)

		getACall := mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), keyA).
			Return("cached-a", nil).
			After(listCall).
			Times(1)

		getBCall := mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), keyB).
			Return("", nil).
			After(getACall).
			Times(1)

		updateDisableA := mockBalanceRepo.
			EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, balanceA.IDtoUUID(), gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
			DoAndReturn(func(ctx context.Context, orgID, ledID, balID uuid.UUID, payload mmodel.UpdateBalance) error {
				assert.NotNil(t, payload.AllowReceiving)
				assert.False(t, *payload.AllowReceiving)
				assert.NotNil(t, payload.AllowSending)
				assert.False(t, *payload.AllowSending)
				return nil
			}).
			After(getBCall).
			Times(1)

		updateDisableB := mockBalanceRepo.
			EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, balanceB.IDtoUUID(), gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
			DoAndReturn(func(ctx context.Context, orgID, ledID, balID uuid.UUID, payload mmodel.UpdateBalance) error {
				assert.NotNil(t, payload.AllowReceiving)
				assert.False(t, *payload.AllowReceiving)
				assert.NotNil(t, payload.AllowSending)
				assert.False(t, *payload.AllowSending)
				return nil
			}).
			After(updateDisableA).
			Times(1)

		delACall := mockRedisRepo.
			EXPECT().
			Del(gomock.Any(), keyA).
			Return(nil).
			After(updateDisableB).
			Times(1)

		delBCall := mockRedisRepo.
			EXPECT().
			Del(gomock.Any(), keyB).
			Return(delErr).
			After(delACall).
			Times(1)

		setRestore := mockRedisRepo.
			EXPECT().
			Set(gomock.Any(), keyA, "cached-a", time.Duration(0)).
			Return(nil).
			After(delBCall).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, balanceA.IDtoUUID(), gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
			DoAndReturn(func(ctx context.Context, orgID, ledID, balID uuid.UUID, payload mmodel.UpdateBalance) error {
				assert.NotNil(t, payload.AllowReceiving)
				assert.True(t, *payload.AllowReceiving)
				assert.NotNil(t, payload.AllowSending)
				assert.True(t, *payload.AllowSending)
				return nil
			}).
			After(setRestore).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, balanceB.IDtoUUID(), gomock.AssignableToTypeOf(mmodel.UpdateBalance{})).
			DoAndReturn(func(ctx context.Context, orgID, ledID, balID uuid.UUID, payload mmodel.UpdateBalance) error {
				assert.NotNil(t, payload.AllowReceiving)
				assert.True(t, *payload.AllowReceiving)
				assert.NotNil(t, payload.AllowSending)
				assert.True(t, *payload.AllowSending)
				return nil
			}).
			After(setRestore).
			Times(1)

		err := uc.DeleteAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID)

		assert.Error(t, err)
		assert.Equal(t, delErr, err)
	})
}
