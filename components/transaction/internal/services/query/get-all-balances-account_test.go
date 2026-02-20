package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllBalancesByAccountID validates repository delegation, Redis overlay, and error handling.
func TestGetAllBalancesByAccountID(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()

	// Success without Redis overlay: values should remain unchanged
	t.Run("SuccessNoRedisOverlay", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		item := &mmodel.Balance{
			Alias:     "@user",
			Key:       "k1",
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(2),
			Version:   1,
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{item}, mockCur, nil).
			Times(1)

		expectedKey := utils.BalanceInternalKey(organizationID, ledgerID, item.Alias+"#"+item.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Eq([]string{expectedKey})).
			Return(map[string]string{}, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo, RedisRepo: mockRedisRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(10)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(2)))
		assert.Equal(t, int64(1), res[0].Version)
	})

	// Success with Redis overlay: values should be updated from cache
	t.Run("SuccessWithRedisOverlay", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		item := &mmodel.Balance{
			Alias:     "@user",
			Key:       "k2",
			Available: decimal.NewFromInt(1),
			OnHold:    decimal.NewFromInt(1),
			Version:   1,
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{item}, mockCur, nil).
			Times(1)

		expectedKey := utils.BalanceInternalKey(organizationID, ledgerID, item.Alias+"#"+item.Key)
		cachePayload := `{"available":"123.45","onHold":"6","version":42}`
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Eq([]string{expectedKey})).
			Return(map[string]string{expectedKey: cachePayload}, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo, RedisRepo: mockRedisRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromFloat(123.45)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(6)))
		assert.Equal(t, int64(42), res[0].Version)
	})

	// Empty result should early-return and avoid Redis calls
	t.Run("EmptyResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{}, mockCur, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 0)
	})

	// Repository not found should map to business error
	t.Run("RepoItemNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	// Generic repository error should be propagated
	t.Run("RepoGenericError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		errDB := errors.New("database error")
		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errDB).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "database error")
	})

	// Redis error should not fail; values remain from repository
	t.Run("RedisErrorShouldNotFail", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		item := &mmodel.Balance{
			Alias:     "@user",
			Key:       "k3",
			Available: decimal.NewFromInt(7),
			OnHold:    decimal.NewFromInt(3),
			Version:   5,
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{item}, mockCur, nil).
			Times(1)

		expectedKey := utils.BalanceInternalKey(organizationID, ledgerID, item.Alias+"#"+item.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Eq([]string{expectedKey})).
			Return(nil, errors.New("redis error")).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo, RedisRepo: mockRedisRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(7)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(3)))
		assert.Equal(t, int64(5), res[0].Version)
	})

	t.Run("InvalidCachePayloadSkipsOverlay", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		item := &mmodel.Balance{
			Alias:     "@user",
			Key:       "k4",
			Available: decimal.NewFromInt(3),
			OnHold:    decimal.NewFromInt(1),
			Version:   0,
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{item}, mockCur, nil).
			Times(1)

		expectedKey := utils.BalanceInternalKey(organizationID, ledgerID, item.Alias+"#"+item.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Eq([]string{expectedKey})).
			Return(map[string]string{expectedKey: "{not-json}"}, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo, RedisRepo: mockRedisRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 1)
		// Values remain as from repository (overlay skipped)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(3)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(1)))
		assert.Equal(t, int64(0), res[0].Version)
	})
}
