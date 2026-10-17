// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestGetBalanceByID(t *testing.T) {
	t.Parallel()
	t.Run("RepoReturnsNilBalance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := uuid.Must(libCommons.GenerateUUIDv7())
		orgID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, nil)

		// Ensure Redis is not called when balance is nil
		redisRepo.EXPECT().MGet(gomock.Any(), gomock.Any()).Times(0)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.Error(t, err)
		var nf pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &nf))
		assert.Nil(t, out)
	})
	t.Run("RedisOverlayApplied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := uuid.Must(libCommons.GenerateUUIDv7())
		orgID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

		base := &mmodel.Balance{
			ID:             id.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:          "@alice",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("0"),
			OnHold:         decimal.RequireFromString("0"),
			Version:        1,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}

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

		redisRepo.EXPECT().MGet(gomock.Any(), []string{internalKey}).Return(map[string]string{internalKey: string(b)}, nil)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.NoError(t, err)
		assert.Equal(t, decimal.RequireFromString("123.45"), out.Available)
		assert.Equal(t, decimal.RequireFromString("6.78"), out.OnHold)
		assert.Equal(t, int64(9), out.Version)
	})
	t.Run("NotFoundReturnsError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		id := uuid.Must(libCommons.GenerateUUIDv7())
		orgID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}

		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)

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

		id := uuid.Must(libCommons.GenerateUUIDv7())
		orgID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

		balanceRepo := balance.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}

		balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, context.Canceled)

		out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

		assert.Error(t, err)
		assert.Nil(t, out)
	})
}
