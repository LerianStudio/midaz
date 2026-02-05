//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: All integration tests in this file have unit test counterparts with equivalent logic
// in update-balance_test.go. This is intentional and correct:
// - Unit tests mock Redis to test the algorithm quickly
// - Integration tests validate the same behavior against a real Redis instance via testcontainers

func TestIntegration_FilterStaleBalances_CacheNewerVersion_FiltersBalance(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)
	redisRepo, err := redis.NewConsumerRedis(conn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	// Pre-populate Redis with balance at version 10
	// SplitAliasWithKey("0#@account1#default") returns "@account1#default"
	balanceKey := "@account1#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKey)
	cachedBalance := mmodel.BalanceRedis{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          "@account1",
		AccountID:      libCommons.GenerateUUIDv7().String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        10,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		Key:            balanceKey,
	}
	data, err := json.Marshal(cachedBalance)
	require.NoError(t, err, "failed to marshal cached balance")
	require.NoError(t, container.Client.Set(ctx, internalKey, data, 0).Err(), "failed to set cached balance")

	// Balance to update with version 5 (older than cache)
	balances := []*mmodel.Balance{
		{
			ID:        libCommons.GenerateUUIDv7().String(),
			Alias:     "0#@account1#default", // SplitAliasWithKey returns "default"
			Version:   5,
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.Zero,
		},
	}

	// Act
	result := uc.filterStaleBalances(ctx, organizationID, ledgerID, balances, logger)

	// Assert
	assert.Empty(t, result, "balance should be filtered out when cache version (10) > update version (5)")
}

func TestIntegration_FilterStaleBalances_CacheOlderVersion_IncludesBalance(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)
	redisRepo, err := redis.NewConsumerRedis(conn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	// Pre-populate Redis with balance at version 3
	// SplitAliasWithKey("0#@account1#default") returns "@account1#default"
	balanceKey := "@account1#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKey)
	cachedBalance := mmodel.BalanceRedis{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          "@account1",
		AccountID:      libCommons.GenerateUUIDv7().String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        3,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		Key:            balanceKey,
	}
	data, err := json.Marshal(cachedBalance)
	require.NoError(t, err, "failed to marshal cached balance")
	require.NoError(t, container.Client.Set(ctx, internalKey, data, 0).Err(), "failed to set cached balance")

	// Balance to update with version 5 (newer than cache)
	balanceID := libCommons.GenerateUUIDv7().String()
	balances := []*mmodel.Balance{
		{
			ID:        balanceID,
			Alias:     "0#@account1#default",
			Version:   5,
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.Zero,
		},
	}

	// Act
	result := uc.filterStaleBalances(ctx, organizationID, ledgerID, balances, logger)

	// Assert
	require.Len(t, result, 1, "balance should be included when update version (5) > cache version (3)")
	assert.Equal(t, balanceID, result[0].ID)
}

func TestIntegration_FilterStaleBalances_CacheMiss_IncludesBalance(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)
	redisRepo, err := redis.NewConsumerRedis(conn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	// DO NOT pre-populate Redis - simulate cache miss

	balanceID := libCommons.GenerateUUIDv7().String()
	balances := []*mmodel.Balance{
		{
			ID:        balanceID,
			Alias:     "0#@account1#default",
			Version:   5,
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.Zero,
		},
	}

	// Act
	result := uc.filterStaleBalances(ctx, organizationID, ledgerID, balances, logger)

	// Assert: fail-open - include balance when cache unavailable
	require.Len(t, result, 1, "balance should be included when cache miss (fail-open)")
	assert.Equal(t, balanceID, result[0].ID)
}

func TestIntegration_FilterStaleBalances_MultipleBalances_FiltersOnlyStale(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)
	redisRepo, err := redis.NewConsumerRedis(conn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	// Pre-populate Redis with balances at different versions
	// SplitAliasWithKey("0#@account1#key1") returns "@account1#key1"

	// Balance 1: cache version 10 > update version 5 → filtered
	key1 := "@account1#key1"
	internalKey1 := utils.BalanceInternalKey(organizationID, ledgerID, key1)
	cached1 := mmodel.BalanceRedis{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          "@account1",
		AccountID:      libCommons.GenerateUUIDv7().String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        10,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		Key:            "key1",
	}
	data1, _ := json.Marshal(cached1)
	require.NoError(t, container.Client.Set(ctx, internalKey1, data1, 0).Err())

	// Balance 2: cache version 3 < update version 8 → included
	key2 := "@account2#key2"
	internalKey2 := utils.BalanceInternalKey(organizationID, ledgerID, key2)
	cached2 := mmodel.BalanceRedis{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          "@account2",
		AccountID:      libCommons.GenerateUUIDv7().String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(2000),
		OnHold:         decimal.Zero,
		Version:        3,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		Key:            "key2",
	}
	data2, _ := json.Marshal(cached2)
	require.NoError(t, container.Client.Set(ctx, internalKey2, data2, 0).Err())

	// Balance 3: no cache entry → included (fail-open)
	// @account3#key3 not set in Redis

	balance1ID := libCommons.GenerateUUIDv7().String()
	balance2ID := libCommons.GenerateUUIDv7().String()
	balance3ID := libCommons.GenerateUUIDv7().String()

	balances := []*mmodel.Balance{
		{
			ID:        balance1ID,
			Alias:     "0#@account1#key1", // → @account1#key1, cache v10 > update v5
			Version:   5,
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.Zero,
		},
		{
			ID:        balance2ID,
			Alias:     "1#@account2#key2", // → @account2#key2, cache v3 < update v8
			Version:   8,
			Available: decimal.NewFromInt(1800),
			OnHold:    decimal.Zero,
		},
		{
			ID:        balance3ID,
			Alias:     "2#@account3#key3", // → @account3#key3, cache miss
			Version:   1,
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
		},
	}

	// Act
	result := uc.filterStaleBalances(ctx, organizationID, ledgerID, balances, logger)

	// Assert
	require.Len(t, result, 2, "should include 2 balances (balance2 and balance3)")

	resultIDs := make([]string, len(result))
	for i, b := range result {
		resultIDs[i] = b.ID
	}
	assert.ElementsMatch(t, []string{balance2ID, balance3ID}, resultIDs,
		"should include balance2 (cache older) and balance3 (cache miss)")
}
