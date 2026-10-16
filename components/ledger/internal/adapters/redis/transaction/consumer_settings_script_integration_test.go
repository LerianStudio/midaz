//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// These tests exercise the raw CAS contract of the settings compare-and-set
// script, which the configuration copy into the blob uses, against the real
// engine (Valkey via testcontainers). JSON validation and replacement
// construction belong to the Go caller; Lua only compares the observed raw
// value and atomically installs the replacement.

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
