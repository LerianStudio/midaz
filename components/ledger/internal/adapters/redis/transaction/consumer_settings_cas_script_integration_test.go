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
)

// These tests exercise scripts/update_balance_settings_cas.lua directly
// against the real engine (Valkey via testcontainers), independent of
// UpdateBalanceCacheSettings, to lock the three-way CAS contract the retry
// loop depends on: match commits, mismatch conflicts without mutating the
// key, and an absent key is a no-op rather than an error.

func TestIntegration_UpdateBalanceSettingsCASScript_MatchWritesAndSetsTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-cas-test:" + uuid.NewString()
	current := `{"Available":"100","Version":1}`
	next := `{"Available":"100","Version":1,"AllowOverdraft":1}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, current, time.Hour).Err())

	result, err := updateBalanceSettingsCASScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, current, next, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, result, "a matching expected value must commit the write")

	written, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, next, written, "the key must hold the new blob after a successful CAS")

	ttl, err := infra.redisContainer.Client.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 86000*time.Second, "TTL must be (re)applied on a successful CAS write")
	assert.LessOrEqual(t, ttl, 86400*time.Second)
}

func TestIntegration_UpdateBalanceSettingsCASScript_MismatchConflictsAndLeavesBlobIntact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-cas-test:" + uuid.NewString()
	actual := `{"Available":"100","Version":2}` // a concurrent writer already bumped Version
	staleExpected := `{"Available":"100","Version":1}`
	attemptedWrite := `{"Available":"100","Version":1,"AllowOverdraft":1}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, actual, time.Hour).Err())

	result, err := updateBalanceSettingsCASScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, staleExpected, attemptedWrite, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, -1, result, "a stale expected value must be reported as a conflict")

	unchanged, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, actual, unchanged, "a conflicting CAS must not mutate the key")
}

func TestIntegration_UpdateBalanceSettingsCASScript_AbsentKeyIsNoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "settings-cas-test:" + uuid.NewString() // never seeded

	result, err := updateBalanceSettingsCASScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, `{"Available":"100"}`, `{"Available":"100","AllowOverdraft":1}`, "86400").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 0, result, "an absent key must be reported as a no-op, not an error")

	exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the CAS must not create the key on a no-op")
}
