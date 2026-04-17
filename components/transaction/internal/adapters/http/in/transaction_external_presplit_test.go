// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9" //nolint:depguard // test-only: miniredis-backed client for cache-state assertions in balance-key pre-split tests.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

func TestApplyExternalPreSplitBalanceKeys(t *testing.T) { //nolint:funlen
	t.Run("inflow external source gets shard key from destination", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		router := shard.NewRouter(8)
		h := &TransactionHandler{Query: &query.UseCase{ShardRouter: router}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source:     pkgTransaction.Source{From: []pkgTransaction.FromTo{{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey}}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey}}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		assert.Equal(t, router.ResolveExternalBalanceKey("@alice"), input.Send.Source.From[0].BalanceKey)
		assert.Equal(t, constant.DefaultBalanceKey, input.Send.Distribute.To[0].BalanceKey)
	})

	t.Run("outflow external destination gets shard key from source", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		router := shard.NewRouter(8)
		h := &TransactionHandler{Query: &query.UseCase{ShardRouter: router}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source:     pkgTransaction.Source{From: []pkgTransaction.FromTo{{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey}}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey}}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		assert.Equal(t, router.ResolveExternalBalanceKey("@alice"), input.Send.Distribute.To[0].BalanceKey)
		assert.Equal(t, constant.DefaultBalanceKey, input.Send.Source.From[0].BalanceKey)
	})

	t.Run("explicit non-default external key is normalized to canonical shard", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		router := shard.NewRouter(8)
		h := &TransactionHandler{Query: &query.UseCase{ShardRouter: router}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source:     pkgTransaction.Source{From: []pkgTransaction.FromTo{{AccountAlias: "@external/USD", BalanceKey: "custom-key"}}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey}}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		assert.Equal(t, router.ResolveExternalBalanceKey("@alice"), input.Send.Source.From[0].BalanceKey)
	})

	t.Run("multiple counterparties use deterministic spread", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		router := shard.NewRouter(8)
		h := &TransactionHandler{Query: &query.UseCase{ShardRouter: router}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source: pkgTransaction.Source{From: []pkgTransaction.FromTo{
					{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
					{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
				}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{
					{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
					{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey},
				}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		counterparties := []string{"@alice", "@bob"}
		for i := range input.Send.Source.From {
			expectedCounterparty := pickExternalCounterpartyAlias(counterparties, "@external/USD", i)
			expectedKey := router.ResolveExternalBalanceKey(expectedCounterparty)
			assert.Equal(t, expectedKey, input.Send.Source.From[i].BalanceKey)
		}
	})

	t.Run("without shard router does nothing", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		h := &TransactionHandler{Query: &query.UseCase{}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source:     pkgTransaction.Source{From: []pkgTransaction.FromTo{{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey}}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey}}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		assert.Equal(t, constant.DefaultBalanceKey, input.Send.Source.From[0].BalanceKey)
	})

	t.Run("uses shard manager override for canonical external key", func(t *testing.T) {
		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()

		mini, err := miniredis.Run()
		require.NoError(t, err)

		client := goredis.NewClient(&goredis.Options{Addr: mini.Addr()})

		t.Cleanup(func() {
			require.NoError(t, client.Close())
			mini.Close()
		})

		router := shard.NewRouter(8)
		manager := internalsharding.NewManager(&libRedis.RedisConnection{Client: client, Connected: true}, router, nil, internalsharding.Config{})
		require.NotNil(t, manager)

		require.NoError(t, manager.SetRoutingOverride(ctx, organizationID, ledgerID, "@alice", 5))

		h := &TransactionHandler{Query: &query.UseCase{ShardRouter: router, ShardManager: manager}}

		input := &pkgTransaction.Transaction{
			Send: pkgTransaction.Send{
				Source:     pkgTransaction.Source{From: []pkgTransaction.FromTo{{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey}}},
				Distribute: pkgTransaction.Distribute{To: []pkgTransaction.FromTo{{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey}}},
			},
		}

		h.ApplyExternalPreSplitBalanceKeys(ctx, organizationID, ledgerID, input)

		assert.Equal(t, "shard_5", input.Send.Source.From[0].BalanceKey)
	})
}
