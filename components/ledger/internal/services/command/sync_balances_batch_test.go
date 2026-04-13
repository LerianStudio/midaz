// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// toSyncKeys converts a string slice to SyncKey slice for test convenience.
// Each key gets a distinct score (1001, 1002, ...) so tests can detect if
// SyncBalancesBatch or RemoveBalanceSyncKeysBatch drops, rewrites, or reorders scores.
func toSyncKeys(keys []string) []redis.SyncKey {
	syncKeys := make([]redis.SyncKey, len(keys))
	for i, k := range keys {
		syncKeys[i] = redis.SyncKey{Key: k, Score: float64(1001 + i)}
	}

	return syncKeys
}

// syncKeyScore returns the score that toSyncKeys assigns to the i-th key (0-indexed).
func syncKeyScore(i int) float64 {
	return float64(1001 + i)
}

// TestSyncBalancesBatch_EmptyKeys verifies that when given empty keys,
// the use case returns immediately with zero synced and no error.
func TestSyncBalancesBatch_EmptyKeys(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	uc := UseCase{}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, []redis.SyncKey{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.KeysProcessed)
	assert.Equal(t, 0, result.BalancesAggregated)
	assert.Equal(t, int64(0), result.BalancesSynced)
	assert.Equal(t, int64(0), result.KeysRemoved)
}

// TestSyncBalancesBatch_AllKeysExpired verifies that when all keys have expired
// (nil balance data), the orphaned keys are cleaned up from the schedule.
func TestSyncBalancesBatch_AllKeysExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc2#default",
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(map[string]*mmodel.BalanceRedis{
			keys[0]: nil, // expired
			keys[1]: nil, // expired
		}, nil).
		Times(1)

	// With the fix: orphaned keys are cleaned up even when no valid balances exist
	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []redis.SyncKey) (int64, error) {
			assert.Len(t, keysToRemove, 2, "Both expired keys should be removed")
			for i, sk := range keysToRemove {
				assert.Equal(t, keys[i], sk.Key, "Key mismatch at index %d", i)
				assert.Equal(t, syncKeyScore(i), sk.Score, "Score mismatch at index %d — claimed score must be preserved", i)
			}

			return 2, nil
		}).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed)
	assert.Equal(t, 0, result.BalancesAggregated)
	assert.Equal(t, int64(0), result.BalancesSynced)
	assert.Equal(t, int64(2), result.KeysRemoved, "Orphaned keys should be removed")
}

// TestSyncBalancesBatch_SuccessWithAggregation verifies the full flow:
// fetch balances, aggregate, persist to DB, and remove from schedule.
func TestSyncBalancesBatch_SuccessWithAggregation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID1 := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID2 := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc2#default",
	}

	balanceData := map[string]*mmodel.BalanceRedis{
		keys[0]: {
			ID:        balanceID1.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
		keys[1]: {
			ID:        balanceID2.String(),
			Alias:     "@acc2",
			AssetCode: "USD",
			Version:   3,
			Available: decimal.NewFromInt(500),
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	// Step 1: Fetch balances from Redis
	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Step 2: Persist to database
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 2)
			return 2, nil
		}).
		Times(1)

	// Step 3: Remove from schedule
	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(2), nil).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed)
	assert.Equal(t, 2, result.BalancesAggregated)
	assert.Equal(t, int64(2), result.BalancesSynced)
	assert.Equal(t, int64(2), result.KeysRemoved)
}

// TestSyncBalancesBatch_PartialData verifies that when some keys have data
// and others are nil (expired), only valid balances are synced but all keys are removed.
func TestSyncBalancesBatch_PartialData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc2#default",
	}

	balanceData := map[string]*mmodel.BalanceRedis{
		keys[0]: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
		keys[1]: nil, // expired - orphaned key
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1)
			return 1, nil
		}).
		Times(1)

	// Both keys removed: 1 valid + 1 orphaned
	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(2), nil).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed)
	assert.Equal(t, 1, result.BalancesAggregated)
	assert.Equal(t, int64(1), result.BalancesSynced)
}

// TestSyncBalancesBatch_RedisError verifies that when Redis fetch fails,
// the error is propagated and no DB write is attempted.
func TestSyncBalancesBatch_RedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(nil, errors.New("redis connection error")).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "redis connection error")
}

// TestSyncBalancesBatch_DBError verifies that when DB persist fails,
// the error is propagated and keys are not removed from schedule.
func TestSyncBalancesBatch_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
	}

	balanceData := map[string]*mmodel.BalanceRedis{
		keys[0]: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(0), errors.New("database connection error")).
		Times(1)

	// RemoveBalanceSyncKeysBatch should NOT be called when DB fails

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.Error(t, err)
	assert.NotNil(t, result, "Result should be returned even on DB error (contains partial metrics)")
	assert.Equal(t, int64(0), result.BalancesSynced, "No balances should be synced on DB error")
	assert.Equal(t, int64(0), result.KeysRemoved, "No keys removed (no orphaned keys in this test)")
	assert.Contains(t, err.Error(), "database connection error")
}

// TestSyncBalancesBatch_ScheduleCleanupFailure verifies that when schedule cleanup fails,
// the operation still succeeds (balances are already persisted).
func TestSyncBalancesBatch_ScheduleCleanupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
	}

	balanceData := map[string]*mmodel.BalanceRedis{
		keys[0]: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("redis cleanup error")).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	// Should succeed despite cleanup failure
	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.BalancesSynced)
	assert.Equal(t, int64(0), result.KeysRemoved) // cleanup failed
}

// TestSyncBalancesBatch_AggregationKeepsHighestVersion verifies idempotent handling
// when the same Redis key appears multiple times in a batch. With the Sorted Set
// scheduling mechanism, duplicate keys map to the same balance data, and the
// aggregator deduplicates them to a single entry. Version-based deduplication
// is thoroughly tested in aggregation_test.go where inputs are constructed
// directly without Redis layer constraints.
func TestSyncBalancesBatch_AggregationKeepsHighestVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	// Same Redis key appearing twice in the batch (simulates duplicate scheduling).
	// With Sorted Set architecture, identical keys return the same balance from Redis.
	key1 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default"
	key2 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default"

	keys := []string{key1, key2}

	// Since key1 == key2, this map has a single entry (Go map semantics).
	// Both iterations in the loop retrieve the same balance data.
	balanceData := map[string]*mmodel.BalanceRedis{
		key1: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5, // This entry is overwritten by key2 (same key)
			Available: decimal.NewFromInt(1000),
		},
		key2: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   10, // Only this entry exists in the map
			Available: decimal.NewFromInt(2000),
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Verify that only 1 balance is synced after deduplication of identical keys
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1, "Expected exactly 1 balance after deduplication")
			assert.Equal(t, int64(10), balances[0].Version, "Expected the single map entry's version")
			assert.Equal(t, decimal.NewFromInt(2000), balances[0].Available, "Expected balance with higher version")
			return 1, nil
		}).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed, "Both key entries in slice were processed")
	assert.Equal(t, 1, result.BalancesAggregated, "Deduplicated to 1 balance (same composite key)")
	assert.Equal(t, int64(1), result.BalancesSynced)
}

// TestSyncBalancesBatch_InvalidKeyFormat verifies that malformed Redis keys
// are gracefully skipped for processing but still removed from the schedule.
func TestSyncBalancesBatch_InvalidKeyFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	// Mix of valid and invalid keys
	validKey := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default"
	invalidKey1 := "invalid:key:format"                                                        // Too few parts
	invalidKey2 := "balance:{transactions}:not-a-uuid:" + ledgerID.String() + ":@acc2#default" // Invalid org UUID
	invalidKey3 := "balance:{transactions}:" + organizationID.String() + ":not-a-uuid:@acc3#x" // Invalid ledger UUID

	keys := []string{validKey, invalidKey1, invalidKey2, invalidKey3}

	balanceData := map[string]*mmodel.BalanceRedis{
		validKey: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
		invalidKey1: {
			ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:     "@acc2",
			AssetCode: "USD",
			Version:   3,
		},
		invalidKey2: {
			ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:     "@acc3",
			AssetCode: "USD",
			Version:   2,
		},
		invalidKey3: {
			ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:     "@acc4",
			AssetCode: "USD",
			Version:   1,
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Only the valid key should result in a balance being synced
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1, "Only valid key should produce a balance")
			assert.Equal(t, balanceID.String(), balances[0].ID)
			return 1, nil
		}).
		Times(1)

	// All 4 keys removed: 1 valid + 3 invalid (orphaned due to parse errors)
	// Build expected score map from input so we can verify scores are preserved through the pipeline
	inputSyncKeys := toSyncKeys(keys)
	expectedScores := make(map[string]float64, len(inputSyncKeys))
	for _, sk := range inputSyncKeys {
		expectedScores[sk.Key] = sk.Score
	}

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []redis.SyncKey) (int64, error) {
			assert.Len(t, keysToRemove, 4, "All keys should be removed (valid + invalid)")
			removedStrs := make([]string, len(keysToRemove))
			for i, k := range keysToRemove {
				removedStrs[i] = k.Key
				assert.Equal(t, expectedScores[k.Key], k.Score, "Score for %s must be preserved from input", k.Key)
			}
			assert.Contains(t, removedStrs, validKey, "valid key should be removed")
			assert.Contains(t, removedStrs, invalidKey1, "invalid key 1 should be removed")
			assert.Contains(t, removedStrs, invalidKey2, "invalid key 2 should be removed")
			assert.Contains(t, removedStrs, invalidKey3, "invalid key 3 should be removed")
			return 4, nil
		}).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 4, result.KeysProcessed, "All keys were attempted")
	assert.Equal(t, 1, result.BalancesAggregated, "Only valid key produced a balance")
	assert.Equal(t, int64(1), result.BalancesSynced)
	assert.Equal(t, int64(4), result.KeysRemoved, "All 4 keys removed")
}

// TestSyncBalancesBatch_ExactKeysRemoved verifies that all processed keys
// (both valid and orphaned) are removed from the schedule.
func TestSyncBalancesBatch_ExactKeysRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID1 := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID2 := uuid.Must(libCommons.GenerateUUIDv7())

	key1 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default"
	key2 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc2#default"
	key3 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc3#default"

	keys := []string{key1, key2, key3}

	balanceData := map[string]*mmodel.BalanceRedis{
		key1: {
			ID:        balanceID1.String(),
			Alias:     "@acc1",
			AssetCode: "USD",
			Version:   5,
		},
		key2: nil, // Expired - orphaned key, MUST be removed to prevent infinite loop
		key3: {
			ID:        balanceID2.String(),
			Alias:     "@acc3",
			AssetCode: "USD",
			Version:   3,
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(2), nil).
		Times(1)

	// Verify ALL keys are removed: key1, key2 (orphaned), and key3
	inputSyncKeys := toSyncKeys(keys)
	expectedScores := make(map[string]float64, len(inputSyncKeys))
	for _, sk := range inputSyncKeys {
		expectedScores[sk.Key] = sk.Score
	}

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []redis.SyncKey) (int64, error) {
			assert.Len(t, keysToRemove, 3, "Expected all 3 keys to be removed")
			removedStrs := make([]string, len(keysToRemove))
			for i, k := range keysToRemove {
				removedStrs[i] = k.Key
				assert.Equal(t, expectedScores[k.Key], k.Score, "Score for %s must be preserved", k.Key)
			}
			assert.Contains(t, removedStrs, key1, "key1 should be in removal list")
			assert.Contains(t, removedStrs, key2, "key2 (orphaned) MUST be in removal list")
			assert.Contains(t, removedStrs, key3, "key3 should be in removal list")
			return 3, nil
		}).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.KeysProcessed)
	assert.Equal(t, 2, result.BalancesAggregated)
	assert.Equal(t, int64(2), result.BalancesSynced)
	assert.Equal(t, int64(3), result.KeysRemoved, "All 3 keys removed (2 valid + 1 orphaned)")
}

// TestSyncBalancesBatch_ContextCancellation verifies that the operation respects
// context cancellation and returns appropriate error.
func TestSyncBalancesBatch_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockRedis := redis.NewMockRedisRepository(ctrl)

	// Redis should receive the canceled context and return context error
	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(nil, context.Canceled).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestSyncBalancesBatch_OrphanedKeysCleanedUp verifies that keys with nil balance values
// (expired TTL) are removed from the schedule to prevent infinite reprocessing loops.
// Bug fix: Previously these keys were skipped but never added to keysToRemove.
func TestSyncBalancesBatch_OrphanedKeysCleanedUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	// Mix of valid keys and orphaned keys (nil balance - expired TTL)
	validKey := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@valid-account#default"
	orphanedKey1 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@orphaned-1#default"
	orphanedKey2 := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@orphaned-2#default"

	keys := []string{validKey, orphanedKey1, orphanedKey2}

	balanceData := map[string]*mmodel.BalanceRedis{
		validKey: {
			ID:        balanceID.String(),
			Alias:     "@valid-account",
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
		orphanedKey1: nil, // Expired - orphaned key
		orphanedKey2: nil, // Expired - orphaned key
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Only valid balance is synced to DB
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1, "Only valid key should produce a balance")
			return 1, nil
		}).
		Times(1)

	// KEY ASSERTION: ALL 3 keys should be removed (1 valid + 2 orphaned)
	// This is the bug fix: orphaned keys MUST be in removal list
	inputSyncKeys := toSyncKeys(keys)
	expectedScores := make(map[string]float64, len(inputSyncKeys))
	for _, sk := range inputSyncKeys {
		expectedScores[sk.Key] = sk.Score
	}

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []redis.SyncKey) (int64, error) {
			assert.Len(t, keysToRemove, 3, "Expected 3 keys to be removed (1 valid + 2 orphaned)")
			removedStrs := make([]string, len(keysToRemove))
			for i, k := range keysToRemove {
				removedStrs[i] = k.Key
				assert.Equal(t, expectedScores[k.Key], k.Score, "Score for %s must be preserved", k.Key)
			}
			assert.Contains(t, removedStrs, validKey, "valid key should be in removal list")
			assert.Contains(t, removedStrs, orphanedKey1, "orphaned key 1 should be in removal list")
			assert.Contains(t, removedStrs, orphanedKey2, "orphaned key 2 should be in removal list")
			return 3, nil
		}).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.KeysProcessed, "All 3 keys were processed")
	assert.Equal(t, 1, result.BalancesAggregated, "Only 1 valid balance aggregated")
	assert.Equal(t, int64(1), result.BalancesSynced)
	assert.Equal(t, int64(3), result.KeysRemoved, "All 3 keys removed (including orphaned)")
}

// TestSyncBalancesBatch_MalformedKeyWithTrailingHash verifies that keys with trailing #
// (empty partition key) are handled correctly by falling back to BalanceRedis.Key.
// This tests the bug fix for the infinite sync loop caused by malformed cache keys.
func TestSyncBalancesBatch_MalformedKeyWithTrailingHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed key with trailing # (empty partition key)
	malformedKey := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#"

	keys := []string{malformedKey}

	balanceData := map[string]*mmodel.BalanceRedis{
		malformedKey: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			Key:       "asset-freeze", // The actual key from BalanceRedis should be used
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Verify the balance is synced with the correct key from BalanceRedis
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1)
			assert.Equal(t, "asset-freeze", balances[0].Key, "Should use BalanceRedis.Key, not parsed empty key")
			return 1, nil
		}).
		Times(1)

	// KEY ASSERTION: Verify malformed key is removed from schedule to break the infinite loop.
	// Before this fix, the key would remain in the schedule and be reprocessed indefinitely.
	inputSyncKeys := toSyncKeys(keys)
	expectedScores := make(map[string]float64, len(inputSyncKeys))
	for _, sk := range inputSyncKeys {
		expectedScores[sk.Key] = sk.Score
	}

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []redis.SyncKey) (int64, error) {
			removedStrs := make([]string, len(keysToRemove))
			for i, k := range keysToRemove {
				removedStrs[i] = k.Key
				assert.Equal(t, expectedScores[k.Key], k.Score, "Score for %s must be preserved", k.Key)
			}
			assert.Contains(t, removedStrs, malformedKey, "Malformed key MUST be removed from schedule to prevent infinite loop")
			return int64(len(keysToRemove)), nil
		}).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.KeysProcessed)
	assert.Equal(t, 1, result.BalancesAggregated)
	assert.Equal(t, int64(1), result.BalancesSynced)
	assert.Equal(t, int64(1), result.KeysRemoved, "Malformed key must be removed to break sync loop")
}

// TestSyncBalancesBatch_MalformedKeyFallbackToDefault verifies that when parsed partition
// is empty/default AND BalanceRedis.Key is also default, no unnecessary override happens.
func TestSyncBalancesBatch_MalformedKeyFallbackToDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed key with trailing # (empty partition key)
	malformedKey := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#"

	keys := []string{malformedKey}

	balanceData := map[string]*mmodel.BalanceRedis{
		malformedKey: {
			ID:        balanceID.String(),
			Alias:     "@acc1",
			Key:       "default", // BalanceRedis.Key is also "default"
			AssetCode: "USD",
			Version:   5,
			Available: decimal.NewFromInt(1000),
		},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	// Verify the balance is synced with "default" key
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1)
			assert.Equal(t, "default", balances[0].Key, "Should keep default when both parsed and BalanceRedis.Key are default")
			return 1, nil
		}).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	uc := UseCase{
		TransactionRedisRepo: mockRedis,
		BalanceRepo:          mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, toSyncKeys(keys))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.KeysProcessed)
	assert.Equal(t, 1, result.BalancesAggregated)
	assert.Equal(t, int64(1), result.BalancesSynced)
}
