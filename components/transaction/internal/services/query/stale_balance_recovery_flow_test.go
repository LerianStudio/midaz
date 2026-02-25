// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRecoverLaggedBalancesForAliases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	aliasWithKey := "@alice#default"
	router := shard.NewRouter(8)
	partition := int32(router.ResolveBalance("@alice", "default"))

	t.Run("returns original aliases when partition is caught up", func(t *testing.T) {
		uc := &UseCase{
			ConsumerLagFenceEnabled: true,
			LagChecker:              &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: true}},
			ShardRouter:             router,
			BalanceOperationsTopic:  "ledger.balance.operations",
			StaleBalanceRecoverer:   &stubStaleBalanceRecoverer{},
		}

		recovered, remaining, err := uc.recoverLaggedBalancesForAliases(ctx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err)
		assert.Empty(t, recovered)
		assert.Equal(t, []string{aliasWithKey}, remaining)
	})

	t.Run("recovers lagged alias and caches it", func(t *testing.T) {
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		recoveredBalance := &mmodel.Balance{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Alias:          "@alice",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(900),
			OnHold:         decimal.Zero,
			Version:        2,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		expectedKey := utils.BalanceShardKey(int(partition), orgID, ledgerID, aliasWithKey)
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), expectedKey, gomock.Any(), 15*time.Second).
			Return(nil).
			Times(1)

		uc := &UseCase{
			RedisRepo:               mockRedisRepo,
			ConsumerLagFenceEnabled: true,
			LagChecker:              &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}},
			ShardRouter:             router,
			BalanceOperationsTopic:  "ledger.balance.operations",
			BalanceCacheTTL:         15 * time.Second,
			StaleBalanceRecoverer:   &stubStaleBalanceRecoverer{recovered: map[string]*mmodel.Balance{aliasWithKey: recoveredBalance}},
		}

		recovered, remaining, err := uc.recoverLaggedBalancesForAliases(ctx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err)
		require.Len(t, recovered, 1)
		assert.Equal(t, recoveredBalance.ID, recovered[0].ID)
		assert.Empty(t, remaining)
	})

	t.Run("propagates recoverer error", func(t *testing.T) {
		uc := &UseCase{
			ConsumerLagFenceEnabled: true,
			LagChecker:              &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}},
			ShardRouter:             router,
			BalanceOperationsTopic:  "ledger.balance.operations",
			StaleBalanceRecoverer:   &stubStaleBalanceRecoverer{err: errors.New("replay failed")},
		}

		_, _, err := uc.recoverLaggedBalancesForAliases(ctx, orgID, ledgerID, []string{aliasWithKey})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "replay failed")
	})
}

type fixedRecoverer struct {
	balance *mmodel.Balance
	alias   string
}

func (f fixedRecoverer) RecoverLaggedAliases(
	_ context.Context,
	_ string,
	_, _ uuid.UUID,
	_ map[int32][]string,
) (map[string]*mmodel.Balance, error) {
	return map[string]*mmodel.Balance{f.alias: f.balance}, nil
}

func TestRecoverLaggedBalancesForAliases_ConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	aliasWithKey := "@alice#default"
	router := shard.NewRouter(8)
	partition := int32(router.ResolveBalance("@alice", "default"))

	recoveredBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(900),
		OnHold:         decimal.Zero,
		Version:        2,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	uc := &UseCase{
		ConsumerLagFenceEnabled: true,
		LagChecker:              &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}},
		ShardRouter:             router,
		BalanceOperationsTopic:  "ledger.balance.operations",
		StaleBalanceRecoverer:   fixedRecoverer{alias: aliasWithKey, balance: recoveredBalance},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			recovered, remaining, err := uc.recoverLaggedBalancesForAliases(ctx, orgID, ledgerID, []string{aliasWithKey})
			if err != nil {
				errCh <- err
				return
			}

			if len(recovered) != 1 || len(remaining) != 0 {
				errCh <- errors.New("unexpected recovery result")
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}
