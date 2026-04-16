// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package sharding

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// TestPubSubSubscriber_InvalidatesRouteCacheOnRoutingUpdate asserts that when
// a SetRoutingOverride is issued on one manager, a peer manager subscribed to
// the routing channel invalidates its local route cache so the next
// ResolveBalanceShard reads the fresh override from Redis.
func TestPubSubSubscriber_InvalidatesRouteCacheOnRoutingUpdate(t *testing.T) {
	t.Parallel()

	mgrPublisher, mini, _ := newTestManager(t, Config{RouteCacheTTL: time.Minute})

	clientSub := redis.NewClient(&redis.Options{Addr: mini.Addr()})

	t.Cleanup(func() { require.NoError(t, clientSub.Close()) })

	mgrSubscriber := NewManager(
		&libRedis.RedisConnection{Client: clientSub, Connected: true},
		mgrPublisher.router,
		nil,
		Config{RouteCacheTTL: time.Minute},
	)
	require.NotNil(t, mgrSubscriber)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@merchant"

	// Prime the subscriber's local cache by installing a routing override via
	// the subscriber manager itself (which caches on write) and then reading
	// through it. A pure FNV resolve does not populate the cache (ResolveBalanceShard
	// only caches Redis-backed overrides), so we must seed with SetRoutingOverride.
	require.NoError(t, mgrSubscriber.SetRoutingOverride(ctx, orgID, ledgerID, alias, 0))

	primeShard, err := mgrSubscriber.ResolveBalanceShard(ctx, orgID, ledgerID, alias, constant.DefaultBalanceKey)
	require.NoError(t, err)
	require.Equal(t, 0, primeShard)
	require.Equal(t, 1, mgrSubscriber.RouteCacheSize())

	metrics := &SubscriberMetrics{}
	subDone := make(chan error, 1)

	go func() {
		subDone <- mgrSubscriber.SubscribeRoutingUpdates(ctx, uuid.Nil, uuid.Nil, metrics)
	}()

	// Brief wait lets PSUBSCRIBE register before we publish. Without this the
	// first message is occasionally dropped by miniredis and the test flakes.
	time.Sleep(50 * time.Millisecond)

	// Publish a new routing override on the authoritative Redis instance,
	// targeting a different shard than the one currently cached.
	targetShard := (primeShard + 1) % mgrPublisher.router.ShardCount()
	require.NoError(t, mgrPublisher.SetRoutingOverride(ctx, orgID, ledgerID, alias, targetShard))

	// Wait for the subscriber to observe the invalidation.
	require.Eventually(t, func() bool {
		return metrics.InvalidationsTotal() >= 1
	}, 2*time.Second, 10*time.Millisecond, "subscriber should invalidate on routing update")

	// Local cache must be empty now so the next resolve reads the override.
	assert.Equal(t, 0, mgrSubscriber.RouteCacheSize())

	resolved, err := mgrSubscriber.ResolveBalanceShard(ctx, orgID, ledgerID, alias, constant.DefaultBalanceKey)
	require.NoError(t, err)
	assert.Equal(t, targetShard, resolved)

	cancel()

	select {
	case err := <-subDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not exit after ctx cancel")
	}
}

// TestPubSubSubscriber_DecodesMalformedMessages asserts the subscriber survives
// garbage payloads and only counts valid invalidations.
func TestPubSubSubscriber_DecodesMalformedMessages(t *testing.T) {
	t.Parallel()

	mgr, mini, _ := newTestManager(t, Config{RouteCacheTTL: time.Minute})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metrics := &SubscriberMetrics{}
	subDone := make(chan error, 1)

	go func() {
		subDone <- mgr.SubscribeRoutingUpdates(ctx, uuid.Nil, uuid.Nil, metrics)
	}()

	// Wait briefly for the subscriber goroutine to complete its SUBSCRIBE handshake.
	time.Sleep(50 * time.Millisecond)

	// Valid channel, malformed payload (no colon).
	mini.Publish(utils.ShardRoutingUpdatesChannel(uuid.New(), uuid.New()), "malformed-no-colon")

	// Completely bogus channel name (still matches the wildcard).
	mini.Publish("shard_routing_updates:not-a-uuid-pair", "@alice:1")

	require.Eventually(t, func() bool {
		return metrics.DecodeErrorsTotal() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	<-subDone
}

// TestCreateTransaction_WaitsForMigrationUnlock is implemented at the command
// layer; it ensures the command-side helper blocks until the lock drops. See
// ../services/command/shard_routing_d2_test.go for the command integration.

// TestWaitForDrain_PollsInFlightCounter asserts the new counter-based drain
// blocks while writers hold the counter above zero and returns as soon as the
// counter clears, without sleeping a fixed interval.
func TestWaitForDrain_PollsInFlightCounter(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{MigrationWaitMax: 500 * time.Millisecond})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@busy"

	// Prime two in-flight writes.
	_, err := mgr.IncrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)

	_, err = mgr.IncrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)

	drained := make(chan error, 1)

	start := time.Now()

	go func() {
		drained <- mgr.waitForDrainByCounter(ctx, orgID, ledgerID, alias)
	}()

	// Give the poller a couple of iterations.
	time.Sleep(5 * time.Millisecond)

	select {
	case <-drained:
		t.Fatal("drain returned before counter cleared")
	default:
	}

	// Decrement both counters asynchronously to simulate writers completing.
	go func() {
		_, _ = mgr.DecrementInFlight(ctx, orgID, ledgerID, alias)

		time.Sleep(2 * time.Millisecond)

		_, _ = mgr.DecrementInFlight(ctx, orgID, ledgerID, alias)
	}()

	select {
	case err := <-drained:
		require.NoError(t, err)
		assert.Less(t, time.Since(start), 500*time.Millisecond)
	case <-time.After(1 * time.Second):
		t.Fatal("drain did not observe counter clear")
	}
}

// TestWaitForDrain_TimesOutWhenInFlightStuck asserts the per-manager
// MigrationWaitMax ceiling is enforced when writers never complete.
func TestWaitForDrain_TimesOutWhenInFlightStuck(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{MigrationWaitMax: 20 * time.Millisecond})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@stuck"

	_, err := mgr.IncrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)

	err = mgr.waitForDrainByCounter(ctx, orgID, ledgerID, alias)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMigrationInProgress)
}

// TestTryAcquireRebalancePermits_AtomicLuaEval asserts that under concurrent
// contention the Lua-backed permit acquisition produces exactly one winner,
// eliminating the SETNX+DEL thundering-herd race.
func TestTryAcquireRebalancePermits_AtomicLuaEval(t *testing.T) {
	t.Parallel()

	// Use two separate Managers that share the same backing Redis to model two
	// independent rebalancer pods. Running them concurrently exercises the Lua
	// atomicity guarantee; without it, a 3-step SETNX+DEL sequence would
	// occasionally hand source+target to racer A and the account key to racer B.
	mgr1, mini, _ := newTestManager(t, Config{ShardMigrationCooldown: 5 * time.Second, AccountMigrationCooldown: 5 * time.Second})

	client2 := redis.NewClient(&redis.Options{Addr: mini.Addr()})

	t.Cleanup(func() { require.NoError(t, client2.Close()) })

	mgr2 := NewManager(
		&libRedis.RedisConnection{Client: client2, Connected: true},
		mgr1.router,
		nil,
		Config{ShardMigrationCooldown: 5 * time.Second, AccountMigrationCooldown: 5 * time.Second},
	)
	require.NotNil(t, mgr2)

	const iterations = 50

	for i := 0; i < iterations; i++ {
		account := HotAccount{
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Alias:          "@hot",
		}

		// Use a fresh source/target pair each iteration so the shard cooldown
		// keys from the previous iteration don't block the new attempt.
		sourceShard := 2 * i
		targetShard := sourceShard + 1

		var (
			wg              sync.WaitGroup
			winners         atomic.Int64
			totalSuccessErr atomic.Int64
		)

		wg.Add(2)

		runRacer := func(mgr *Manager) {
			defer wg.Done()

			ok, err := mgr.TryAcquireRebalancePermits(context.Background(), sourceShard, targetShard, account)
			if err != nil {
				totalSuccessErr.Add(1)

				return
			}

			if ok {
				winners.Add(1)
			}
		}

		go runRacer(mgr1)
		go runRacer(mgr2)

		wg.Wait()

		require.EqualValues(t, 0, totalSuccessErr.Load(), "no racer should error")
		require.EqualValues(t, 1, winners.Load(), "exactly one racer should win permits on iteration %d", i)
	}
}

// TestAccountCooldown_RedisOverrideBeatsCfgDefault asserts that a per-account
// cooldown override installed in Redis overrides the static configured default.
func TestAccountCooldown_RedisOverrideBeatsCfgDefault(t *testing.T) {
	t.Parallel()

	mgr, _, client := newTestManager(t, Config{
		ShardMigrationCooldown:   10 * time.Second,
		AccountMigrationCooldown: 5 * time.Minute,
	})

	ctx := context.Background()
	orgID := uuid.New()

	// Default: cfg wins.
	cooldown := mgr.resolveAccountCooldown(ctx, client, orgID, "@alice")
	assert.Equal(t, 5*time.Minute, cooldown)

	// Install override.
	require.NoError(t, mgr.SetAccountCooldownOverride(ctx, orgID, "@alice", 90*time.Second, time.Hour))

	override := mgr.resolveAccountCooldown(ctx, client, orgID, "@alice")
	assert.Equal(t, 90*time.Second, override)

	// Clearing (cooldown <= 0) reverts to cfg default.
	require.NoError(t, mgr.SetAccountCooldownOverride(ctx, orgID, "@alice", 0, 0))

	reverted := mgr.resolveAccountCooldown(ctx, client, orgID, "@alice")
	assert.Equal(t, 5*time.Minute, reverted)

	// Malformed value also falls back to cfg default.
	require.NoError(t, client.Set(ctx, utils.ShardAccountCooldownOverrideKey(orgID, "@alice"), "not-a-duration", time.Minute).Err())

	fallback := mgr.resolveAccountCooldown(ctx, client, orgID, "@alice")
	assert.Equal(t, 5*time.Minute, fallback)
}

// TestAccountCooldown_EndToEndThroughPermits asserts the override is honored
// by the Lua permit script (the account lock TTL reflects the override, not
// the cfg default).
func TestAccountCooldown_EndToEndThroughPermits(t *testing.T) {
	t.Parallel()

	mgr, _, client := newTestManager(t, Config{
		ShardMigrationCooldown:   100 * time.Millisecond,
		AccountMigrationCooldown: 1 * time.Hour,
	})

	ctx := context.Background()

	account := HotAccount{OrganizationID: uuid.New(), LedgerID: uuid.New(), Alias: "@vip"}

	// Install a much shorter override.
	require.NoError(t, mgr.SetAccountCooldownOverride(ctx, account.OrganizationID, account.Alias, 50*time.Millisecond, time.Hour))

	ok, err := mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	require.True(t, ok)

	accountKey := utils.ShardRebalanceAccountCooldownKey(account.OrganizationID, account.LedgerID, account.Alias)
	ttl, err := client.PTTL(ctx, accountKey).Result()
	require.NoError(t, err)
	assert.LessOrEqual(t, ttl, 100*time.Millisecond)
	assert.Greater(t, ttl, time.Duration(0))
}

// TestInFlightCounter_IncrementDecrementSymmetric asserts the counter floors
// at zero even if a decrement outruns its increment.
func TestInFlightCounter_IncrementDecrementSymmetric(t *testing.T) {
	t.Parallel()

	mgr, _, _ := newTestManager(t, Config{})
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@sym"

	after, err := mgr.IncrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)
	assert.EqualValues(t, 1, after)

	after, err = mgr.DecrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)
	assert.EqualValues(t, 0, after)

	// Orphan decrement should clamp to 0, not go negative.
	after, err = mgr.DecrementInFlight(ctx, orgID, ledgerID, alias)
	require.NoError(t, err)
	assert.EqualValues(t, 0, after)
}
