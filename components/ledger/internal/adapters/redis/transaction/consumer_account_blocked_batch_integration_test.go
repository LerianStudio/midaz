//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// seedCachedBalance writes a BalanceRedis-shaped blob to Redis exactly as the
// balance atomic Lua script would (PascalCase field names), with the given
// transactional state and AccountBlocked flag, under a 1-hour TTL.
func seedCachedBalance(t *testing.T, infra *integrationTestInfra, internalKey string, accountBlocked int) cachedBalance {
	t.Helper()

	cb := cachedBalance{
		ID:                    uuid.New().String(),
		Available:             "7777",
		OnHold:                "123",
		Version:               42,
		AccountType:           "deposit",
		AccountID:             uuid.New().String(),
		AssetCode:             "USD",
		AllowSending:          1,
		AllowReceiving:        1,
		AccountBlocked:        accountBlocked,
		Key:                   "default",
		Direction:             "credit",
		OverdraftUsed:         "250.50",
		AllowOverdraft:        0,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}

	payload, err := json.Marshal(cb)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(context.Background(), internalKey, payload, time.Hour).Err())

	return cb
}

// assertTransactionalStatePreserved pins the write-back invariant: the atomic
// script's authoritative fields (which may be ahead of PostgreSQL while sync is
// pending) survive an AccountBlocked-only mutation verbatim.
func assertTransactionalStatePreserved(t *testing.T, want, got cachedBalance) {
	t.Helper()

	assert.Equal(t, want.ID, got.ID, "ID must be preserved")
	assert.Equal(t, want.Available, got.Available, "Available must be preserved")
	assert.Equal(t, want.OnHold, got.OnHold, "OnHold must be preserved")
	assert.Equal(t, want.Version, got.Version, "Version must be preserved")
	assert.Equal(t, want.OverdraftUsed, got.OverdraftUsed, "OverdraftUsed must be preserved")
	// Settings-derived fields are untouched by this path.
	assert.Equal(t, want.AllowOverdraft, got.AllowOverdraft)
	assert.Equal(t, want.OverdraftLimitEnabled, got.OverdraftLimitEnabled)
	assert.Equal(t, want.OverdraftLimit, got.OverdraftLimit)
	assert.Equal(t, want.BalanceScope, got.BalanceScope)
}

// TestIntegration_SetAccountBlockedMany_FlipsOnlyBlockedFlag exercises the money
// path end to end against a real Redis: a cached balance whose transactional
// state is ahead of PostgreSQL keeps every such field while ONLY AccountBlocked
// flips, in both directions, and the canonical 1-day TTL is (re)applied.
func TestIntegration_SetAccountBlockedMany_FlipsOnlyBlockedFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, "@blockable#default")

	seeded := seedCachedBalance(t, infra, internalKey, 0)

	// Block: 0 -> 1, transactional state preserved.
	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, []string{internalKey}, true))

	blocked := readCachedBalance(t, infra, internalKey)
	assert.Equal(t, 1, blocked.AccountBlocked, "AccountBlocked must flip to 1 on block")
	assertTransactionalStatePreserved(t, seeded, blocked)

	ttl, err := infra.redisContainer.Client.TTL(ctx, internalKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 86000*time.Second, "TTL must match the canonical 1-day cache TTL")
	assert.LessOrEqual(t, ttl, balanceCacheSettingsTTL)

	// Unblock: 1 -> 0, transactional state still preserved.
	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, []string{internalKey}, false))

	unblocked := readCachedBalance(t, infra, internalKey)
	assert.Equal(t, 0, unblocked.AccountBlocked, "AccountBlocked must flip back to 0 on unblock")
	assertTransactionalStatePreserved(t, seeded, unblocked)
}

// TestIntegration_SetAccountBlockedMany_MutatesEveryKeyInBatch proves the whole
// key set of an account is flipped in one call, each blob keeping its own
// transactional state.
func TestIntegration_SetAccountBlockedMany_MutatesEveryKeyInBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	keyDefault := utils.BalanceInternalKey(orgID, ledgerID, "@blockable#default")
	keySavings := utils.BalanceInternalKey(orgID, ledgerID, "@blockable#savings")

	seededDefault := seedCachedBalance(t, infra, keyDefault, 0)
	seededSavings := seedCachedBalance(t, infra, keySavings, 0)

	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, []string{keyDefault, keySavings}, true))

	gotDefault := readCachedBalance(t, infra, keyDefault)
	gotSavings := readCachedBalance(t, infra, keySavings)

	assert.Equal(t, 1, gotDefault.AccountBlocked)
	assert.Equal(t, 1, gotSavings.AccountBlocked)
	assertTransactionalStatePreserved(t, seededDefault, gotDefault)
	assertTransactionalStatePreserved(t, seededSavings, gotSavings)
}

// TestIntegration_SetAccountBlockedMany_CacheMissIsNoOp verifies a key with no
// cache entry is left untouched: no partial entry is created, matching the
// "next transaction reloads from PostgreSQL" contract.
func TestIntegration_SetAccountBlockedMany_CacheMissIsNoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, "@missing#default")

	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, []string{internalKey}, true))

	exists, err := infra.redisContainer.Client.Exists(ctx, internalKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a block mutation on a missing key must not create one")
}

// TestIntegration_SetAccountBlockedMany_MissingKeyInBatchStillMutatesPresent
// confirms a mix of present and absent keys mutates only the present ones and
// never resurrects the absent one.
func TestIntegration_SetAccountBlockedMany_MissingKeyInBatchStillMutatesPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	present := utils.BalanceInternalKey(orgID, ledgerID, "@present#default")
	absent := utils.BalanceInternalKey(orgID, ledgerID, "@absent#default")

	seeded := seedCachedBalance(t, infra, present, 0)

	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, []string{present, absent}, true))

	got := readCachedBalance(t, infra, present)
	assert.Equal(t, 1, got.AccountBlocked)
	assertTransactionalStatePreserved(t, seeded, got)

	exists, err := infra.redisContainer.Client.Exists(ctx, absent).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the absent key must not be created")
}

// TestIntegration_SetAccountBlockedMany_CorruptBlobFailsClosed proves the
// fail-closed, all-or-nothing contract: a non-JSON cached value makes the whole
// batch error, and a sibling valid key in the same call is NOT mutated.
func TestIntegration_SetAccountBlockedMany_CorruptBlobFailsClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	valid := utils.BalanceInternalKey(orgID, ledgerID, "@valid#default")
	corrupt := utils.BalanceInternalKey(orgID, ledgerID, "@corrupt#default")

	seeded := seedCachedBalance(t, infra, valid, 0)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, corrupt, "}{not-json", time.Hour).Err())

	err := infra.repo.SetAccountBlockedMany(ctx, []string{valid, corrupt}, true)
	require.Error(t, err, "a corrupt cached blob must surface as an error (fail-closed)")
	assert.Contains(t, err.Error(), "corrupt cached balance")

	// All-or-nothing: the valid sibling must be untouched by the aborted batch.
	got := readCachedBalance(t, infra, valid)
	assert.Equal(t, 0, got.AccountBlocked, "no partial flip may be observed on the valid key")
	assertTransactionalStatePreserved(t, seeded, got)
}
