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

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
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
			redisErr:      errors.New("redis down"),
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
			redisRepo := redis.NewMockRedisRepository(ctrl)

			uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

			balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(base, nil)

			internalKey := utils.BalanceInternalKey(orgID, ledgerID, base.Alias+"#"+base.Key)

			redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(tt.redisResponse, tt.redisErr)

			out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

			assert.NoError(t, err)
			assert.True(t, out.Available.Equal(decimal.RequireFromString(tt.available)), "available should match postgres value")
			assert.True(t, out.OnHold.Equal(decimal.RequireFromString(tt.onHold)), "onHold should match postgres value")
			assert.Equal(t, tt.version, out.Version, "version should match postgres value")
		})
	}
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
