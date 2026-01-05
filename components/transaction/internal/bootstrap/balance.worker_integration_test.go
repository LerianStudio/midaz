//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	redistestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// INTEGRATION TESTS - BALANCE SYNC SCHEDULE
// =============================================================================

// TestIntegration_BalanceSyncSchedule_FullFlow tests the complete schedule lifecycle:
// 1. Add a balance key to the sync schedule (sorted set)
// 2. Verify GetBalanceSyncKeys returns it when due
// 3. Verify RemoveBalanceSyncKey removes it
func TestIntegration_BalanceSyncSchedule_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup Redis container
	container := redistestutil.SetupContainer(t)
	conn := redistestutil.CreateConnection(t, container.Addr)

	repo, err := redis.NewConsumerRedis(conn, true)
	require.NoError(t, err, "should create Redis repository")

	ctx := context.Background()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create a balance key following the pattern used by the worker
	balanceKey := utils.BalanceInternalKey(orgID, ledgerID, "default")

	// Step 1: Store balance data in Redis
	t.Log("Step 1: Storing balance data in Redis")
	balanceData := mmodel.BalanceRedis{
		ID:        libCommons.GenerateUUIDv7().String(),
		Alias:     "@test-sync",
		AccountID: libCommons.GenerateUUIDv7().String(),
		AssetCode: "USD",
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
		Version:   1,
	}
	balanceJSON, err := json.Marshal(balanceData)
	require.NoError(t, err)

	err = repo.Set(ctx, balanceKey, string(balanceJSON), 3600) // 1 hour TTL
	require.NoError(t, err, "should set balance data")

	// Step 2: Add key to the sync schedule with a past timestamp (immediately due)
	t.Log("Step 2: Adding key to sync schedule (past timestamp = immediately due)")
	pastScore := float64(time.Now().Add(-1 * time.Minute).Unix())
	_, err = container.Client.ZAdd(ctx, utils.BalanceSyncScheduleKey, goredis.Z{
		Score:  pastScore,
		Member: balanceKey,
	}).Result()
	require.NoError(t, err, "should add key to schedule")

	// Step 3: Verify GetBalanceSyncKeys returns our key
	t.Log("Step 3: Verifying GetBalanceSyncKeys returns the scheduled key")
	keys, err := repo.GetBalanceSyncKeys(ctx, 10)
	require.NoError(t, err, "GetBalanceSyncKeys should succeed")
	assert.Contains(t, keys, balanceKey, "should return our scheduled key")
	t.Logf("GetBalanceSyncKeys returned %d keys", len(keys))

	// Step 4: Remove the key from schedule
	t.Log("Step 4: Removing key from schedule")
	err = repo.RemoveBalanceSyncKey(ctx, balanceKey)
	require.NoError(t, err, "RemoveBalanceSyncKey should succeed")

	// Step 5: Verify key is no longer in schedule
	t.Log("Step 5: Verifying key is removed from schedule")
	keysAfter, err := repo.GetBalanceSyncKeys(ctx, 10)
	require.NoError(t, err)
	assert.NotContains(t, keysAfter, balanceKey, "key should be removed from schedule")

	t.Log("Integration test passed: balance sync schedule full flow verified")
}

// TestIntegration_BalanceSyncWorker_TTLBehavior tests TTL detection:
// 1. Create a key with short TTL
// 2. Verify TTL returns positive value
// 3. Poll until key expires (avoids flaky time.Sleep)
// 4. Verify TTL returns -2 (key doesn't exist)
func TestIntegration_BalanceSyncWorker_TTLBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup Redis container
	container := redistestutil.SetupContainer(t)

	ctx := context.Background()
	testKey := "test:ttl:behavior:" + libCommons.GenerateUUIDv7().String()

	// Step 1: Set key with 2 second TTL
	t.Log("Step 1: Setting key with 2 second TTL")
	err := container.Client.Set(ctx, testKey, "test-value", 2*time.Second).Err()
	require.NoError(t, err, "should set key with TTL")

	// Step 2: Verify TTL is positive
	t.Log("Step 2: Verifying TTL is positive")
	ttl, err := container.Client.TTL(ctx, testKey).Result()
	require.NoError(t, err, "TTL should succeed")
	assert.Greater(t, ttl, time.Duration(0), "TTL should be positive")
	assert.LessOrEqual(t, ttl, 2*time.Second, "TTL should be <= 2 seconds")
	t.Logf("Initial TTL: %v", ttl)

	// Step 3: Poll until key expires (condition-based waiting, not fixed sleep)
	t.Log("Step 3: Polling until key expires")
	require.Eventually(t, func() bool {
		ttlCheck, _ := container.Client.TTL(ctx, testKey).Result()
		return ttlCheck == -2*time.Nanosecond
	}, 5*time.Second, 100*time.Millisecond, "key should expire within timeout")

	// Step 4: Verify TTL returns -2 (key doesn't exist)
	t.Log("Step 4: Verifying TTL returns -2 for expired key")
	ttlAfter, err := container.Client.TTL(ctx, testKey).Result()
	require.NoError(t, err, "TTL should succeed even for missing key")

	// Redis returns -2 for non-existent keys
	// go-redis represents this as -2 * time.Nanosecond = -2ns
	assert.Equal(t, -2*time.Nanosecond, ttlAfter,
		"TTL should return -2ns for expired/missing key (got %v)", ttlAfter)
	t.Logf("TTL after expiration: %v", ttlAfter)

	t.Log("Integration test passed: TTL behavior verified")
}

// TestIntegration_BalanceSyncSchedule_FutureKeys tests that future-scheduled keys
// are NOT returned by GetBalanceSyncKeys until they're due.
func TestIntegration_BalanceSyncSchedule_FutureKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup Redis container
	container := redistestutil.SetupContainer(t)
	conn := redistestutil.CreateConnection(t, container.Addr)

	repo, err := redis.NewConsumerRedis(conn, true)
	require.NoError(t, err, "should create Redis repository")

	ctx := context.Background()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create keys: one due now, one due in the future
	dueNowKey := utils.BalanceInternalKey(orgID, ledgerID, "due-now")
	futureDueKey := utils.BalanceInternalKey(orgID, ledgerID, "future-due")

	// Step 1: Add key with past score (immediately due)
	t.Log("Step 1: Adding key with past score (immediately due)")
	pastScore := float64(time.Now().Add(-1 * time.Minute).Unix())
	_, err = container.Client.ZAdd(ctx, utils.BalanceSyncScheduleKey, goredis.Z{
		Score:  pastScore,
		Member: dueNowKey,
	}).Result()
	require.NoError(t, err)

	// Step 2: Add key with future score (not due yet)
	t.Log("Step 2: Adding key with future score (1 hour from now)")
	futureScore := float64(time.Now().Add(1 * time.Hour).Unix())
	_, err = container.Client.ZAdd(ctx, utils.BalanceSyncScheduleKey, goredis.Z{
		Score:  futureScore,
		Member: futureDueKey,
	}).Result()
	require.NoError(t, err)

	// Step 3: Verify only the due key is returned
	t.Log("Step 3: Verifying only past-due key is returned")
	keys, err := repo.GetBalanceSyncKeys(ctx, 10)
	require.NoError(t, err)

	assert.Contains(t, keys, dueNowKey, "should return the due key")
	assert.NotContains(t, keys, futureDueKey, "should NOT return the future key")
	t.Logf("GetBalanceSyncKeys returned %d keys (expected 1)", len(keys))

	// Cleanup
	_ = repo.RemoveBalanceSyncKey(ctx, dueNowKey)
	_ = repo.RemoveBalanceSyncKey(ctx, futureDueKey)

	t.Log("Integration test passed: future keys correctly filtered")
}
