// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	pkgShard "github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// newTestShardManagerWithWaitCeiling builds a ShardManager with a small
// MigrationWaitMax so a migration-in-progress assertion fails fast.
func newTestShardManagerWithWaitCeiling(t *testing.T, waitMax time.Duration) (*internalsharding.Manager, redis.UniversalClient) {
	t.Helper()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	conn := &libRedis.RedisConnection{Client: client, Connected: true}

	t.Cleanup(func() {
		closeErr := client.Close()
		if closeErr != nil {
			require.ErrorIs(t, closeErr, redis.ErrClosed)
		}

		mini.Close()
	})

	manager := internalsharding.NewManager(
		conn,
		pkgShard.NewRouter(8),
		nil,
		internalsharding.Config{MigrationWaitMax: waitMax},
	)
	require.NotNil(t, manager)

	return manager, client
}

// TestCreateTransaction_WaitsForMigrationUnlock verifies the command-path
// guard added at the entry of CreateTransaction. During an in-progress
// migration (signalled by the MigrationLockKey), the write path must block
// until the lock drops (tested here via the short-circuited error path: the
// lock lives longer than MigrationWaitMax so the error surfaces).
func TestCreateTransaction_WaitsForMigrationUnlock(t *testing.T) {
	t.Parallel()

	manager, client := newTestShardManagerWithWaitCeiling(t, 5*time.Millisecond)
	uc := &UseCase{
		ShardManager: manager,
		ShardRouter:  pkgShard.NewRouter(8),
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@locked"

	// Simulate an in-progress migration for the alias by holding the lock.
	require.NoError(t, client.Set(ctx, utils.MigrationLockKey(orgID, ledgerID, alias), "migration", time.Second).Err())

	txn := &pkgTransaction.Transaction{
		Send: pkgTransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(100),
			Source: pkgTransaction.Source{
				From: []pkgTransaction.FromTo{
					{AccountAlias: alias, Amount: &pkgTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)}},
				},
			},
			Distribute: pkgTransaction.Distribute{
				To: []pkgTransaction.FromTo{
					{AccountAlias: "@counter", Amount: &pkgTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)}},
				},
			},
		},
	}

	// CreateTransaction would need a TransactionRepo mock to reach the DB path;
	// here the migration-lock check happens BEFORE repo access, so we only need
	// to assert the function fails fast with ErrMigrationInProgress wrapped.
	_, err := uc.CreateTransaction(ctx, orgID, ledgerID, uuid.Nil, txn)
	require.Error(t, err)
	assert.ErrorIs(t, err, internalsharding.ErrMigrationInProgress)
}

// TestCollectTransactionAliases asserts aliases are deduplicated across source
// and distribute legs, empty aliases are dropped, and order is stable.
func TestCollectTransactionAliases(t *testing.T) {
	t.Parallel()

	txn := &pkgTransaction.Transaction{
		Send: pkgTransaction.Send{
			Source: pkgTransaction.Source{
				From: []pkgTransaction.FromTo{
					{AccountAlias: "@alice"},
					{AccountAlias: ""},
					{AccountAlias: "@bob"},
					{AccountAlias: "@alice"}, // duplicate
				},
			},
			Distribute: pkgTransaction.Distribute{
				To: []pkgTransaction.FromTo{
					{AccountAlias: "@carol"},
					{AccountAlias: "@bob"}, // duplicate across legs
				},
			},
		},
	}

	aliases := collectTransactionAliases(txn)
	assert.Equal(t, []string{"@alice", "@bob", "@carol"}, aliases)

	assert.Nil(t, collectTransactionAliases(nil))
}
