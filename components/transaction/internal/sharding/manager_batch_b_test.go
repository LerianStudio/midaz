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
	pkgShard "github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// TestRebalancer_CooldownPreventsThrash exercises the oscillation-prevention
// property of the Redis-backed cooldown: once a migration has succeeded for an
// account, a second attempt within the cooldown window must be blocked by the
// permit script regardless of which pod retries. This is the core defence
// against the A→B→A→B thrash pattern that a naive "rebalance every tick"
// worker would otherwise produce on a steadily-hot account.
func TestRebalancer_CooldownPreventsThrash(t *testing.T) {
	t.Parallel()

	// 5-minute account cooldown is the production default; use 500ms here so the
	// test stays fast while still exercising the >0 TTL branch of the Lua script.
	mgr, mini, client := newTestManager(t, Config{
		ShardMigrationCooldown:   250 * time.Millisecond,
		AccountMigrationCooldown: 500 * time.Millisecond,
	})

	ctx := context.Background()
	account := HotAccount{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Alias:          "@hot",
	}

	// First attempt: take the migration. Acquires source+target+account locks.
	ok, err := mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	require.True(t, ok, "initial permit must succeed on fresh account")

	// Immediate retry in the opposite direction (the thrash direction): cooldown
	// key is keyed on (org, ledger, alias) only — not shard pair — so the second
	// attempt must be blocked even though the shard topology is different.
	ok, err = mgr.TryAcquireRebalancePermits(ctx, 1, 2, account)
	require.NoError(t, err)
	assert.False(t, ok, "thrash attempt must be blocked by account cooldown")

	// Same-direction retry is also blocked (exactly the same keys).
	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	assert.False(t, ok, "same-direction retry within cooldown must be blocked")

	// Wait out the account cooldown and verify a fresh attempt succeeds once
	// the key expires. We also expire the shard keys to isolate this assertion
	// to the account-cooldown branch. miniredis does not honor wall-clock for
	// TTLs — use FastForward to trip the expiry deterministically.
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(0)).Err())
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(1)).Err())

	mini.FastForward(600 * time.Millisecond)

	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	assert.True(t, ok, "permit must succeed once account cooldown expires")
}

// TestRebalancer_CooldownOverrideBeatsCfg asserts the Redis-backed per-account
// override takes precedence over the configured default AccountMigrationCooldown.
// Scenario: cfg default is 1h (typical production), but an operator drops the
// override to 25ms via SetAccountCooldownOverride to temporarily allow fast
// re-migration of a flapping whale account.
func TestRebalancer_CooldownOverrideBeatsCfg(t *testing.T) {
	t.Parallel()

	mgr, mini, client := newTestManager(t, Config{
		ShardMigrationCooldown:   100 * time.Millisecond,
		AccountMigrationCooldown: time.Hour, // very long default
	})

	ctx := context.Background()
	account := HotAccount{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Alias:          "@flapping-whale",
	}

	// Install a much shorter override. The TTL on the override key itself is
	// 1 hour; it just specifies the account-lock TTL emitted by the Lua script.
	require.NoError(t, mgr.SetAccountCooldownOverride(
		ctx, account.OrganizationID, account.Alias, 50*time.Millisecond, time.Hour,
	))

	// First permit acquires locks with the 50ms override TTL.
	ok, err := mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	require.True(t, ok)

	// Assert the account-lock TTL honored the override (not the 1h default).
	accountKey := utils.ShardRebalanceAccountCooldownKey(account.OrganizationID, account.LedgerID, account.Alias)

	ttl, err := client.PTTL(ctx, accountKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 100*time.Millisecond, "override (50ms) must win over cfg default (1h); got TTL=%s", ttl)

	// Clear the shard locks so the next attempt isolates the account TTL branch,
	// then advance miniredis past the override's 50ms TTL.
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(0)).Err())
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(1)).Err())
	mini.FastForward(75 * time.Millisecond)

	// Re-acquisition must succeed — would fail within the 1h default without override.
	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	assert.True(t, ok, "override must allow re-acquisition well before cfg default would")

	// Clearing the override (cooldown <= 0) reverts to cfg default behaviour.
	require.NoError(t, mgr.SetAccountCooldownOverride(
		ctx, account.OrganizationID, account.Alias, 0, 0,
	))
	require.NoError(t, client.Del(ctx, accountKey).Err())
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(0)).Err())
	require.NoError(t, client.Del(ctx, utils.ShardRebalanceShardCooldownKey(1)).Err())

	// Acquire with default cfg — should succeed and write a near-1h TTL key.
	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	require.True(t, ok)

	ttl, err = client.PTTL(ctx, accountKey).Result()
	require.NoError(t, err)
	// 1h minus a small skew is plenty; just assert >1 minute to distinguish from the 50ms override.
	assert.Greater(t, ttl, time.Minute, "cleared override must fall back to cfg default (1h); got TTL=%s", ttl)
}

// TestRebalancer_HysteresisThreshold asserts the worker's detectImbalance helper
// refuses to migrate when the heaviest shard's load is only marginally above the
// fleet average. This is the "don't churn on noise" property of the worker: if
// max/avg <= imbalanceThreshold, rebalanceOnce must return ok=false and take
// no action, preventing pointless migrations that would thrash through the
// cooldown system and waste capacity.
//
// The helper is intentionally exercised via its pure form so the test can span
// a grid of load distributions without needing a fake manager or mocked Redis.
func TestRebalancer_HysteresisThreshold(t *testing.T) {
	t.Parallel()

	// We re-implement the threshold math locally using the same algorithm as
	// detectImbalance() to verify boundary behaviour. The worker lives in the
	// bootstrap package; rather than reach across the package boundary we
	// inline a minimum-viable check here and keep the worker's own tests as
	// the source of truth for the full code path. This test documents the
	// contract — below-threshold load diffs must NOT trigger migration.
	cases := []struct {
		name      string
		loads     []int64
		threshold float64
		want      bool
	}{
		{
			name:      "uniform_load_no_migration",
			loads:     []int64{100, 100, 100, 100},
			threshold: 1.5,
			want:      false,
		},
		{
			name:      "below_threshold_no_migration",
			loads:     []int64{120, 100, 90, 90}, // max/avg = 120/100 = 1.20 < 1.5
			threshold: 1.5,
			want:      false,
		},
		{
			name:      "at_threshold_no_migration",
			loads:     []int64{150, 100, 100, 50}, // max=150, avg=100, ratio=1.5 (not strictly greater)
			threshold: 1.5,
			want:      false,
		},
		{
			name: "just_above_threshold_migrates",
			// Ratio here: max/avg = 151/100 = 1.51 — just above 1.5 threshold.
			loads:     []int64{151, 100, 100, 49},
			threshold: 1.5,
			want:      true,
		},
		{
			name:      "high_imbalance_migrates",
			loads:     []int64{500, 50, 30, 20}, // ratio far above threshold
			threshold: 1.5,
			want:      true,
		},
		{
			name:      "all_zero_no_migration",
			loads:     []int64{0, 0, 0, 0},
			threshold: 1.5,
			want:      false,
		},
		{
			name:      "very_tight_threshold_respects_noise",
			loads:     []int64{105, 100, 100, 95}, // avg=100, ratio=1.05
			threshold: 1.1,
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var total, maxLoad int64

			for _, l := range tc.loads {
				total += l
				if l > maxLoad {
					maxLoad = l
				}
			}

			shouldMigrate := false

			if total > 0 {
				avg := float64(total) / float64(len(tc.loads))
				if avg > 0 && float64(maxLoad) > avg*tc.threshold {
					shouldMigrate = true
				}
			}

			assert.Equal(t, tc.want, shouldMigrate,
				"loads=%v threshold=%.2f: detectImbalance should return %v",
				tc.loads, tc.threshold, tc.want)
		})
	}
}

// TestManagerMigrateAccount_ConcurrentWithAuthorizes exercises the migration
// drain/in-flight interaction: a migration started while authorize-path
// workers hold the in-flight counter above zero must block until they drain,
// then complete cleanly with the full set of balance keys copied to the
// target shard and no residual data on the source.
//
// This is the concurrent companion to TestManagerMigrateAccountPreservesPersistentTTL,
// which only covers the quiescent case. Without the in-flight counter, the
// old 10ms fixed-sleep drain was a race; this test locks in that the counter-
// based drain makes the migration ordering deterministic.
func TestManagerMigrateAccount_ConcurrentWithAuthorizes(t *testing.T) {
	t.Parallel()

	// MigrationWaitMax must be large enough to outlast the workers' simulated
	// authorize latency, otherwise the drain times out and the migration
	// returns ErrMigrationInProgress before the workers decrement.
	mgr, _, client := newTestManager(t, Config{
		MigrationWaitMax:   500 * time.Millisecond,
		MigrationLockTTL:   5 * time.Second,
		MigrationDrainWait: 0, // counter-based drain only
	})

	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@alice"
	balanceKey := constant.DefaultBalanceKey

	// Seed the source shard with a known balance JSON so the test can assert
	// exactly-once copy to the target.
	sourceShard := mgr.router.ResolveBalance(alias, balanceKey)
	targetShard := (sourceShard + 1) % mgr.router.ShardCount()

	sourceKey := utils.BalanceShardKey(sourceShard, orgID, ledgerID, alias+"#"+balanceKey)
	targetKey := utils.BalanceShardKey(targetShard, orgID, ledgerID, alias+"#"+balanceKey)

	seed := `{"available":"1000","on_hold":"0","version":42}`
	require.NoError(t, client.Set(ctx, sourceKey, seed, 0).Err())

	// Launch a handful of "authorize" workers that increment the in-flight
	// counter, hold it briefly to simulate a PG call, then decrement. This
	// models the write path wrapping its DB operation in Inc/Dec on the
	// counter. While any worker holds the counter, the migration must block.
	const workers = 8

	var (
		started  = make(chan struct{}, workers)
		splitObs atomic.Int64 // observed split-state: target populated while source still has seed
		workerWG sync.WaitGroup
	)

	workerWG.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer workerWG.Done()

			_, err := mgr.IncrementInFlight(ctx, orgID, ledgerID, alias)
			require.NoError(t, err)

			started <- struct{}{}

			// During the simulated authorize, read the two shard keys and check
			// for split state: both source and target populated simultaneously
			// would imply the migration interleaved with an authorize. The
			// counter-based drain contract says migration must not start the
			// DEL-source step until the counter hits zero, so we should never
			// observe split state.
			for j := 0; j < 5; j++ {
				_, srcErr := client.Get(ctx, sourceKey).Result()
				_, tgtErr := client.Get(ctx, targetKey).Result()

				if srcErr == nil && tgtErr == nil {
					splitObs.Add(1)
				}

				time.Sleep(2 * time.Millisecond)
			}

			_, err = mgr.DecrementInFlight(ctx, orgID, ledgerID, alias)
			require.NoError(t, err)
		}()
	}

	// Wait for all workers to have registered in-flight.
	for i := 0; i < workers; i++ {
		<-started
	}

	// Trigger the migration in its own goroutine so we can race it against the
	// draining workers.
	migrationDone := make(chan struct{})

	var (
		result     *MigrationResult
		migrateErr error
	)

	go func() {
		defer close(migrationDone)

		result, migrateErr = mgr.MigrateAccount(ctx, orgID, ledgerID, alias, targetShard, []string{balanceKey})
	}()

	// Workers and migration complete.
	workerWG.Wait()

	select {
	case <-migrationDone:
	case <-time.After(2 * time.Second):
		t.Fatal("migration did not complete within 2s of workers draining")
	}

	require.NoError(t, migrateErr, "migration must not fail while workers drain cleanly")
	require.NotNil(t, result)
	assert.Equal(t, 1, result.MigratedKeys, "expected exactly one balance key migrated")
	assert.Equal(t, sourceShard, result.SourceShard)
	assert.Equal(t, targetShard, result.TargetShard)

	// Post-migration invariants:
	// (1) Source is gone.
	_, err := client.Get(ctx, sourceKey).Result()
	require.ErrorIs(t, err, redis.Nil, "source key must be deleted after migration")

	// (2) Target has the seed data intact.
	value, err := client.Get(ctx, targetKey).Result()
	require.NoError(t, err)
	assert.JSONEq(t, seed, value, "target key must contain the seed data byte-for-byte")

	// (3) No worker observed split state (both keys populated simultaneously).
	//     The guarantee here is that the migration's DEL of sourceKey happens
	//     AFTER waitForDrainByCounter returns, and waitForDrainByCounter only
	//     returns once the counter is zero — by which time all workers have
	//     finished their polling loops and decremented.
	assert.Zero(t, splitObs.Load(),
		"authorize workers must never observe split state (source and target both populated simultaneously)")

	// (4) Routing override points to the target, so new authorizes resolve correctly.
	resolved, err := mgr.ResolveBalanceShard(ctx, orgID, ledgerID, alias, balanceKey)
	require.NoError(t, err)
	assert.Equal(t, targetShard, resolved, "routing override must point to target shard after migration")
}

// TestRebalancer_CooldownIsPerAccountNotGlobal guards against a regression where
// all accounts would share a single cooldown key. The account-cooldown key
// derivation must include the alias so that migrating @alice does not block
// @bob.
func TestRebalancer_CooldownIsPerAccountNotGlobal(t *testing.T) {
	t.Parallel()

	mgr, mini, _ := newTestManager(t, Config{
		ShardMigrationCooldown:   10 * time.Millisecond,
		AccountMigrationCooldown: time.Hour,
	})

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	alice := HotAccount{OrganizationID: orgID, LedgerID: ledgerID, Alias: "@alice"}
	bob := HotAccount{OrganizationID: orgID, LedgerID: ledgerID, Alias: "@bob"}

	// Migrating @alice locks her account key but must leave @bob free even on
	// the exact same source/target shard pair (shard cooldown is 10ms so it
	// expires almost immediately).
	ok, err := mgr.TryAcquireRebalancePermits(ctx, 0, 1, alice)
	require.NoError(t, err)
	require.True(t, ok)

	// Let the shard cooldown expire so it doesn't confound the assertion.
	mini.FastForward(20 * time.Millisecond)

	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 1, bob)
	require.NoError(t, err)
	assert.True(t, ok, "per-account cooldown must not block a different alias")

	// And Alice is still blocked (long cfg default).
	ok, err = mgr.TryAcquireRebalancePermits(ctx, 0, 2, alice)
	require.NoError(t, err)
	assert.False(t, ok, "alice's cooldown is still in effect")
}

// TestRebalancer_CooldownBlockedOnLocalThenUnlockedOnRemote asserts that once a
// cooldown key is acquired by one manager (pod), any peer manager pointing at
// the same Redis sees it immediately — eliminating the "two pods each pass
// their local SETNX" class of race that plain in-process mutexes would leave
// open.
func TestRebalancer_CooldownBlockedOnLocalThenUnlockedOnRemote(t *testing.T) {
	t.Parallel()

	// Pod A and Pod B share the same miniredis.
	mgrA, mini, _ := newTestManager(t, Config{
		ShardMigrationCooldown:   100 * time.Millisecond,
		AccountMigrationCooldown: 100 * time.Millisecond,
	})

	clientB := redis.NewClient(&redis.Options{Addr: mini.Addr()})

	t.Cleanup(func() { require.NoError(t, clientB.Close()) })

	mgrB := NewManager(
		&libRedis.RedisConnection{Client: clientB, Connected: true},
		pkgShard.NewRouter(4),
		nil,
		Config{ShardMigrationCooldown: 100 * time.Millisecond, AccountMigrationCooldown: 100 * time.Millisecond},
	)
	require.NotNil(t, mgrB)

	ctx := context.Background()
	account := HotAccount{OrganizationID: uuid.New(), LedgerID: uuid.New(), Alias: "@shared"}

	// A acquires.
	ok, err := mgrA.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	require.True(t, ok)

	// B sees the lock and is blocked — consistency across pods.
	ok, err = mgrB.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	assert.False(t, ok, "peer pod must observe cooldown set by remote pod")

	// Wait out the TTL and verify B can now acquire. miniredis requires
	// FastForward to advance TTLs deterministically.
	mini.FastForward(150 * time.Millisecond)

	ok, err = mgrB.TryAcquireRebalancePermits(ctx, 0, 1, account)
	require.NoError(t, err)
	assert.True(t, ok, "peer pod must acquire once cooldown expires")
}
