// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestSyncBalancesBatch_EmptyKeys verifies that when given empty keys,
// the use case returns immediately with zero synced and no error.
func TestSyncBalancesBatch_EmptyKeys(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	uc := UseCase{}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, []string{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.KeysProcessed)
	assert.Equal(t, 0, result.BalancesAggregated)
	assert.Equal(t, int64(0), result.BalancesSynced)
	assert.Equal(t, int64(0), result.KeysRemoved)
}

// TestSyncBalancesBatch_AllKeysExpired verifies that when all keys have expired
// (nil balance data), the use case returns zero synced without error.
func TestSyncBalancesBatch_AllKeysExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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

	uc := UseCase{
		RedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed)
	assert.Equal(t, 0, result.BalancesAggregated)
	assert.Equal(t, int64(0), result.BalancesSynced)
}

// TestSyncBalancesBatch_SuccessWithAggregation verifies the full flow:
// fetch balances, aggregate, persist to DB, and remove from schedule.
func TestSyncBalancesBatch_SuccessWithAggregation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID1 := libCommons.GenerateUUIDv7()
	balanceID2 := libCommons.GenerateUUIDv7()

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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
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
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed)
	assert.Equal(t, 2, result.BalancesAggregated)
	assert.Equal(t, int64(2), result.BalancesSynced)
	assert.Equal(t, int64(2), result.KeysRemoved)
}

// TestSyncBalancesBatch_PartialData verifies that when some keys have data
// and others are nil (expired), only valid balances are processed.
func TestSyncBalancesBatch_PartialData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

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
		keys[1]: nil, // expired
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(balanceData, nil).
		Times(1)

	mockBalance.EXPECT().
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1)
			return 1, nil
		}).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	keys := []string{
		"balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default",
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)

	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), keys).
		Return(nil, errors.New("redis connection error")).
		Times(1)

	uc := UseCase{
		RedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "redis connection error")
}

// TestSyncBalancesBatch_DBError verifies that when DB persist fails,
// the error is propagated and keys are not removed from schedule.
func TestSyncBalancesBatch_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(0), errors.New("database connection error")).
		Times(1)

	// RemoveBalanceSyncKeysBatch should NOT be called when DB fails

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection error")
}

// TestSyncBalancesBatch_ScheduleCleanupFailure verifies that when schedule cleanup fails,
// the operation still succeeds (balances are already persisted).
func TestSyncBalancesBatch_ScheduleCleanupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("redis cleanup error")).
		Times(1)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	// Should succeed despite cleanup failure
	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
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
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.KeysProcessed, "Both key entries in slice were processed")
	assert.Equal(t, 1, result.BalancesAggregated, "Deduplicated to 1 balance (same composite key)")
	assert.Equal(t, int64(1), result.BalancesSynced)
}

// TestSyncBalancesBatch_InvalidKeyFormat verifies that malformed Redis keys
// are gracefully skipped without failing the entire batch operation.
func TestSyncBalancesBatch_InvalidKeyFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

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
			ID:        libCommons.GenerateUUIDv7().String(),
			Alias:     "@acc2",
			AssetCode: "USD",
			Version:   3,
		},
		invalidKey2: {
			ID:        libCommons.GenerateUUIDv7().String(),
			Alias:     "@acc3",
			AssetCode: "USD",
			Version:   2,
		},
		invalidKey3: {
			ID:        libCommons.GenerateUUIDv7().String(),
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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			assert.Len(t, balances, 1, "Only valid key should produce a balance")
			assert.Equal(t, balanceID.String(), balances[0].ID)
			return 1, nil
		}).
		Times(1)

	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []string) (int64, error) {
			assert.Len(t, keysToRemove, 1, "Only valid key should be in removal list")
			assert.Equal(t, validKey, keysToRemove[0])
			return 1, nil
		}).
		Times(1)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 4, result.KeysProcessed, "All keys were attempted")
	assert.Equal(t, 1, result.BalancesAggregated, "Only valid key produced a balance")
	assert.Equal(t, int64(1), result.BalancesSynced)
	assert.Equal(t, int64(1), result.KeysRemoved)
}

// TestSyncBalancesBatch_ExactKeysRemoved verifies that exactly the synced keys
// are passed to RemoveBalanceSyncKeysBatch (not more, not less).
func TestSyncBalancesBatch_ExactKeysRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID1 := libCommons.GenerateUUIDv7()
	balanceID2 := libCommons.GenerateUUIDv7()

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
		key2: nil, // Expired - should not be in removal list
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
		SyncBatch(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(int64(2), nil).
		Times(1)

	// Verify EXACT keys are removed: only key1 and key3 (not key2 which was nil)
	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keysToRemove []string) (int64, error) {
			assert.Len(t, keysToRemove, 2, "Expected exactly 2 keys to be removed")
			assert.Contains(t, keysToRemove, key1, "key1 should be in removal list")
			assert.Contains(t, keysToRemove, key3, "key3 should be in removal list")
			assert.NotContains(t, keysToRemove, key2, "key2 (expired) should NOT be in removal list")
			return 2, nil
		}).
		Times(1)

	uc := UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalance,
	}

	result, err := uc.SyncBalancesBatch(context.TODO(), organizationID, ledgerID, keys)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.KeysProcessed)
	assert.Equal(t, 2, result.BalancesAggregated)
	assert.Equal(t, int64(2), result.BalancesSynced)
	assert.Equal(t, int64(2), result.KeysRemoved)
}

// TestSyncBalancesBatch_ContextCancellation verifies that the operation respects
// context cancellation and returns appropriate error.
func TestSyncBalancesBatch_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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
		RedisRepo: mockRedis,
	}

	result, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, keys)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}
