package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllBalances(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:        10,
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()
	t.Run("Success_no_cache_overlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias",
			Key:       "k1",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(100),
			OnHold:    decimal.NewFromInt(10),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), bal.Alias+"#"+bal.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(100)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(10)))
	})

	t.Run("Success_with_cache_overlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias2",
			Key:       "k2",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(1),
			OnHold:    decimal.NewFromInt(2),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), bal.Alias+"#"+bal.Key)
		cached := mmodel.BalanceRedis{
			Available: decimal.NewFromInt(999),
			OnHold:    decimal.NewFromInt(777),
		}
		data, _ := json.Marshal(cached)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: string(data)}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(999)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(777)))
	})

	t.Run("Redis_error_should_not_fail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias3",
			Key:       "k3",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(5),
			OnHold:    decimal.NewFromInt(6),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), bal.Alias+"#"+bal.Key)
		_ = key // key construction is deterministic; expectation uses Any to avoid tight coupling
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("redis down")).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(5)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(6)))
	})

	t.Run("Repo_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		errMsg := "errDatabaseItemNotFound"
		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New(errMsg)).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("No_balances_found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{}, mockCur, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})
}

func TestGetAllBalancesByAlias(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "test-alias"

	t.Parallel()

	t.Run("GetAllBalancesByAlias_success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-1",
				AccountID: "account-id-1",
				Alias:     alias,
				AssetCode: "BRL",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("GetAllBalancesByAlias_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		errMsg := "error getting balances"

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(nil, errors.New(errMsg)).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.Error(t, err)
		assert.Equal(t, errMsg, err.Error())
		assert.Nil(t, res)
	})
}
