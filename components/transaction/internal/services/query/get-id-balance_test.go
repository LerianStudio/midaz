package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetBalanceByID(t *testing.T) {
	t.Parallel()
	t.Run("SuccessNoCacheOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		bal := &mmodel.Balance{
			ID:             id.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(200),
			Version:        1,
			AccountType:    "checking",
			AllowSending:   true,
			AllowReceiving: true,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(bal, nil)

		internalKey := utils.BalanceInternalKey(orgID, ledgerID, bal.Alias+"#"+bal.Key)

		redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("", nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.NoError(t, err)
		assert.Equal(t, bal, out)
	})

	t.Run("RepoReturnsNilBalance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, nil)

		// Ensure Redis is not called when balance is nil
		redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.Error(t, err)
		var nf pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &nf))
		assert.Nil(t, out)
	})
	t.Run("RedisOverlayApplied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		base := &mmodel.Balance{
			ID:             id.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@alice",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("0"),
			OnHold:         decimal.RequireFromString("0"),
			Version:        1,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

		cached := mmodel.BalanceRedis{
			ID:             id.String(),
			Alias:          base.Alias,
			AccountID:      base.AccountID,
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("123.45"),
			OnHold:         decimal.RequireFromString("6.78"),
			Version:        9,
			AccountType:    "checking",
			AllowSending:   1,
			AllowReceiving: 1,
		}

		b, _ := json.Marshal(cached)

		internalKey := utils.BalanceInternalKey(orgID, ledgerID, base.Alias+"#"+base.Key)

		redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(string(b), nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.NoError(t, err)
		assert.Equal(t, decimal.RequireFromString("123.45"), out.Available)
		assert.Equal(t, decimal.RequireFromString("6.78"), out.OnHold)
		assert.Equal(t, int64(9), out.Version)
	})
	t.Run("RedisErrorShouldNotFail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		base := &mmodel.Balance{
			ID:             id.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@bob",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("10"),
			OnHold:         decimal.RequireFromString("1"),
			Version:        2,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

		internalKey := utils.BalanceInternalKey(orgID, ledgerID, base.Alias+"#"+base.Key)

		redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("", errors.New("redis down"))

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.NoError(t, err)
		assert.True(t, out.Available.Equal(decimal.RequireFromString("10")))
		assert.True(t, out.OnHold.Equal(decimal.RequireFromString("1")))
		assert.Equal(t, int64(2), out.Version)
	})
	t.Run("InvalidCachePayloadSkipsOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		base := &mmodel.Balance{
			ID:             id.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@carol",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("5"),
			OnHold:         decimal.RequireFromString("0"),
			Version:        1,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

		internalKey := utils.BalanceInternalKey(orgID, ledgerID, base.Alias+"#"+base.Key)

		redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("{not-json}", nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.NoError(t, err)
		assert.True(t, out.Available.Equal(decimal.RequireFromString("5")))
		assert.True(t, out.OnHold.Equal(decimal.RequireFromString("0")))
		assert.Equal(t, int64(1), out.Version)
	})
	t.Run("NotFoundReturnsError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, notFoundErr)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.Error(t, err)
		var nf pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &nf))
		assert.Nil(t, out)
	})
	t.Run("RepoErrorPreventsRedisCall", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, context.Canceled)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.Error(t, err)
		assert.Nil(t, out)
	})
}

func TestGetBalanceByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetBalanceByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetBalanceByID_NilBalanceID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil balanceID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "balanceID must not be nil UUID"),
			"panic message should mention balanceID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
