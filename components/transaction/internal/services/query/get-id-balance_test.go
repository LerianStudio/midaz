// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	redisAdapter "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// shardLookupFailRedisClient is a focused test stub that only implements HGet.
// If production code calls other Redis methods, the embedded nil interface will
// panic, signaling that this stub needs to be extended for the new code path.
type shardLookupFailRedisClient struct {
	goredis.UniversalClient
}

func (f *shardLookupFailRedisClient) HGet(_ context.Context, _, _ string) *goredis.StringCmd {
	return goredis.NewStringResult("", errors.New("forced shard lookup failure")) //nolint:err113
}

func TestGetBalanceByID(t *testing.T) { //nolint:funlen
	t.Parallel()
	t.Run("SuccessNoCacheOverlay", func(t *testing.T) {
		t.Parallel()

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
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(bal, nil)

		internalKey := utils.BalanceInternalKey(orgID, ledgerID, bal.Alias+"#"+bal.Key)

		redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("", nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.NoError(t, err)
		assert.Equal(t, bal, out)
	})

	t.Run("RepoReturnsNilBalance", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, nil)

		// Ensure Redis is not called when balance is nil
		redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.Error(t, err)

		var nf pkg.EntityNotFoundError
		require.ErrorAs(t, err, &nf)
		assert.Nil(t, out)
	})
	t.Run("RedisOverlayApplied", func(t *testing.T) {
		t.Parallel()

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
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

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

		require.NoError(t, err)
		assert.Equal(t, decimal.RequireFromString("123.45"), out.Available)
		assert.Equal(t, decimal.RequireFromString("6.78"), out.OnHold)
		assert.Equal(t, int64(9), out.Version)
	})

	t.Run("RedisOverlayAppliedUsesShardKeyWhenShardRouterConfigured", func(t *testing.T) {
		t.Parallel()

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
			Alias:          "@sharded-user",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("1"),
			OnHold:         decimal.RequireFromString("2"),
			Version:        3,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		router := shard.NewRouter(8)
		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo, ShardRouter: router}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

		cached := mmodel.BalanceRedis{
			ID:             id.String(),
			Alias:          base.Alias,
			AccountID:      base.AccountID,
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("999.99"),
			OnHold:         decimal.RequireFromString("0.01"),
			Version:        17,
			AccountType:    "checking",
			AllowSending:   1,
			AllowReceiving: 1,
		}

		payload, marshalErr := json.Marshal(cached)
		require.NoError(t, marshalErr)

		shardID := router.Resolve(base.Alias)
		shardKey := utils.BalanceShardKey(shardID, orgID, ledgerID, base.Alias+"#"+base.Key)

		redisRepo.EXPECT().Get(gomock.Any(), shardKey).Return(string(payload), nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.NoError(t, err)
		assert.True(t, out.Available.Equal(decimal.RequireFromString("999.99")))
		assert.True(t, out.OnHold.Equal(decimal.RequireFromString("0.01")))
		assert.Equal(t, int64(17), out.Version)
	})

	t.Run("ShardResolutionErrorFallsBackToDeterministicShard", func(t *testing.T) {
		t.Parallel()

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
			Alias:          "@sharded-user",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("1"),
			OnHold:         decimal.RequireFromString("2"),
			Version:        3,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		router := shard.NewRouter(8)
		shardManager := internalsharding.NewManager(&libRedis.RedisConnection{Client: &shardLookupFailRedisClient{}}, router, nil, internalsharding.Config{})
		require.NotNil(t, shardManager)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo, ShardRouter: router, ShardManager: shardManager}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)
		fallbackShardID := router.Resolve(base.Alias)
		fallbackKey := utils.BalanceShardKey(fallbackShardID, orgID, ledgerID, base.Alias+"#"+base.Key)
		redisRepo.EXPECT().Get(gomock.Any(), fallbackKey).Return("", errors.New("redis unavailable")) //nolint:err113

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.NoError(t, err)
		require.NotNil(t, out)
		assert.Equal(t, base.ID, out.ID)
	})

	// Table-driven: graceful degradation when Redis fails or returns invalid data
	gracefulDegradationTests := []struct {
		name          string
		alias         string
		available     string
		onHold        string
		version       int64
		redisResponse string
		redisErr      error
	}{
		{
			name:          "RedisErrorShouldNotFail",
			alias:         "@bob",
			available:     "10",
			onHold:        "1",
			version:       2,
			redisResponse: "",
			redisErr:      errors.New("redis down"), //nolint:err113
		},
		{
			name:          "InvalidCachePayloadSkipsOverlay",
			alias:         "@carol",
			available:     "5",
			onHold:        "0",
			version:       1,
			redisResponse: "{not-json}",
			redisErr:      nil,
		},
	}

	for _, tt := range gracefulDegradationTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
				Alias:          tt.alias,
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.RequireFromString(tt.available),
				OnHold:         decimal.RequireFromString(tt.onHold),
				Version:        tt.version,
			}

			balanceRepo := balance.NewMockRepository(ctrl)
			redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

			uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

			balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

			internalKey := utils.BalanceInternalKey(orgID, ledgerID, base.Alias+"#"+base.Key)

			redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(tt.redisResponse, tt.redisErr)

			out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

			require.NoError(t, err)
			assert.True(t, out.Available.Equal(decimal.RequireFromString(tt.available)), "available should match postgres value")
			assert.True(t, out.OnHold.Equal(decimal.RequireFromString(tt.onHold)), "onHold should match postgres value")
			assert.Equal(t, tt.version, out.Version, "version should match postgres value")
		})
	}

	t.Run("NotFoundReturnsError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, notFoundErr)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.Error(t, err)

		var nf pkg.EntityNotFoundError
		require.ErrorAs(t, err, &nf)
		assert.Nil(t, out)
	})
	t.Run("RepoErrorPreventsRedisCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := libCommons.GenerateUUIDv7()
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redisAdapter.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, context.Canceled)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		require.Error(t, err)
		assert.Nil(t, out)
	})
}
