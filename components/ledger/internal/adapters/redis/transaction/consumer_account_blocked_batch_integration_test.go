//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestIntegration_SetAccountBlockedMany_MultiChunkSuccess proves the success path
// processes EVERY chunk, not just the first: keys are chunked at maxRedisBatchSize
// (1000) into separate EVALs, so a key that lands in the second chunk must be
// flipped exactly like a first-chunk key. Seeds maxRedisBatchSize+5 keys under one
// tenant and asserts each one -- including the second-chunk tail [1000..1004] -- is
// blocked, with the transactional state of a sampled second-chunk key preserved.
func TestIntegration_SetAccountBlockedMany_MultiChunkSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	const total = maxRedisBatchSize + 5 // 1005: chunk 1 = 1000, chunk 2 = 5.

	keys := make([]string, 0, total)
	seeds := make(map[string]cachedBalance, total)

	for i := 0; i < total; i++ {
		key := utils.BalanceInternalKey(orgID, ledgerID, fmt.Sprintf("@blockable%d#default", i))
		keys = append(keys, key)
		seeds[key] = seedCachedBalance(t, infra, key, 0)
	}

	// Unique-UUID namespacing keeps these keys off every other test, and the
	// explicit teardown keeps ~1000 keys from lingering in the reusable container.
	t.Cleanup(func() {
		require.NoError(t, infra.redisContainer.Client.Del(context.Background(), keys...).Err())
	})

	require.NoError(t, infra.repo.SetAccountBlockedMany(ctx, keys, true))

	// Every key across BOTH chunks must be flipped.
	for _, key := range keys {
		got := readCachedBalance(t, infra, key)
		assert.Equal(t, 1, got.AccountBlocked, "every key across all chunks must be blocked: %s", key)
	}

	// Spot-check the second chunk explicitly: these indices only exist because a
	// later chunk was processed.
	for i := maxRedisBatchSize; i < total; i++ {
		got := readCachedBalance(t, infra, keys[i])
		assert.Equal(t, 1, got.AccountBlocked, "second-chunk key at index %d must be blocked", i)
	}

	// Non-block fields on a sampled second-chunk key survive verbatim.
	sampledKey := keys[maxRedisBatchSize+2]
	assertTransactionalStatePreserved(t, seeds[sampledKey], readCachedBalance(t, infra, sampledKey))
}

// TestIntegration_SetAccountBlockedMany_CorruptKeyInLaterChunkCommitsEarlierChunk
// documents the cross-chunk reality: chunks are independent EVALs run in slice
// order, so an earlier chunk COMMITS before a later chunk aborts on a corrupt blob.
//
// This cross-chunk non-atomicity is BY DESIGN -- it matches the old DelMany, which
// also chunked without a cross-chunk rollback. The all-or-nothing guarantee holds
// only WITHIN a chunk (see the two-pass Lua script). The convergence mechanism for
// the earlier chunk's committed flips is the source of truth (PostgreSQL) plus the
// idempotent retry of this same call: re-running it once the corrupt blob is gone
// re-flips the same keys to the same value, so no divergence survives.
func TestIntegration_SetAccountBlockedMany_CorruptKeyInLaterChunkCommitsEarlierChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	keys := make([]string, 0, maxRedisBatchSize+3)

	// Chunk 1: exactly maxRedisBatchSize valid blobs.
	for i := 0; i < maxRedisBatchSize; i++ {
		key := utils.BalanceInternalKey(orgID, ledgerID, fmt.Sprintf("@chunk1-%d#default", i))
		keys = append(keys, key)
		seedCachedBalance(t, infra, key, 0)
	}

	// Chunk 2: a couple of valid blobs plus ONE corrupt (non-JSON) blob.
	chunk1Sample := keys[0]

	validChunk2 := utils.BalanceInternalKey(orgID, ledgerID, "@chunk2-valid#default")
	keys = append(keys, validChunk2)
	seedCachedBalance(t, infra, validChunk2, 0)

	corrupt := utils.BalanceInternalKey(orgID, ledgerID, "@chunk2-corrupt#default")
	require.NoError(t, infra.redisContainer.Client.Set(ctx, corrupt, "}{not-json", time.Hour).Err())
	keys = append(keys, corrupt)

	validChunk2b := utils.BalanceInternalKey(orgID, ledgerID, "@chunk2-valid2#default")
	keys = append(keys, validChunk2b)
	seedCachedBalance(t, infra, validChunk2b, 0)

	t.Cleanup(func() {
		require.NoError(t, infra.redisContainer.Client.Del(context.Background(), keys...).Err())
	})

	err := infra.repo.SetAccountBlockedMany(ctx, keys, true)

	// (a) The call fails closed on the corrupt blob in the later chunk.
	require.Error(t, err, "a corrupt cached blob in a later chunk must surface as an error (fail-closed)")
	assert.Contains(t, err.Error(), "corrupt cached balance")

	// (b) The corrupt key's raw value is UNCHANGED: the second chunk's two-pass
	// script aborts before any SET, so it never partially writes.
	rawCorrupt, gerr := infra.redisContainer.Client.Get(ctx, corrupt).Result()
	require.NoError(t, gerr)
	assert.Equal(t, "}{not-json", rawCorrupt, "the corrupt blob must be left byte-for-byte unchanged")

	// (c) Cross-chunk reality: chunk 1 committed BEFORE chunk 2 aborted, so a
	// chunk-1 key IS already flipped despite the overall error. This is the
	// by-design non-atomicity documented in the function comment above.
	gotChunk1 := readCachedBalance(t, infra, chunk1Sample)
	assert.Equal(t, 1, gotChunk1.AccountBlocked,
		"an earlier chunk commits before the later chunk aborts (cross-chunk non-atomicity is by design)")
}
