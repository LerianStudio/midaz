//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

const (
	companionRef         = "@source#overdraft"
	companionSeedVersion = 4
)

// newCompanionCacheFixture builds an overdraft-enabled account whose primary
// balance is cached and whose companion exists only as the request seed, the
// state of an account that has never drawn. The fixture holds the account's
// admission, as the balance load that seeded the companion does.
func newCompanionCacheFixture(t *testing.T, client redis.UniversalClient) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.addCompanion("0")
	f.input.Execution.Balances[1].Version = companionSeedVersion
	f.seed(t, 0, f.input.Execution.Balances[0])
	f.syncAccountProtection(t)

	return f
}

func (f *integrationFixture) companionKey() string {
	return f.resolved.Balances[companionRef].Balance
}

func (f *integrationFixture) scheduled(t *testing.T, key string) bool {
	t.Helper()

	_, err := f.client.ZScore(context.Background(), f.resolved.Schedule, key).Result()
	if err == redis.Nil {
		return false
	}

	require.NoError(t, err)

	return true
}

// requireCacheTTL asserts that a key carries the balance cache expiry, allowing
// only for the seconds the test itself takes to read it.
func requireCacheTTL(t *testing.T, client redis.UniversalClient, key string) {
	t.Helper()

	ttl, err := client.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, cachepolicy.BalanceTTL-time.Minute)
	require.LessOrEqual(t, ttl, cachepolicy.BalanceTTL)
}

func requireOnlyPrimaryResult(t *testing.T, raw string) {
	t.Helper()

	result := decodeIntegrationResult(t, raw)
	require.Len(t, result.Movements, 1)
	require.Equal(t, "@source#default", result.Movements[0].BalanceRef)
	require.Len(t, result.Final, 1)
	require.Equal(t, "@source#default", result.Final[0].BalanceRef)
	require.NotContains(t, raw, companionRef, "an unmoved companion is not part of the execution result")
}

func TestIntegrationEngineCompanionCachePublishesAnUnmovedSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	ctx := context.Background()
	f := newCompanionCacheFixture(t, container.Client)
	seed := f.input.Execution.Balances[1]

	raw, err := f.run(t)
	require.NoError(t, err)
	requireOnlyPrimaryResult(t, raw)

	cached, err := container.Client.Get(ctx, f.companionKey()).Bytes()
	require.NoError(t, err, "a committed execution publishes the companion it seeded")
	companion, err := balancecache.Decode(cached)
	require.NoError(t, err)
	require.Equal(t, seed.ID, companion.ID)
	require.Equal(t, seed.AccountID, companion.AccountID)
	require.Equal(t, "overdraft", companion.Key)
	require.True(t, companion.Available.Equal(decimal.Zero), "the companion is published exactly as seeded")
	require.Equal(t, int64(companionSeedVersion), companion.Version, "publishing an unmoved companion advances no version")
	requireCacheTTL(t, container.Client, f.companionKey())
	require.True(t, f.scheduled(t, f.companionKey()), "the published companion is scheduled for balance synchronization")

	recoveryField := f.input.Execution.Transactions[0].ID.String() + ":" + f.input.Execution.ExecutionID.String()
	recovery, err := container.Client.HGet(ctx, f.resolved.Recovery, recoveryField).Result()
	require.NoError(t, err)
	require.NotContains(t, recovery, companionRef, "the recovery evidence carries no companion movement or final state")

	replayed, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, raw, replayed)

	t.Run("a later execution reads the cached companion without an admission", func(t *testing.T) {
		f.rotateExecution()
		// No admission is held any more, and the request carries a divergent seed:
		// a draw can only commit when the companion it moves comes from Redis.
		require.NoError(t, container.Client.Del(ctx, sourceAccountProtection(t, f).Ownership).Err())
		f.input.Execution.Balances[1].Available = decimal.NewFromInt(5)
		f.input.Execution.Balances[1].Version = 9
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(100)

		raw, err := f.run(t)
		require.NoError(t, err)

		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Movements, 2)
		require.Equal(t, "overdraft_companion", result.Movements[1].Role)
		require.Equal(t, integrationState{Available: "0", OnHold: "0", OverdraftUsed: "0", Version: "4"}, result.Movements[1].Before)
		require.Equal(t, integrationState{Available: "30", OnHold: "0", OverdraftUsed: "0", Version: "5"}, result.Movements[1].After)
	})
}

func TestIntegrationEngineCompanionCacheRefreshesACachedCompanion(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	ctx := context.Background()
	f := newCompanionCacheFixture(t, container.Client)
	f.seed(t, 1, f.input.Execution.Balances[1])
	require.NoError(t, container.Client.Expire(ctx, f.companionKey(), 30*time.Second).Err())
	value, err := container.Client.Get(ctx, f.companionKey()).Result()
	require.NoError(t, err)

	raw, err := f.run(t)
	require.NoError(t, err)
	requireOnlyPrimaryResult(t, raw)

	requireCacheTTL(t, container.Client, f.companionKey())
	refreshed, err := container.Client.Get(ctx, f.companionKey()).Result()
	require.NoError(t, err)
	require.Equal(t, value, refreshed, "a refreshed companion keeps its cached value")
	require.False(t, f.scheduled(t, f.companionKey()), "an unchanged cached companion has nothing to synchronize")
}

func TestIntegrationEngineCompanionCacheIsNotWrittenByARefusal(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, test := range []struct {
		name   string
		cached bool
	}{
		{name: "seeded companion"},
		{name: "cached companion", cached: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			f := newCompanionCacheFixture(t, container.Client)
			primary := f.input.Execution.Balances[0]
			primary.Available, primary.AllowOverdraft = decimal.NewFromInt(10), false
			f.input.Execution.Balances[0] = primary
			f.seed(t, 0, primary)

			if test.cached {
				f.seed(t, 1, f.input.Execution.Balances[1])
				require.NoError(t, container.Client.Expire(ctx, f.companionKey(), 30*time.Second).Err())
			}

			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"insufficient_funds"`)
			require.Equal(t, before, f.capture(t), "a refusal publishes no companion and refreshes no expiry")
		})
	}
}

func TestIntegrationEngineCompanionCacheRequiresTheSeedProof(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, test := range []struct {
		name    string
		prepare func(t *testing.T, f *integrationFixture)
	}{
		{name: "no admission held", prepare: func(t *testing.T, f *integrationFixture) {
			require.NoError(t, f.client.Del(context.Background(), sourceAccountProtection(t, f).Ownership).Err())
		}},
		{name: "admission held by another operation", prepare: func(t *testing.T, f *integrationFixture) {
			require.NoError(t, f.client.Set(context.Background(), sourceAccountProtection(t, f).Ownership, "another-admission-token", 0).Err())
		}},
		{name: "companion deletion marker", prepare: func(t *testing.T, f *integrationFixture) {
			require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances[companionRef].Deleted, "1", time.Hour).Err())
		}},
		{name: "companion legacy deletion marker", prepare: func(t *testing.T, f *integrationFixture) {
			require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances[companionRef].LegacyDeleted, "1", time.Hour).Err())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompanionCacheFixture(t, container.Client)
			test.prepare(t, f)

			raw, err := f.run(t)
			require.NoError(t, err, "an execution that never used the companion is not refused because of it")
			requireOnlyPrimaryResult(t, raw)
			require.Zero(t, container.Client.Exists(context.Background(), f.companionKey()).Val(),
				"a companion without the seed proof is not published")
			require.False(t, f.scheduled(t, f.companionKey()))
		})
	}
}

func TestIntegrationEngineCompanionCacheLeavesAnUnwrittenAccountAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newCompanionCacheFixture(t, container.Client)

	// A second account sits in the pool with a cached primary and a seeded
	// companion, but no requirement or posting names it.
	idle := f.input.Execution.Balances[0]
	idle.ID = uuid.MustParse("0d2c6f0e-5a0b-4c3e-9f7e-2b8f6d1c4a90")
	idle.AccountID = uuid.MustParse("5e3a1b7c-8d4f-4a2e-b6c9-7f1d0e2a3b58")
	idle.Alias, idle.BalanceRef = "@idle", "@idle#default"
	idleCompanion := f.input.Execution.Balances[1]
	idleCompanion.ID = uuid.MustParse("9b4e2d1a-6c3f-4e8b-a7d5-1c0f2e3a4b6d")
	idleCompanion.AccountID, idleCompanion.Alias, idleCompanion.BalanceRef = idle.AccountID, "@idle", "@idle#overdraft"
	f.input.Execution.Balances = append(f.input.Execution.Balances, idle, idleCompanion)

	for _, ref := range []string{idle.BalanceRef, idleCompanion.BalanceRef} {
		f.resolved.Balances[ref] = testResolvedBalanceKeys(
			strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", ref, 1),
		)
	}

	f.seed(t, 2, idle)
	f.syncAccountProtection(t)

	raw, err := f.run(t)
	require.NoError(t, err)
	requireOnlyPrimaryResult(t, raw)
	require.Zero(t, container.Client.Exists(context.Background(), f.resolved.Balances[idleCompanion.BalanceRef].Balance).Val(),
		"only the companions of accounts this execution writes are published")
	require.Equal(t, int64(1), container.Client.Exists(context.Background(), f.companionKey()).Val())
}

func TestIntegrationEngineCompanionCacheChargesThePreparedBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	// withBudget runs the fixture execution in isolation under one prepared-byte
	// budget. A deletion marker on the companion leaves every prepared value as it
	// is except the companion publication itself.
	withBudget := func(t *testing.T, budget int, publish bool) (*integrationFixture, map[string]integrationStoredKey, error) {
		t.Helper()

		f := newCompanionCacheFixture(t, container.Client)
		if !publish {
			require.NoError(t, container.Client.Set(context.Background(), f.resolved.Balances[companionRef].Deleted, "1", time.Hour).Err())
		}

		f.limits.MaxPreparedBytes = budget
		before := f.capture(t)
		_, err := f.run(t)

		return f, before, err
	}

	// The smallest budget an execution needs when it publishes no companion.
	low, high := 1, 1<<20
	for low < high {
		middle := (low + high) / 2
		t.Run(fmt.Sprintf("probe without the companion at %d bytes", middle), func(t *testing.T) {
			_, _, err := withBudget(t, middle, false)
			if err == nil {
				high = middle

				return
			}

			require.ErrorContains(t, err, `"code":"prepared_bytes_exceeded"`)
			low = middle + 1
		})
	}

	withoutCompanion := low

	var companionBytes int

	t.Run("the published companion value", func(t *testing.T) {
		f, _, err := withBudget(t, 1<<20, true)
		require.NoError(t, err)
		companionBytes = len(container.Client.Get(context.Background(), f.companionKey()).Val())
		require.Positive(t, companionBytes)
	})

	t.Run("a budget that fits the execution but not the companion refuses before any write", func(t *testing.T) {
		f, before, err := withBudget(t, withoutCompanion, true)
		require.ErrorContains(t, err, `"code":"prepared_bytes_exceeded"`)
		require.NotContains(t, err.Error(), "indeterminate")
		require.Equal(t, before, f.capture(t), "an exceeded budget writes nothing")
	})

	t.Run("a budget that also fits the companion value commits", func(t *testing.T) {
		f, _, err := withBudget(t, withoutCompanion+companionBytes, true)
		require.NoError(t, err)
		require.Equal(t, int64(1), container.Client.Exists(context.Background(), f.companionKey()).Val())
	})
}
