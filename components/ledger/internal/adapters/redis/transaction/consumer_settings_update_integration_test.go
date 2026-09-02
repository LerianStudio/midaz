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

// TestIntegration_UpdateBalanceCacheSettings_HappyPath exercises
// UpdateBalanceCacheSettings end to end against a real Redis: the settings
// fields land, the live transactional state the Lua script owns is
// preserved verbatim, and the canonical 1-day TTL is (re)applied.
func TestIntegration_UpdateBalanceCacheSettings_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@settings-update-happy"
	balanceKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	seeded := cachedBalance{
		ID:                    uuid.New().String(),
		Available:             "7777",
		OnHold:                "123",
		Version:               42,
		AccountType:           "deposit",
		AccountID:             uuid.New().String(),
		AssetCode:             "USD",
		AllowSending:          1,
		AllowReceiving:        1,
		Key:                   balanceKey,
		Direction:             "credit",
		OverdraftUsed:         "250.50",
		AllowOverdraft:        0,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	payload, err := json.Marshal(seeded)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, payload, time.Hour).Err())

	limit := "1000.00"
	newSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, balanceKey, newSettings))

	final := readCachedBalance(t, infra, internalKey)

	// Settings-derived fields reflect the new payload.
	assert.Equal(t, 1, final.AllowOverdraft)
	assert.Equal(t, 1, final.OverdraftLimitEnabled)
	assert.Equal(t, "1000.00", final.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, final.BalanceScope)

	// Live transactional state is preserved verbatim.
	assert.Equal(t, "7777", final.Available)
	assert.Equal(t, "123", final.OnHold)
	assert.Equal(t, int64(42), final.Version)
	assert.Equal(t, "250.50", final.OverdraftUsed)
	assert.Equal(t, seeded.ID, final.ID)

	ttl, err := infra.redisContainer.Client.TTL(ctx, internalKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 86000*time.Second, "TTL must match the canonical 1-day settings TTL")
	assert.LessOrEqual(t, ttl, balanceCacheSettingsTTL)
}

// TestIntegration_UpdateBalanceCacheSettings_CacheMissIsNoOp verifies against
// a real Redis that a key with no cache entry is left untouched: no key is
// created, matching the documented "next transaction reloads from
// PostgreSQL" contract.
func TestIntegration_UpdateBalanceCacheSettings_CacheMissIsNoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceKey := "@settings-update-miss#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	err := infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, balanceKey,
		&mmodel.BalanceSettings{AllowOverdraft: true})
	require.NoError(t, err)

	exists, err := infra.redisContainer.Client.Exists(ctx, internalKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a settings update on a missing key must not create one")
}
