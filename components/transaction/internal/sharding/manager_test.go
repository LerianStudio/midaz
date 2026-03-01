// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package sharding

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgShard "github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

func newTestManager(t *testing.T, cfg Config) (*Manager, *miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	conn := &libRedis.RedisConnection{Client: client, Connected: true}

	manager := NewManager(conn, pkgShard.NewRouter(4), nil, cfg)
	require.NotNil(t, manager)

	t.Cleanup(func() {
		require.NoError(t, client.Close())
		mini.Close()
	})

	return manager, mini, client
}

func TestManagerRoutingOverrideVisibleAcrossInstancesWithoutCache(t *testing.T) {
	t.Parallel()

	mgrA, mini, _ := newTestManager(t, Config{RouteCacheTTL: 0})

	clientB := redis.NewClient(&redis.Options{Addr: mini.Addr()})

	t.Cleanup(func() {
		require.NoError(t, clientB.Close())
	})

	mgrB := NewManager(&libRedis.RedisConnection{Client: clientB, Connected: true}, pkgShard.NewRouter(4), nil, Config{RouteCacheTTL: 0})
	require.NotNil(t, mgrB)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@merchant"

	require.NoError(t, mgrA.SetRoutingOverride(ctx, orgID, ledgerID, alias, 1))
	first, err := mgrB.ResolveBalanceShard(ctx, orgID, ledgerID, alias, constant.DefaultBalanceKey)
	require.NoError(t, err)
	assert.Equal(t, 1, first)

	require.NoError(t, mgrA.SetRoutingOverride(ctx, orgID, ledgerID, alias, 2))
	second, err := mgrB.ResolveBalanceShard(ctx, orgID, ledgerID, alias, constant.DefaultBalanceKey)
	require.NoError(t, err)
	assert.Equal(t, 2, second)
}

func TestManagerResolveBalanceShardIgnoresOutOfRangeOverride(t *testing.T) {
	t.Parallel()

	mgr, _, client := newTestManager(t, Config{RouteCacheTTL: 0})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@merchant"
	balanceKey := constant.DefaultBalanceKey

	expected := mgr.router.ResolveBalance(alias, balanceKey)
	require.NoError(t, client.HSet(ctx, utils.ShardRoutingKey(orgID, ledgerID), alias, "999").Err())

	resolved, err := mgr.ResolveBalanceShard(ctx, orgID, ledgerID, alias, balanceKey)
	require.NoError(t, err)
	assert.Equal(t, expected, resolved)
}

func TestManagerMigrateAccountPreservesPersistentTTL(t *testing.T) {
	t.Parallel()

	mgr, _, client := newTestManager(t, Config{})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@alice"
	balanceKey := constant.DefaultBalanceKey

	sourceShard := mgr.router.ResolveBalance(alias, balanceKey)
	targetShard := (sourceShard + 1) % mgr.router.ShardCount()

	sourceKey := utils.BalanceShardKey(sourceShard, orgID, ledgerID, alias+"#"+balanceKey)
	targetKey := utils.BalanceShardKey(targetShard, orgID, ledgerID, alias+"#"+balanceKey)

	require.NoError(t, client.Set(ctx, sourceKey, `{"available":"10"}`, 0).Err())

	result, err := mgr.MigrateAccount(ctx, orgID, ledgerID, alias, targetShard, []string{balanceKey})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.MigratedKeys)

	_, srcErr := client.Get(ctx, sourceKey).Result()
	require.ErrorIs(t, srcErr, redis.Nil)

	value, getErr := client.Get(ctx, targetKey).Result()
	require.NoError(t, getErr)
	assert.JSONEq(t, `{"available":"10"}`, value)

	ttl, ttlErr := client.TTL(ctx, targetKey).Result()
	require.NoError(t, ttlErr)
	assert.Equal(t, time.Duration(-1), ttl)
}

func TestManagerMigrateAccountRejectsExternalAlias(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{})

	result, err := mgr.MigrateAccount(context.Background(), uuid.New(), uuid.New(), "@external/USD", 1, nil)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestManagerIsolationMarkersExpireAndCleanupSetEntries(t *testing.T) {
	t.Parallel()

	mgr, mini, _ := newTestManager(t, Config{IsolationTTL: 20 * time.Millisecond})

	account := HotAccount{OrganizationID: uuid.New(), LedgerID: uuid.New(), Alias: "@vip"}
	require.NoError(t, mgr.MarkAccountIsolated(context.Background(), account, 2))

	counts, err := mgr.GetShardIsolationCounts(context.Background(), 4)
	require.NoError(t, err)
	assert.EqualValues(t, 1, counts[2])

	mini.FastForward(50 * time.Millisecond)

	counts, err = mgr.GetShardIsolationCounts(context.Background(), 4)
	require.NoError(t, err)
	assert.EqualValues(t, 0, counts[2])
}

func TestManagerWaitForAliasesUnlockedTimeout(t *testing.T) {
	t.Parallel()

	mgr, _, client := newTestManager(t, Config{MigrationWaitMax: 5 * time.Millisecond})
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@busy"

	require.NoError(t, client.Set(ctx, utils.MigrationLockKey(orgID, ledgerID, alias), "1", time.Second).Err())

	err := mgr.WaitForAliasesUnlocked(ctx, orgID, ledgerID, []string{alias})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration in progress")
}

func TestManagerSetAndGetRebalancePaused(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{})

	require.NoError(t, mgr.SetRebalancerPaused(context.Background(), true))
	paused, err := mgr.IsRebalancerPaused(context.Background())
	require.NoError(t, err)
	assert.True(t, paused)

	require.NoError(t, mgr.SetRebalancerPaused(context.Background(), false))
	paused, err = mgr.IsRebalancerPaused(context.Background())
	require.NoError(t, err)
	assert.False(t, paused)
}

func TestManagerGetShardLoadsAndTopHotAccounts(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{MetricsWindow: time.Minute})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	require.NoError(t, mgr.RecordShardAliasLoad(ctx, orgID, ledgerID, "@alice", 1, 3))
	require.NoError(t, mgr.RecordShardAliasLoad(ctx, orgID, ledgerID, "@bob", 1, 1))
	require.NoError(t, mgr.RecordShardAliasLoad(ctx, orgID, ledgerID, "@charlie", 2, 2))

	loads, err := mgr.GetShardLoads(ctx, 4, time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, loads)
	assert.Equal(t, 1, loads[0].ShardID)
	assert.GreaterOrEqual(t, loads[0].Load, int64(4))

	hot, err := mgr.TopHotAccounts(ctx, 1, time.Minute, 2)
	require.NoError(t, err)
	require.NotEmpty(t, hot)
	assert.Equal(t, "@alice", hot[0].Alias)
}

func TestManagerTryAcquireRebalancePermitsContention(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{ShardMigrationCooldown: time.Second, AccountMigrationCooldown: time.Second})
	account := HotAccount{OrganizationID: uuid.New(), LedgerID: uuid.New(), Alias: "@alice"}

	ok, err := mgr.TryAcquireRebalancePermits(context.Background(), 0, 1, account)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = mgr.TryAcquireRebalancePermits(context.Background(), 0, 1, account)
	require.NoError(t, err)
	assert.False(t, ok)
}
