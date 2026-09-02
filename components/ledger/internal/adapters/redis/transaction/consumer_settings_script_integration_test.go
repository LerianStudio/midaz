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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// These tests exercise scripts/update_balance_settings.lua directly against
// the real engine (Valkey via testcontainers), independent of
// UpdateBalanceCacheSettings, to lock the contract the Go caller depends on:
// an absent key is a no-op, a corrupt blob is reported rather than silently
// overwritten, a valid blob gets the settings fields rewritten with legacy
// camelCase aliases dropped, and — the parity claim behind moving the
// mutation into Lua — a blob produced by the real balance_atomic_operation.lua
// round-trips through this script with its transactional fields byte-for-byte
// intact.

func TestIntegration_UpdateBalanceSettingsScript_AbsentKeyIsNoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-script-test:" + uuid.NewString() // never seeded

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, 1, "500.00", mmodel.BalanceScopeTransactional, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 0, result, "an absent key must be reported as a no-op, not an error")

	exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the script must not create the key on a no-op")
}

func TestIntegration_UpdateBalanceSettingsScript_CorruptBlobReturnsErrorCodeAndLeavesValueIntact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-script-test:" + uuid.NewString()
	corrupt := "not-valid-json{{{"

	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, corrupt, time.Hour).Err())

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, 1, "500.00", mmodel.BalanceScopeTransactional, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, -2, result, "a non-JSON cached value must be reported as corrupt")

	unchanged, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, corrupt, unchanged, "a corrupt blob must not be mutated")
}

func TestIntegration_UpdateBalanceSettingsScript_ValidBlobAppliesSettingsAndDedupesLegacyCasing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-script-test:" + uuid.NewString()

	// Legacy document carrying both live transactional state and camelCase
	// aliases a pre-fix Go writer would have left behind.
	legacy := map[string]any{
		"ID":            "balance-id",
		"Available":     "7777",
		"OnHold":        "123",
		"Version":       42,
		"OverdraftUsed": "250.50",
		"Direction":     "credit",
		// Legacy camelCase keys that must be dropped by the script.
		"allowOverdraft":        0,
		"overdraftLimitEnabled": 0,
		"overdraftLimit":        "0",
		"balanceScope":          "transactional",
	}
	payload, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, payload, time.Hour).Err())

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, 1, "1000.00", mmodel.BalanceScopeTransactional, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, result, "a valid blob must be written")

	var written map[string]any
	raw, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &written))

	// Settings-derived fields reflect the new values under Lua-native
	// CamelCase keys.
	assert.EqualValues(t, 1, written["AllowOverdraft"])
	assert.EqualValues(t, 1, written["OverdraftLimitEnabled"])
	assert.Equal(t, "1000.00", written["OverdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, written["BalanceScope"])

	// Legacy camelCase keys are purged.
	for _, legacyKey := range []string{"allowOverdraft", "overdraftLimitEnabled", "overdraftLimit", "balanceScope"} {
		_, present := written[legacyKey]
		assert.False(t, present, "legacy camelCase key %q must be removed from the cache document", legacyKey)
	}

	// Live transactional state and identity fields are preserved verbatim.
	assert.Equal(t, "balance-id", written["ID"])
	assert.Equal(t, "7777", written["Available"])
	assert.Equal(t, "123", written["OnHold"])
	assert.EqualValues(t, 42, written["Version"])
	assert.Equal(t, "250.50", written["OverdraftUsed"])
	assert.Equal(t, "credit", written["Direction"])

	ttl, err := infra.redisContainer.Client.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 86000*time.Second)
	assert.LessOrEqual(t, ttl, 86400*time.Second)
}

// TestIntegration_UpdateBalanceSettingsScript_ParityWithBalanceAtomicScript
// proves the encoding-parity claim behind moving the settings mutation into
// Lua: a cache blob produced by a REAL balance_atomic_operation.lua write
// (via ProcessBalanceAtomicOperation, not hand-seeded) round-trips through
// update_balance_settings.lua with its transactional fields byte-for-byte
// unchanged, because both scripts perform the exact same
// cjson.decode -> mutate -> cjson.encode step.
func TestIntegration_UpdateBalanceSettingsScript_ParityWithBalanceAtomicScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@settings-script-parity"
	balanceKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// A plain, non-overdraft credit-direction debit lets balance_atomic_operation.lua
	// NX-seed the key and mutate it for real, producing a blob this test did
	// not construct by hand.
	op := overdraftOp(orgID, ledgerID, alias, "deposit", "credit",
		decimal.NewFromInt(1000), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
	require.NoError(t, err)

	before := readCachedBalance(t, infra, internalKey)
	require.Equal(t, "900", before.Available)
	require.Equal(t, int64(2), before.Version, "the atomic write increments Version from the NX-seeded 1")

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{internalKey}, 1, 1, "50.00", mmodel.BalanceScopeInternal, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, result)

	after := readCachedBalance(t, infra, internalKey)

	// Settings applied.
	assert.Equal(t, 1, after.AllowOverdraft)
	assert.Equal(t, 1, after.OverdraftLimitEnabled)
	assert.Equal(t, "50.00", after.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeInternal, after.BalanceScope)

	// Transactional state written by the real atomic script is byte-identical.
	assert.Equal(t, before.Available, after.Available)
	assert.Equal(t, before.OnHold, after.OnHold)
	assert.Equal(t, before.Version, after.Version)
	assert.Equal(t, before.OverdraftUsed, after.OverdraftUsed)
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.Direction, after.Direction)
}
