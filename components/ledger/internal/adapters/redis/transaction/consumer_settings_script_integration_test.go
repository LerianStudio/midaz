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

// These tests exercise the public settings PATCH and the raw CAS contract
// against the real engine (Valkey via testcontainers). JSON validation and
// replacement construction belong to the Go caller; Lua only compares the
// observed raw value and atomically installs the replacement.

func TestIntegration_UpdateBalanceSettingsScript_AbsentKeyIsNoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-script-test:" + uuid.NewString() // never seeded

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, "", `{"AllowOverdraft":1}`, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 0, result, "an absent key must be reported as a no-op, not an error")

	exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the script must not create the key on a no-op")
}

func TestIntegration_UpdateBalanceSettingsScript_DeleteMarkerBlocksMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "balance:{transactions}:settings-script-test:" + uuid.NewString()
	markerKey := deleteMarkerKeyForIntegration(key)
	original := `{"Available":"100","OnHold":"0","Version":1}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, original, time.Hour).Err())
	require.NoError(t, infra.redisContainer.Client.Set(ctx, markerKey, "owner", time.Hour).Err())

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, 1, "500.00", mmodel.BalanceScopeTransactional, "86400").Result()

	require.Error(t, err, "a live delete marker must reject settings mutation")
	assert.Contains(t, err.Error(), constant.ErrAccountIneligibility.Error())
	assert.Nil(t, result)

	unchanged, getErr := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, getErr)
	assert.Equal(t, original, unchanged, "the guarded settings update must leave the balance intact")
}

func TestIntegration_UpdateBalanceSettingsScript_LegacyDeleteMarkerBlocksMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "balance:{transactions}:settings-script-legacy-marker-test:" + uuid.NewString()
	original := `{"Available":"100","OnHold":"0","Version":1}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, original, time.Hour).Err())
	require.NoError(t, infra.redisContainer.Client.Set(ctx, legacyDeleteMarkerKeyForIntegration(key), "owner", time.Hour).Err())

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, 1, "500.00", mmodel.BalanceScopeTransactional, "86400").Result()

	require.Error(t, err, "a live legacy delete marker must reject settings mutation")
	assert.Contains(t, err.Error(), constant.ErrAccountIneligibility.Error())
	assert.Nil(t, result)

	unchanged, getErr := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, getErr)
	assert.Equal(t, original, unchanged, "the legacy-guarded settings update must leave the balance intact")
}

func TestIntegration_UpdateBalanceSettingsScript_TenantMarkerBlocksMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	tenantID := "settings-marker-tenant-" + uuid.NewString()
	ctx := context.Background()
	key := "balance:{transactions}:settings-script-test:" + uuid.NewString()
	physicalKey := "tenant:" + tenantID + ":" + key
	physicalMarkerKey := "tenant:" + tenantID + ":" + deleteMarkerKeyForIntegration(key)
	original := `{"Available":"100","OnHold":"0","Version":1}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, physicalKey, original, time.Hour).Err())
	require.NoError(t, infra.redisContainer.Client.Set(ctx, physicalMarkerKey, "owner", time.Hour).Err())

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{physicalKey}, 1, 1, "500.00", mmodel.BalanceScopeTransactional, "86400").Result()

	require.Error(t, err, "a tenant-prefixed delete marker must reject settings mutation")
	assert.Contains(t, err.Error(), constant.ErrAccountIneligibility.Error())
	assert.Nil(t, result)

	unchanged, getErr := infra.redisContainer.Client.Get(ctx, physicalKey).Result()
	require.NoError(t, getErr)
	assert.Equal(t, original, unchanged, "the tenant-guarded settings update must leave the balance intact")
}

func TestIntegration_UpdateBalanceSettingsScript_CorruptBlobReturnsErrorCodeAndLeavesValueIntact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	corrupt := "not-valid-json{{{"

	orgID := uuid.New()
	ledgerID := uuid.New()
	cacheKey := "@settings-script-corrupt#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, corrupt, time.Hour).Err())

	limit := "500.00"
	err := infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          mmodel.BalanceScopeTransactional,
	})
	require.Error(t, err, "a corrupt cached value must be rejected by the public PATCH")

	unchanged, err := infra.redisContainer.Client.Get(ctx, internalKey).Result()
	require.NoError(t, err)
	assert.Equal(t, corrupt, unchanged, "a corrupt blob must not be mutated")
}

func TestIntegration_UpdateBalanceSettingsScript_ValidBlobAppliesSettingsAndDedupesLegacyCasing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	cacheKey := "@settings-script-valid#default"
	key := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)

	// Legacy document carrying both live transactional state and camelCase
	// aliases a pre-fix Go writer would have left behind.
	legacy := map[string]any{
		"ID":            uuid.New().String(),
		"AccountID":     uuid.New().String(),
		"AccountType":   "liability",
		"AssetCode":     "USD",
		"Alias":         "@settings-script-valid",
		"Key":           cacheKey,
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

	limit := "1000.00"
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}))

	var written map[string]any
	raw, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &written))

	// Settings-derived fields reflect the new values in both canonical forms.
	assert.EqualValues(t, 1, written["AllowOverdraft"])
	assert.EqualValues(t, 1, written["OverdraftLimitEnabled"])
	assert.Equal(t, "1000", written["OverdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, written["BalanceScope"])
	assert.Equal(t, true, written["allowOverdraft"])
	assert.Equal(t, true, written["overdraftLimitEnabled"])
	assert.Equal(t, "1000", written["overdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, written["balanceScope"])

	// Historical all-lower spellings are purged.
	for _, legacyKey := range []string{"allowoverdraft", "overdraftlimitenabled", "overdraftlimit", "balancescope"} {
		_, present := written[legacyKey]
		assert.False(t, present, "historical lower spelling %q must be removed from the cache document", legacyKey)
	}

	// Live transactional state and identity fields are preserved verbatim.
	assert.NotEmpty(t, written["ID"])
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
// proves that the public settings PATCH can update a blob produced by the
// real balance_atomic_operation.lua write without changing live state.
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
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)

	before := readCachedBalance(t, infra, internalKey)
	require.Equal(t, "900", before.Available)
	require.Equal(t, int64(2), before.Version, "the atomic write increments Version from the NX-seeded 1")

	limit := "50.00"
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, balanceKey, &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          mmodel.BalanceScopeInternal,
	}))

	after := readCachedBalance(t, infra, internalKey)

	// Settings applied.
	assert.Equal(t, 1, after.AllowOverdraft)
	assert.Equal(t, 1, after.OverdraftLimitEnabled)
	assert.Equal(t, "50", after.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeInternal, after.BalanceScope)

	// Transactional state written by the real atomic script is byte-identical.
	assert.Equal(t, before.Available, after.Available)
	assert.Equal(t, before.OnHold, after.OnHold)
	assert.Equal(t, before.Version, after.Version)
	assert.Equal(t, before.OverdraftUsed, after.OverdraftUsed)
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.Direction, after.Direction)
}

func TestIntegration_UpdateBalanceSettingsScript_CASConflictLeavesValueAndTTLUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "settings-script-cas:" + uuid.NewString()
	observed := `{"Version":"7","allowOverdraft":false}`
	concurrent := `{"Version":"8","allowOverdraft":false}`
	replacement := `{"Version":"7","AllowOverdraft":1,"allowOverdraft":true}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, concurrent, 90*time.Minute).Err())
	expiresBefore, err := infra.redisContainer.Client.Do(ctx, "PEXPIRETIME", key).Int64()
	require.NoError(t, err)

	result, err := updateBalanceSettingsScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, observed, replacement, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 2, result)

	actual, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, concurrent, actual)
	expiresAfter, err := infra.redisContainer.Client.Do(ctx, "PEXPIRETIME", key).Int64()
	require.NoError(t, err)
	assert.Equal(t, expiresBefore, expiresAfter, "CAS conflict must not refresh TTL")
}
