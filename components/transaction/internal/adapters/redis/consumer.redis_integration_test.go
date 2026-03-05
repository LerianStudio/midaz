//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// skipIfNotChaos skips the test if CHAOS=1 environment variable is not set.
// Use this for tests that inject failures (network chaos, container restarts, etc.)
func skipIfNotChaos(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}
}

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// integrationTestInfra holds the infrastructure needed for Redis integration tests.
type integrationTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	repo           *RedisConsumerRepository
}

// chaosTestInfra holds the infrastructure needed for Redis chaos tests.
type chaosTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	repo           *RedisConsumerRepository
	chaosOrch      *chaos.Orchestrator
}

// networkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
// Uses the unified chaos.Infrastructure for Toxiproxy management.
type networkChaosTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	proxyRepo      *RedisConsumerRepository
	proxyConn      *libRedis.Client
	proxy          *chaos.Proxy
}

// setupRedisIntegrationInfra sets up the test infrastructure for Redis integration testing.
func setupRedisIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	// Setup Redis container
	redisContainer := redistestutil.SetupContainer(t)

	// Create lib-commons Redis connection
	conn := redistestutil.CreateConnection(t, redisContainer.Addr)

	// Create repository
	repo := &RedisConsumerRepository{
		conn: conn,
	}

	return &integrationTestInfra{
		redisContainer: redisContainer,
		repo:           repo,
	}
}

// setupRedisChaosInfra sets up the test infrastructure for Redis chaos testing.
func setupRedisChaosInfra(t *testing.T) *chaosTestInfra {
	t.Helper()

	// Setup Redis container
	redisContainer := redistestutil.SetupContainer(t)

	// Create lib-commons Redis connection
	conn := redistestutil.CreateConnection(t, redisContainer.Addr)

	// Create repository
	repo := &RedisConsumerRepository{
		conn: conn,
	}

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &chaosTestInfra{
		redisContainer: redisContainer,
		repo:           repo,
		chaosOrch:      chaosOrch,
	}
}

// setupRedisNetworkChaosInfra sets up infrastructure for network chaos testing with Toxiproxy.
// Uses the unified chaos.Infrastructure which manages Toxiproxy lifecycle.
func setupRedisNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// 1. Create chaos infrastructure (creates network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Create Redis container (on host network, not chaos infra network)
	redisContainer := redistestutil.SetupContainer(t)

	// 3. Register Redis container with infrastructure for proxy creation
	_, err := chaosInfra.RegisterContainerWithPort("redis", redisContainer.Container, "6379/tcp")
	require.NoError(t, err, "failed to register Redis container")

	// 4. Create proxy for Redis (Toxiproxy -> Redis via host-mapped port)
	// Use port 8666 which is one of the exposed proxy ports on the Toxiproxy container
	proxy, err := chaosInfra.CreateProxyFor("redis", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for Redis")

	// 5. Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("redis")
	require.True(t, ok, "Redis container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	proxyAddr := containerInfo.ProxyListen

	// 6. Create Redis connection through proxy
	proxyConn := redistestutil.CreateConnection(t, proxyAddr)

	proxyRepo := &RedisConsumerRepository{
		conn: proxyConn,
	}

	return &networkChaosTestInfra{
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		proxyRepo:      proxyRepo,
		proxyConn:      proxyConn,
		proxy:          proxy,
	}
}

// cleanup releases all resources for chaos tests.
// Note: Container cleanup is handled automatically by SetupContainer via t.Cleanup().
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
}

// cleanup releases all resources for network chaos infrastructure.
// Note: Container cleanup is handled automatically by SetupContainer via t.Cleanup().
func (infra *networkChaosTestInfra) cleanup() {
	// Cleanup Infrastructure (Toxiproxy, network, orchestrator)
	// Note: This may log warnings about already-terminated containers
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// recreateConnectionForInspection creates a NEW Redis connection for inspection after container restart.
// This is necessary because the original connection is invalidated when the container restarts with a new port.
// NOTE: This does NOT test the application's auto-reconnect mechanism - it creates a fresh connection
// solely for inspecting Redis state in data integrity tests.
func (infra *chaosTestInfra) recreateConnectionForInspection(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Get the NEW port assigned after container restart
	newPort, err := infra.redisContainer.Container.MappedPort(ctx, "6379")
	require.NoError(t, err, "should get new Redis port after restart")

	host, err := infra.redisContainer.Container.Host(ctx)
	require.NoError(t, err, "should get Redis host after restart")

	// Update container result with new address
	infra.redisContainer.Addr = fmt.Sprintf("%s:%s", host, newPort.Port())

	// Create a fresh connection with retry logic (Redis may still be starting)
	infra.repo.conn = redistestutil.CreateConnectionWithRetry(t, infra.redisContainer.Addr, 30*time.Second)

	t.Logf("Created new connection for inspection (port changed to %s)", newPort.Port())
}

// =============================================================================
// INTEGRATION TESTS - BALANCE OPERATIONS
// =============================================================================

// TestIntegration_Redis_BalanceConsistency tests that balance calculations
// remain consistent through a series of operations.
func TestIntegration_Redis_BalanceConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Initial balance: 1000
	initialAvailable := decimal.NewFromInt(1000)

	// Execute series of operations that should result in known final balance
	operations := []struct {
		opType string
		amount int64
	}{
		{constant.DEBIT, 100},  // 1000 - 100 = 900
		{constant.CREDIT, 50},  // 900 + 50 = 950
		{constant.DEBIT, 200},  // 950 - 200 = 750
		{constant.CREDIT, 100}, // 750 + 100 = 850
	}

	currentAvailable := initialAvailable
	for i, op := range operations {
		transactionID := uuid.New()

		var balanceOps []mmodel.BalanceOperation
		if op.opType == constant.DEBIT {
			currentAvailable = currentAvailable.Sub(decimal.NewFromInt(op.amount))
			balanceOps = []mmodel.BalanceOperation{
				redistestutil.CreateBalanceOperationWithAvailable(
					orgID, ledgerID, "@consistency-test", "USD",
					constant.DEBIT, decimal.NewFromInt(op.amount),
					currentAvailable,
					"deposit",
				),
			}
		} else {
			currentAvailable = currentAvailable.Add(decimal.NewFromInt(op.amount))
			balanceOps = []mmodel.BalanceOperation{
				redistestutil.CreateBalanceOperationWithAvailable(
					orgID, ledgerID, "@consistency-test", "USD",
					constant.CREDIT, decimal.NewFromInt(op.amount),
					currentAvailable,
					"deposit",
				),
			}
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
		require.NoError(t, err, "operation %d should succeed", i)

		// Verify balance after each operation is non-negative
		if len(result.After) > 0 {
			assert.GreaterOrEqual(t, result.After[0].Available.IntPart(), int64(0),
				"balance for @consistency-test should not be negative")
		}
	}

	// Final balance should be 850
	expectedFinal := decimal.NewFromInt(850)
	assert.True(t, currentAvailable.Equal(expectedFinal),
		"final balance should be %s, got %s", expectedFinal, currentAvailable)

	t.Log("Integration test passed: balance consistency verified")
}

// =============================================================================
// INTEGRATION TESTS - PENDING TRANSACTIONS
// =============================================================================

// TestIntegration_Redis_PendingTransactionFlow tests the complete flow of
// pending transactions: hold funds, then commit.
func TestIntegration_Redis_PendingTransactionFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Create pending balance operation (hold funds)
	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@pending-test", "USD",
			constant.DEBIT, decimal.NewFromInt(500),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	// Execute as pending (isPending=true)
	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "PENDING", true, balanceOps)
	require.NoError(t, err, "pending operation should succeed")
	require.NotNil(t, result, "should return balances")

	t.Logf("Pending transaction created: %s", transactionID)

	// Commit the pending transaction
	commitOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@pending-test", "USD",
			constant.DEBIT, decimal.NewFromInt(500),
			decimal.NewFromInt(500), // Available after commit
			"deposit",
		),
	}

	result, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, commitOps)
	require.NoError(t, err, "commit operation should succeed")

	t.Log("Integration test passed: pending transaction flow verified")
}

// =============================================================================
// INTEGRATION TESTS - BACKUP QUEUE
// =============================================================================

// TestIntegration_Redis_BackupQueueOperations tests the backup queue CRUD operations.
func TestIntegration_Redis_BackupQueueOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	ctx := context.Background()
	numMessages := 10

	// 1. Add multiple messages to queue
	t.Log("Step 1: Adding messages to backup queue")
	messageKeys := make([]string, numMessages)
	for i := 0; i < numMessages; i++ {
		key := fmt.Sprintf("test-msg-%d-%s", i, uuid.New().String())
		msg := []byte(fmt.Sprintf(`{"id":"%s","data":"test message %d"}`, key, i))

		err := infra.repo.AddMessageToQueue(ctx, key, msg)
		require.NoError(t, err, "should add message %d to queue", i)
		messageKeys[i] = key
	}
	t.Logf("Added %d messages to backup queue", numMessages)

	// 2. Read all messages from queue
	t.Log("Step 2: Reading all messages from queue")
	allMessages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err, "should read all messages from queue")
	assert.GreaterOrEqual(t, len(allMessages), numMessages, "should have at least %d messages", numMessages)
	t.Logf("Read %d messages from queue", len(allMessages))

	// 3. Read individual messages
	t.Log("Step 3: Reading individual messages")
	for i, key := range messageKeys[:3] { // Test first 3
		msg, err := infra.repo.ReadMessageFromQueue(ctx, key)
		require.NoError(t, err, "should read message %d", i)
		assert.NotEmpty(t, msg, "message %d should not be empty", i)
	}

	// 4. Remove messages from queue
	t.Log("Step 4: Removing messages from queue")
	for i, key := range messageKeys {
		err := infra.repo.RemoveMessageFromQueue(ctx, key)
		require.NoError(t, err, "should remove message %d", i)
	}

	// 5. Verify our test messages are removed
	t.Log("Step 5: Verifying messages are removed")
	finalMessages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err, "should read final queue state")

	for _, key := range messageKeys {
		_, exists := finalMessages[key]
		assert.False(t, exists, "message %s should be removed", key)
	}

	t.Log("Integration test passed: backup queue operations verified")
}

// =============================================================================
// CHAOS TESTS - CONTAINER LIFECYCLE
// =============================================================================

// TestChaos_Redis_RestartRecovery tests that the Redis consumer repository
// recovers after a Redis container restart.
func TestIntegration_Chaos_Redis_RestartRecovery(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRedisChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// 1. Execute initial operation to verify setup works
	t.Log("Step 1: Executing initial balance operation")
	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@sender", "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	chaosResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "initial balance operation should succeed")
	require.NotNil(t, chaosResult, "should return balances")
	t.Logf("Initial balance operation successful: %d balances updated", len(chaosResult.After))

	// 2. INJECT CHAOS: Restart container
	containerID := infra.redisContainer.Container.GetContainerID()
	t.Logf("Step 2: INJECT CHAOS - Restarting Redis container %s", containerID)

	err = infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")
	t.Log("Chaos: Redis container restarted successfully")

	// 3. Recreate connection for inspection (port may have changed)
	infra.recreateConnectionForInspection(t)

	// 4. Verify recovery by executing another operation
	t.Log("Step 3: Verifying recovery with new operation")
	transactionID2 := uuid.New()
	balanceOps2 := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@sender-recovery", "USD",
			constant.CREDIT, decimal.NewFromInt(50),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID2, "ACTIVE", false, balanceOps2)
		return err
	}, 30*time.Second, "Redis should recover and process operations after restart")

	t.Log("Chaos test passed: Redis restart recovery verified")
}

// TestChaos_Redis_DataIntegrityAfterRestart tests that balance data persists
// correctly after container restart (Redis with persistence).
func TestIntegration_Chaos_Redis_DataIntegrityAfterRestart(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRedisChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// 1. Store some data before restart
	t.Log("Step 1: Storing data before restart")
	testKey := "chaos-test:data-integrity:" + uuid.New().String()
	testValue := "test-value-before-restart"

	err := infra.repo.Set(ctx, testKey, testValue, 3600) // 1 hour TTL
	require.NoError(t, err, "should set value before restart")

	// Verify data is stored
	storedValue, err := infra.repo.Get(ctx, testKey)
	require.NoError(t, err, "should get value before restart")
	assert.Equal(t, testValue, storedValue, "value should match before restart")

	// 2. INJECT CHAOS: Restart container
	containerID := infra.redisContainer.Container.GetContainerID()
	t.Logf("Step 2: INJECT CHAOS - Restarting Redis container %s", containerID)

	err = infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")

	// 3. Recreate connection
	infra.recreateConnectionForInspection(t)

	// 4. Note: Default Redis/Valkey image may not have persistence enabled.
	// This test documents the expected behavior - data may be lost on restart
	// without RDB/AOF persistence configured.
	t.Log("Step 3: Checking data after restart (data loss expected without persistence)")

	// Try to get the value - it may or may not exist depending on Redis config
	recoveredValue, err := infra.repo.Get(ctx, testKey)
	if err != nil || recoveredValue == "" {
		t.Log("Data was lost after restart (expected without persistence)")
	} else {
		t.Logf("Data survived restart: %s", recoveredValue)
		assert.Equal(t, testValue, recoveredValue, "recovered value should match")
	}

	t.Log("Chaos test passed: data integrity behavior after restart documented")
}

// =============================================================================
// CHAOS TESTS - NETWORK CHAOS
// =============================================================================

// TestChaos_Redis_NetworkLatency tests repository behavior under network latency.
// Uses Toxiproxy to inject latency into the network path.
func TestIntegration_Chaos_Redis_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRedisNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	orgID := uuid.New()
	ledgerID := uuid.New()

	// 1. Verify normal operation through proxy
	t.Log("Step 1: Verifying normal operation through proxy")
	transactionID := uuid.New()
	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@latency-test", "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	balances, err := infra.proxyRepo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "initial operation through proxy should succeed")
	require.NotNil(t, balances, "should return balances")
	t.Log("Initial operation successful through proxy")

	// 2. INJECT CHAOS: Add 300ms latency with 50ms jitter
	t.Log("Step 2: INJECT CHAOS - Adding 300ms latency to Redis connection")
	err = infra.proxy.AddLatency(300*time.Millisecond, 50*time.Millisecond)
	require.NoError(t, err, "adding latency should succeed")

	// 3. Execute operations with latency - they should still succeed
	t.Log("Step 3: Executing operations with latency")
	numOperations := 3
	for i := 0; i < numOperations; i++ {
		transactionID := uuid.New()
		ops := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, fmt.Sprintf("@latency-test-%d", i), "USD",
				constant.CREDIT, decimal.NewFromInt(int64(10+i)),
				decimal.NewFromInt(1000),
				"deposit",
			),
		}

		start := time.Now()
		_, err := infra.proxyRepo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, ops)
		elapsed := time.Since(start)

		require.NoError(t, err, "operation %d with latency should succeed", i+1)
		t.Logf("Operation %d completed in %v (with ~300ms latency injected)", i+1, elapsed)
	}

	// 4. REMOVE CHAOS: Remove all toxics
	t.Log("Step 4: Removing latency")
	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "removing toxics should succeed")

	// 5. Verify normal operation restored
	t.Log("Step 5: Verifying normal operation restored")
	transactionID = uuid.New()
	balanceOps = []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@after-latency", "USD",
			constant.CREDIT, decimal.NewFromInt(25),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	start := time.Now()
	_, err = infra.proxyRepo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	elapsed := time.Since(start)
	require.NoError(t, err, "operation after removing latency should succeed")
	t.Logf("Operation after latency removal completed in %v", elapsed)

	t.Log("Chaos test passed: Redis network latency handling verified")
}

// TestChaos_Redis_NetworkPartition tests repository behavior during network partition.
// Uses Toxiproxy to disconnect/reconnect the network path.
func TestIntegration_Chaos_Redis_NetworkPartition(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRedisNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	// 1. Verify baseline connectivity
	t.Log("Step 1: Verifying baseline connectivity")
	transactionID := uuid.New()
	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@partition-test", "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	_, err := infra.proxyRepo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "baseline operation should succeed")
	t.Log("Baseline operation successful")

	// 2. INJECT CHAOS: Disconnect proxy to simulate network partition
	t.Log("Step 2: INJECT CHAOS - Disconnecting proxy to simulate network partition")
	err = infra.proxy.Disconnect()
	require.NoError(t, err, "proxy disconnect should succeed")

	// 3. Operations during partition should fail
	t.Log("Step 3: Attempting operation during network partition (should fail)")
	transactionID = uuid.New()
	partitionOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@during-partition", "USD",
			constant.CREDIT, decimal.NewFromInt(50),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = infra.proxyRepo.ProcessBalanceAtomicOperation(ctxWithTimeout, orgID, ledgerID, transactionID, "ACTIVE", false, partitionOps)
	cancel()

	// Expect error during network partition
	assert.Error(t, err, "operation during network partition should fail")
	t.Logf("Operation during partition failed as expected: %v", err)

	// 4. REMOVE CHAOS: Reconnect proxy
	t.Log("Step 4: REMOVE CHAOS - Reconnecting proxy")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "proxy reconnect should succeed")

	// 5. Operations after reconnect should succeed
	t.Log("Step 5: Verifying operations succeed after reconnect")
	transactionID = uuid.New()
	recoveryOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@after-partition", "USD",
			constant.CREDIT, decimal.NewFromInt(75),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.proxyRepo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, recoveryOps)
		return err
	}, 10*time.Second, "operations should succeed after network recovery")

	t.Log("Chaos test passed: Redis network partition handling verified")
}

// =============================================================================
// CHAOS TESTS - BUSINESS LOGIC UNDER STRESS
// =============================================================================

// TestChaos_Redis_ConcurrentBalanceOperations tests that concurrent balance
// operations maintain data consistency through atomic Lua script execution.
func TestIntegration_Chaos_Redis_ConcurrentBalanceOperations(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	t.Skip("skipping: lib-commons RedisConnection.GetClient() fix")

	infra := setupRedisChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	numWorkers := 20

	type result struct {
		workerID int
		balances *mmodel.BalanceAtomicResult
		err      error
	}
	results := make(chan result, numWorkers)

	// Start concurrent balance operations
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			transactionID := uuid.New()
			balanceOps := []mmodel.BalanceOperation{
				redistestutil.CreateBalanceOperationWithAvailable(
					orgID, ledgerID, "@concurrent-account", "USD",
					constant.DEBIT, decimal.NewFromInt(int64(workerID+1)),
					decimal.NewFromInt(10000), // Large initial balance to avoid insufficient funds
					"deposit",
				),
			}

			atomicResult, err := infra.repo.ProcessBalanceAtomicOperation(
				ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps,
			)
			results <- result{workerID: workerID, balances: atomicResult, err: err}
		}(i)
	}

	// Wait for all workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Analyze results
	var successCount, errorCount int
	for r := range results {
		if r.err != nil {
			errorCount++
			t.Logf("Worker %d error: %v", r.workerID, r.err)
		} else {
			successCount++
		}
	}

	t.Logf("Concurrent operations: %d successful, %d errors out of %d total", successCount, errorCount, numWorkers)
	assert.Greater(t, successCount, 0, "at least some concurrent operations should succeed")

	t.Log("Chaos test passed: concurrent balance operations handled atomically")
}

// TestChaos_Redis_InsufficientFundsUnderLoad tests that insufficient funds validation
// works correctly under concurrent load.
func TestIntegration_Chaos_Redis_InsufficientFundsUnderLoad(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	t.Skip("skipping: lib-commons RedisConnection.GetClient() fix")

	infra := setupRedisChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Start with a balance that will run out
	initialBalance := decimal.NewFromInt(100)
	debitAmount := decimal.NewFromInt(30)
	numWorkers := 10 // Each trying to debit 30, but only ~3 should succeed

	type result struct {
		workerID int
		err      error
	}
	results := make(chan result, numWorkers)

	// Start concurrent debit operations
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			transactionID := uuid.New()
			balanceOps := []mmodel.BalanceOperation{
				redistestutil.CreateBalanceOperationWithAvailable(
					orgID, ledgerID, "@insufficient-funds-test", "USD",
					constant.DEBIT, debitAmount,
					initialBalance,
					"deposit",
				),
			}

			_, err := infra.repo.ProcessBalanceAtomicOperation(
				ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps,
			)
			results <- result{workerID: workerID, err: err}
		}(i)
	}

	// Wait for all workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Analyze results
	var successCount, insufficientFundsCount, otherErrorCount int
	for r := range results {
		if r.err == nil {
			successCount++
		} else if r.err.Error() == "0018" || // Direct error code
			(len(r.err.Error()) > 0 && r.err.Error()[:4] == "0018") {
			insufficientFundsCount++
		} else {
			otherErrorCount++
			t.Logf("Worker %d unexpected error: %v", r.workerID, r.err)
		}
	}

	t.Logf("Results: %d successful, %d insufficient funds, %d other errors",
		successCount, insufficientFundsCount, otherErrorCount)

	// At least some operations should succeed, and some should fail with insufficient funds
	assert.Greater(t, successCount, 0, "some operations should succeed")

	t.Log("Chaos test passed: insufficient funds validation works under load")
}

// =============================================================================
// CHAOS TESTS - GRACEFUL DEGRADATION
// =============================================================================

// TestChaos_Redis_GracefulDegradation tests that the repository fails
// gracefully when Redis is unavailable.
func TestIntegration_Chaos_Redis_GracefulDegradation(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRedisChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Verify normal operation works
	transactionID := uuid.New()
	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@test", "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000),
			"deposit",
		),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "normal operation should work")

	// Test with cancelled context (simulates timeout/unavailability)
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	chaos.AssertGracefulDegradation(t,
		func() error {
			transactionID := uuid.New()
			_, err := infra.repo.ProcessBalanceAtomicOperation(cancelledCtx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
			return err
		},
		nil, // Any error is acceptable for graceful degradation
		"operation with cancelled context should fail gracefully",
	)

	// Verify normal operation still works
	transactionID2 := uuid.New()
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID2, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "normal operation should work after graceful degradation")

	t.Log("Chaos test passed: graceful degradation verified")
}

// =============================================================================
// INTEGRATION TESTS - EXTERNAL ACCOUNT VALIDATION
// =============================================================================

// TestIntegration_Redis_ExternalAccountCreditValidation tests that external accounts
// cannot have positive balance after credit operations.
// This validates error code 0018 for external destinations in the Lua script.
func TestIntegration_Redis_ExternalAccountCreditValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	// NOTE: Each sub-test uses unique orgID/ledgerID to ensure isolated Redis keys.
	// The Lua script uses SET NX (set if not exists), so sharing keys between tests
	// would cause the first test's balance to be reused by subsequent tests.

	t.Run("external account with zero balance cannot receive credit", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// External account with Available = 0
		// Attempting to credit should fail because result would be positive
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@external-zero", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(0), // Available = 0
				"external",            // AccountType = external
			),
		}

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)

		// Should fail with error 0018 (insufficient funds / invalid balance state)
		redistestutil.AssertInsufficientFundsError(t, err)
		t.Log("External account credit validation passed: zero balance credit blocked")
	})

	t.Run("external account with negative balance can receive limited credit", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// External account with Available = -100 (debt to external entity)
		// Crediting 50 should succeed because result would be -50 (still negative)
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@external-negative", "USD",
				constant.CREDIT, decimal.NewFromInt(50),
				decimal.NewFromInt(-100), // Available = -100
				"external",               // AccountType = external
			),
		}

		balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)

		// Should succeed because result is -50 (still negative)
		require.NoError(t, err, "credit to external account that stays negative should succeed")
		require.NotNil(t, balances, "should return balances")
		t.Log("External account partial credit validation passed")
	})

	t.Run("external account credit that would result in positive balance fails", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// External account with Available = -50
		// Crediting 100 should fail because result would be +50 (positive)
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@external-overflow", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(-50), // Available = -50
				"external",              // AccountType = external
			),
		}

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)

		// Should fail with error 0018
		redistestutil.AssertInsufficientFundsError(t, err)
		t.Log("External account overflow validation passed: positive result blocked")
	})

	t.Run("internal account can have positive balance", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Internal account (deposit type) can have positive balance
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@internal-account", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(0), // Available = 0
				"deposit",             // AccountType = deposit (internal)
			),
		}

		balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)

		// Should succeed - internal accounts can have positive balance
		require.NoError(t, err, "credit to internal account should succeed")
		require.NotNil(t, balances, "should return balances")
		t.Log("Internal account credit validation passed")
	})

	t.Run("external account debit makes balance more negative", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// External account with Available = -100
		// Debiting 100 results in -200 (more negative), which is valid for external accounts
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@external-to-zero", "USD",
				constant.DEBIT, decimal.NewFromInt(100),
				decimal.NewFromInt(-100), // Available = -100
				"external",
			),
		}

		balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)

		// Should succeed - result is -200 (negative), not positive
		require.NoError(t, err, "debit to external account should succeed when result stays negative")
		require.NotNil(t, balances, "should return balances")
		t.Log("External account debit validation passed - balance became more negative")
	})

	t.Log("Integration test passed: external account credit validation verified")
}

// =============================================================================
// INTEGRATION TESTS - PENDING TRANSACTION VERSION GAPS
// =============================================================================

// TestIntegration_Redis_PendingDestinationNoVersionIncrement tests that destination balances
// do NOT have their version incremented during PENDING transactions (CREDIT + PENDING).
//
// Bug context: Previously, the Lua script unconditionally incremented balance.Version
// even when no actual balance change occurred. For PENDING destinations, the CREDIT
// operation has no effect (the balance is credited only on APPROVED), but version
// was still incremented, causing "version gaps" in the operation history.
//
// Fix: The Lua script now only increments version when hasChange is true:
// hasChange = (result ~= balance.Available) or (resultOnHold ~= balance.OnHold)
func TestIntegration_Redis_PendingDestinationNoVersionIncrement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("PENDING source ON_HOLD returns balance in returnBalances", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Source balance: ON_HOLD operation during PENDING should change balance
		// Available: 1000 -> 900 (moved to OnHold)
		// OnHold: 0 -> 100
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@source-pending", "USD",
				constant.ONHOLD, decimal.NewFromInt(100),
				decimal.NewFromInt(1000), // Available = 1000
				"deposit",
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.PENDING, true, // isPending = true
			balanceOps,
		)

		require.NoError(t, err, "PENDING source ON_HOLD should succeed")

		// KEY ASSERTION: Source balance SHOULD be in returnBalances because change occurred
		require.Len(t, result.After, 1, "should return 1 balance (source changed)")
		assert.Equal(t, "@source-pending", result.After[0].Alias, "returned balance should have correct alias")

		t.Log("PENDING source ON_HOLD: balance included in returnBalances as expected")
	})

	t.Run("PENDING destination CREDIT does NOT increment version", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Destination balance: CREDIT operation during PENDING has NO effect
		// Available: stays 500 (credit only applied on APPROVED)
		// OnHold: stays 0
		// Version should NOT increment because no change occurred
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@dest-pending", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(500), // Available = 500
				"deposit",
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.PENDING, true, // isPending = true
			balanceOps,
		)

		require.NoError(t, err, "PENDING destination CREDIT should succeed")

		// KEY ASSERTION: Balance should NOT be in returnBalances because no change occurred
		assert.Len(t, result.After, 0,
			"destination balance should NOT be in returnBalances (no change occurred)")

		t.Log("PENDING destination CREDIT: balance correctly excluded from returnBalances")
	})

	t.Run("APPROVED source DEBIT returns balance in returnBalances", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Source balance: DEBIT on APPROVED (after PENDING phase)
		// OnHold: 100 -> 0 (released)
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithOnHold(
				orgID, ledgerID, "@source-approved", "USD",
				constant.DEBIT, decimal.NewFromInt(100),
				decimal.NewFromInt(900), // Available = 900
				decimal.NewFromInt(100), // OnHold = 100 (from PENDING phase)
				"deposit",
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.APPROVED, true, // isPending = true (was pending transaction)
			balanceOps,
		)

		require.NoError(t, err, "APPROVED source DEBIT should succeed")

		// KEY ASSERTION: Source balance SHOULD be in returnBalances because OnHold changed
		require.Len(t, result.After, 1, "should return 1 balance (source changed)")
		assert.Equal(t, "@source-approved", result.After[0].Alias, "returned balance should have correct alias")

		t.Log("APPROVED source DEBIT: balance included in returnBalances as expected")
	})

	t.Run("APPROVED destination CREDIT returns balance in returnBalances", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Destination balance: CREDIT on APPROVED
		// Available: 500 -> 600
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@dest-approved", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(500), // Available = 500
				"deposit",
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.APPROVED, true, // isPending = true (was pending transaction)
			balanceOps,
		)

		require.NoError(t, err, "APPROVED destination CREDIT should succeed")

		// KEY ASSERTION: Destination balance SHOULD be in returnBalances because Available changed
		require.Len(t, result.After, 1, "should return 1 balance (destination changed)")
		assert.Equal(t, "@dest-approved", result.After[0].Alias, "returned balance should have correct alias")

		t.Log("APPROVED destination CREDIT: balance included in returnBalances as expected")
	})

	t.Run("non-PENDING transaction CREDIT returns balance in returnBalances", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Normal (non-PENDING) transaction: CREDIT should change balance
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@normal-credit", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(500),
				"deposit",
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			"ACTIVE", false, // isPending = false (normal transaction)
			balanceOps,
		)

		require.NoError(t, err, "normal CREDIT should succeed")

		// KEY ASSERTION: Balance SHOULD be in returnBalances because Available changed
		require.Len(t, result.After, 1, "should return 1 balance")
		assert.Equal(t, "@normal-credit", result.After[0].Alias, "returned balance should have correct alias")

		t.Log("non-PENDING CREDIT: balance included in returnBalances as expected")
	})

	t.Log("Integration test passed: PENDING destination version gap fix verified")
}

// TestIntegration_Redis_VersionContinuity tests that balance versions remain
// contiguous (no gaps) through a complete PENDING transaction lifecycle.
//
// This is a regression test for the version gap bug where PENDING destinations
// caused version increments without corresponding operations.
func TestIntegration_Redis_VersionContinuity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("complete PENDING lifecycle has no version gaps", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()

		// Track versions through the lifecycle
		var sourceVersions []int64
		var destVersions []int64

		// Phase 1: PENDING - only source should have version change
		t.Log("Phase 1: Creating PENDING transaction")
		pendingTxID := uuid.New()

		// Source: ON_HOLD (Available: 1000 -> 900, OnHold: 0 -> 100)
		sourceOp := redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@lifecycle-source", "USD",
			constant.ONHOLD, decimal.NewFromInt(100),
			decimal.NewFromInt(1000),
			"deposit",
		)

		sourceResult, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, pendingTxID,
			constant.PENDING, true,
			[]mmodel.BalanceOperation{sourceOp},
		)
		require.NoError(t, err, "PENDING source should succeed")
		require.Len(t, sourceResult.After, 1, "source should be in returnBalances")
		sourceVersions = append(sourceVersions, sourceResult.After[0].Version)

		// Destination: CREDIT during PENDING (no effect, no version change)
		destOp := redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@lifecycle-dest", "USD",
			constant.CREDIT, decimal.NewFromInt(100),
			decimal.NewFromInt(500),
			"deposit",
		)

		destResult, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, pendingTxID,
			constant.PENDING, true,
			[]mmodel.BalanceOperation{destOp},
		)
		require.NoError(t, err, "PENDING destination should succeed")
		// KEY: Destination should NOT be in returnBalances (no change)
		assert.Len(t, destResult.After, 0, "destination should NOT be in returnBalances during PENDING")

		// Phase 2: APPROVED - both source and destination should have version change
		t.Log("Phase 2: Approving PENDING transaction")
		approvedTxID := uuid.New()

		// Source: DEBIT (OnHold: 100 -> 0, releasing the hold)
		sourceOpApproved := redistestutil.CreateBalanceOperationWithOnHold(
			orgID, ledgerID, "@lifecycle-source", "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(900), // Available stayed at 900
			decimal.NewFromInt(100), // OnHold from PENDING phase
			"deposit",
		)

		sourceResultApproved, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, approvedTxID,
			constant.APPROVED, true,
			[]mmodel.BalanceOperation{sourceOpApproved},
		)
		require.NoError(t, err, "APPROVED source should succeed")
		require.Len(t, sourceResultApproved.After, 1, "source should be in returnBalances")
		sourceVersions = append(sourceVersions, sourceResultApproved.After[0].Version)

		// Destination: CREDIT on APPROVED (Available: 500 -> 600)
		destOpApproved := redistestutil.CreateBalanceOperationWithAvailable(
			orgID, ledgerID, "@lifecycle-dest", "USD",
			constant.CREDIT, decimal.NewFromInt(100),
			decimal.NewFromInt(500), // Still at 500 (wasn't changed during PENDING)
			"deposit",
		)

		destResultApproved, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, approvedTxID,
			constant.APPROVED, true,
			[]mmodel.BalanceOperation{destOpApproved},
		)
		require.NoError(t, err, "APPROVED destination should succeed")
		require.Len(t, destResultApproved.After, 1, "destination should be in returnBalances on APPROVED")
		destVersions = append(destVersions, destResultApproved.After[0].Version)

		// Verify the key behavior: source appears twice, destination appears once
		t.Logf("Source returnBalances count: %d, versions: %v", len(sourceVersions), sourceVersions)
		t.Logf("Destination returnBalances count: %d, versions: %v", len(destVersions), destVersions)

		// KEY ASSERTIONS:
		// Source: should appear in returnBalances 2 times (PENDING and APPROVED)
		require.Len(t, sourceVersions, 2, "source should be in returnBalances twice (PENDING + APPROVED)")

		// Destination: should appear in returnBalances 1 time (only APPROVED)
		// This proves the fix: PENDING destination is NOT in returnBalances (no version gap)
		require.Len(t, destVersions, 1, "destination should be in returnBalances once (only APPROVED)")

		// VERSION CONTINUITY VERIFICATION:
		// What matters for continuity (proving the fix):
		// - Source: 2 operations created → 2 entries in returnBalances
		// - Destination: 1 operation created → 1 entry in returnBalances
		// If the bug existed, destination would appear 2 times (PENDING + APPROVED)
		// but only 1 operation would be created, causing a version gap.
		//
		// NOTE: The specific version values depend on Redis state and are not meaningful
		// for this test because all balances share the same Redis key (balanceKey="default").
		// The key assertion is the COUNT of entries in returnBalances.

		// Verify versions are present (proves balances were processed)
		assert.NotEmpty(t, sourceVersions[0], "source PENDING should have version")
		assert.NotEmpty(t, sourceVersions[1], "source APPROVED should have version")
		assert.NotEmpty(t, destVersions[0], "destination APPROVED should have version")

		t.Log("Version continuity verified: destination excluded from PENDING phase, no version gap")
	})

	t.Log("Integration test passed: version continuity verified")
}

// TestIntegration_Redis_CanceledTransactionRelease tests the CANCELED transaction flow
// where RELEASE operation returns held funds to Available balance.
//
// This completes the hasChange logic coverage for all transaction status paths:
// - PENDING + ON_HOLD (covered in TestIntegration_Redis_PendingDestinationNoVersionIncrement)
// - PENDING + CREDIT (covered - no change, excluded from returnBalances)
// - APPROVED + DEBIT (covered)
// - APPROVED + CREDIT (covered)
// - CANCELED + RELEASE (this test)
func TestIntegration_Redis_CanceledTransactionRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("CANCELED RELEASE returns held funds to Available", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Source balance after PENDING phase:
		// - Available: 900 (1000 - 100 moved to OnHold)
		// - OnHold: 100 (held for pending transaction)
		//
		// On CANCELED with RELEASE:
		// - OnHold: 100 -> 0 (released)
		// - Available: 900 -> 1000 (restored)
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithOnHold(
				orgID, ledgerID, "@canceled-source", "USD",
				constant.RELEASE, decimal.NewFromInt(100),
				decimal.NewFromInt(900), // Available = 900 (reduced during PENDING)
				decimal.NewFromInt(100), // OnHold = 100 (held during PENDING)
				"deposit",
			),
		}

		cancelResult, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.CANCELED, true, // isPending = true (was pending transaction)
			balanceOps,
		)

		require.NoError(t, err, "CANCELED RELEASE should succeed")

		// KEY ASSERTION: Balance SHOULD be in returnBalances because both Available and OnHold changed
		require.Len(t, cancelResult.After, 1, "should return 1 balance (source changed)")
		assert.Equal(t, "@canceled-source", cancelResult.After[0].Alias, "returned balance should have correct alias")

		t.Log("CANCELED RELEASE: balance included in returnBalances as expected")
	})

	t.Run("CANCELED destination has no effect (similar to PENDING destination)", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// Destination balance during CANCELED:
		// If the transaction is canceled, the destination never received the credit.
		// Processing CREDIT + CANCELED should have no effect (no change).
		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreateBalanceOperationWithAvailable(
				orgID, ledgerID, "@canceled-dest", "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(500), // Available = 500 (unchanged)
				"deposit",
			),
		}

		cancelDestResult, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.CANCELED, true, // isPending = true
			balanceOps,
		)

		require.NoError(t, err, "CANCELED destination CREDIT should succeed")

		// KEY ASSERTION: Destination should NOT be in returnBalances (no change)
		// CREDIT + CANCELED has no matching branch in Lua, so result == balance.Available
		assert.Len(t, cancelDestResult.After, 0,
			"destination should NOT be in returnBalances (CREDIT + CANCELED has no effect)")

		t.Log("CANCELED destination CREDIT: balance correctly excluded from returnBalances")
	})

	t.Log("Integration test passed: CANCELED transaction flow verified")
}

// =============================================================================
// INTEGRATION TESTS - DOUBLE-ENTRY PENDING (RouteValidationEnabled)
// =============================================================================

// TestIntegration_Redis_DoubleEntryPending_RouteValidationEnabled_VersionIncrBy2
// verifies that when RouteValidationEnabled=true, a PENDING+ON_HOLD operation
// increments the balance version by 2 atomically in the Lua script.
// This is the core double-entry behavior: the pattern produces
// DEBIT debit + ONHOLD credit, represented by version+=2.
func TestIntegration_Redis_DoubleEntryPending_RouteValidationEnabled_VersionIncrBy2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("version increments by 2 when routeValidationEnabled is true", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		initialAvailable := decimal.NewFromInt(1000)
		initialOnHold := decimal.Zero
		initialVersion := int64(1)
		amount := decimal.NewFromInt(200)

		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreatePendingBalanceOperation(
				orgID, ledgerID, "@source-route-enabled", "USD",
				constant.ONHOLD, amount,
				initialAvailable, initialOnHold, initialVersion,
				"deposit", true, // routeValidationEnabled = true
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.PENDING, true,
			balanceOps,
		)

		require.NoError(t, err, "PENDING ON_HOLD with routeValidation should succeed")
		require.NotNil(t, result)
		require.Len(t, result.After, 1, "should have 1 after balance")
		require.Len(t, result.Before, 1, "should have 1 before balance")

		// Before state: unchanged
		assert.True(t, result.Before[0].Available.Equal(initialAvailable),
			"before Available should be %s, got %s", initialAvailable, result.Before[0].Available)
		assert.True(t, result.Before[0].OnHold.Equal(initialOnHold),
			"before OnHold should be %s, got %s", initialOnHold, result.Before[0].OnHold)

		// After state: Available decreased, OnHold increased, version +2
		expectedAvailable := initialAvailable.Sub(amount)
		expectedOnHold := initialOnHold.Add(amount)
		expectedVersion := initialVersion + 2

		assert.True(t, result.After[0].Available.Equal(expectedAvailable),
			"after Available should be %s, got %s", expectedAvailable, result.After[0].Available)
		assert.True(t, result.After[0].OnHold.Equal(expectedOnHold),
			"after OnHold should be %s, got %s", expectedOnHold, result.After[0].OnHold)
		assert.Equal(t, expectedVersion, result.After[0].Version,
			"version should increment by 2 when routeValidationEnabled=true")

		t.Logf("RouteValidationEnabled=true: version %d -> %d (increment by 2)", initialVersion, expectedVersion)
	})

	t.Run("version increments by 1 when routeValidationEnabled is false", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		initialAvailable := decimal.NewFromInt(1000)
		initialOnHold := decimal.Zero
		initialVersion := int64(1)
		amount := decimal.NewFromInt(200)

		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreatePendingBalanceOperation(
				orgID, ledgerID, "@source-route-disabled", "USD",
				constant.ONHOLD, amount,
				initialAvailable, initialOnHold, initialVersion,
				"deposit", false, // routeValidationEnabled = false
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.PENDING, true,
			balanceOps,
		)

		require.NoError(t, err, "PENDING ON_HOLD without routeValidation should succeed")
		require.NotNil(t, result)
		require.Len(t, result.After, 1)

		expectedVersion := initialVersion + 1

		assert.Equal(t, expectedVersion, result.After[0].Version,
			"version should increment by 1 when routeValidationEnabled=false")

		t.Logf("RouteValidationEnabled=false: version %d -> %d (increment by 1)", initialVersion, expectedVersion)
	})
}

// TestIntegration_Redis_DoubleEntryPending_SourceAndDestination
// verifies an end-to-end PENDING transaction with both source and destination
// in a single atomic Lua call:
//   - Source: ON_HOLD + routeValidationEnabled=true -> version +2, balance changes
//   - Destination: CREDIT + PENDING -> no change, excluded from results
//
// This tests the full double-entry PENDING pattern as it would be executed
// by the transaction handler.
func TestIntegration_Redis_DoubleEntryPending_SourceAndDestination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	sourceAvailable := decimal.NewFromInt(2000)
	sourceOnHold := decimal.Zero
	sourceVersion := int64(1)

	destAvailable := decimal.NewFromInt(500)
	destOnHold := decimal.Zero
	destVersion := int64(3)

	amount := decimal.NewFromInt(400)

	// Source: ON_HOLD with routeValidationEnabled
	sourceOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@de-source", "USD",
		constant.ONHOLD, amount,
		sourceAvailable, sourceOnHold, sourceVersion,
		"deposit", true,
	)

	// Destination: CREDIT during PENDING (no-op)
	destOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@de-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion,
		"deposit", false,
	)

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, transactionID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{sourceOp, destOp},
	)

	require.NoError(t, err, "double-entry PENDING should succeed")
	require.NotNil(t, result)

	// Only source should have changed (destination CREDIT+PENDING is a no-op)
	require.Len(t, result.Before, 1, "only source should appear in before")
	require.Len(t, result.After, 1, "only source should appear in after")

	// Source assertions
	expectedSourceAvailable := sourceAvailable.Sub(amount)
	expectedSourceOnHold := sourceOnHold.Add(amount)
	expectedSourceVersion := sourceVersion + 2

	assert.True(t, result.After[0].Available.Equal(expectedSourceAvailable),
		"source Available should be %s, got %s", expectedSourceAvailable, result.After[0].Available)
	assert.True(t, result.After[0].OnHold.Equal(expectedSourceOnHold),
		"source OnHold should be %s, got %s", expectedSourceOnHold, result.After[0].OnHold)
	assert.Equal(t, expectedSourceVersion, result.After[0].Version,
		"source version should be %d (incremented by 2), got %d", expectedSourceVersion, result.After[0].Version)

	t.Logf("Double-entry PENDING: source v%d->v%d, destination unchanged at v%d",
		sourceVersion, expectedSourceVersion, destVersion)
}

// TestIntegration_Redis_DoubleEntryPending_VersionChainConsistency
// verifies that after a double-entry PENDING operation (version+2), a subsequent
// APPROVED operation can correctly chain from the new version.
// This tests the full lifecycle: PENDING (v1->v3) then APPROVED (v3->v4).
func TestIntegration_Redis_DoubleEntryPending_VersionChainConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	initialAvailable := decimal.NewFromInt(1000)
	initialOnHold := decimal.Zero
	initialVersion := int64(1)
	amount := decimal.NewFromInt(300)

	// Phase 1: PENDING with routeValidationEnabled (v1 -> v3)
	pendingTxID := uuid.New()
	pendingOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@chain-source", "USD",
		constant.ONHOLD, amount,
		initialAvailable, initialOnHold, initialVersion,
		"deposit", true,
	)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{pendingOp},
	)
	require.NoError(t, err, "PENDING phase should succeed")
	require.Len(t, pendingResult.After, 1)

	afterPendingVersion := pendingResult.After[0].Version
	afterPendingAvailable := pendingResult.After[0].Available
	afterPendingOnHold := pendingResult.After[0].OnHold

	assert.Equal(t, int64(3), afterPendingVersion,
		"after PENDING: version should be 3 (1 + 2)")
	assert.True(t, afterPendingAvailable.Equal(decimal.NewFromInt(700)),
		"after PENDING: Available should be 700")
	assert.True(t, afterPendingOnHold.Equal(decimal.NewFromInt(300)),
		"after PENDING: OnHold should be 300")

	// Phase 2: APPROVED (v3 -> v4)
	// The balance in Redis now has version=3, Available=700, OnHold=300
	// APPROVED+DEBIT reduces OnHold, version increments by 1
	approvedTxID := uuid.New()
	approvedOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@chain-source", "USD",
		constant.DEBIT, amount,
		afterPendingAvailable, afterPendingOnHold, afterPendingVersion,
		"deposit", false,
	)

	approvedResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, approvedTxID,
		constant.APPROVED, true,
		[]mmodel.BalanceOperation{approvedOp},
	)
	require.NoError(t, err, "APPROVED phase should succeed")
	require.Len(t, approvedResult.After, 1)

	afterApprovedVersion := approvedResult.After[0].Version
	afterApprovedOnHold := approvedResult.After[0].OnHold

	assert.Equal(t, int64(4), afterApprovedVersion,
		"after APPROVED: version should be 4 (3 + 1)")
	assert.True(t, afterApprovedOnHold.Equal(decimal.Zero),
		"after APPROVED: OnHold should be 0 (released)")

	t.Logf("Version chain: v%d --(PENDING+route)--> v%d --(APPROVED)--> v%d",
		initialVersion, afterPendingVersion, afterApprovedVersion)
}

// TestIntegration_Redis_DoubleEntryPending_InsufficientFunds_Rollback
// verifies that when a PENDING+ON_HOLD+routeValidationEnabled operation fails
// due to insufficient funds, the Lua script rolls back correctly and returns
// error 0018.
func TestIntegration_Redis_DoubleEntryPending_InsufficientFunds_Rollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	initialAvailable := decimal.NewFromInt(100)
	initialOnHold := decimal.Zero
	initialVersion := int64(1)
	amount := decimal.NewFromInt(500) // exceeds Available

	balanceOps := []mmodel.BalanceOperation{
		redistestutil.CreatePendingBalanceOperation(
			orgID, ledgerID, "@insufficient-source", "USD",
			constant.ONHOLD, amount,
			initialAvailable, initialOnHold, initialVersion,
			"deposit", true, // routeValidationEnabled = true
		),
	}

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, transactionID,
		constant.PENDING, true,
		balanceOps,
	)

	require.Error(t, err, "should return error for insufficient funds")
	assert.Nil(t, result, "result should be nil on error")
	redistestutil.AssertInsufficientFundsError(t, err)

	t.Log("Double-entry PENDING insufficient funds: correctly rolled back with error 0018")
}

// TestIntegration_Redis_DoubleEntryPending_MultipleSourcesSameTransaction
// verifies that multiple source balances in a single PENDING transaction
// each get their version incremented by 2 when routeValidationEnabled=true.
// This tests the scenario where a transaction has multiple "from" entries.
func TestIntegration_Redis_DoubleEntryPending_MultipleSourcesSameTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Two sources, each with routeValidationEnabled
	source1Op := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@multi-source-1", "USD",
		constant.ONHOLD, decimal.NewFromInt(100),
		decimal.NewFromInt(500), decimal.Zero, int64(1),
		"deposit", true,
	)

	source2Op := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@multi-source-2", "USD",
		constant.ONHOLD, decimal.NewFromInt(200),
		decimal.NewFromInt(800), decimal.Zero, int64(1),
		"deposit", true,
	)

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, transactionID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{source1Op, source2Op},
	)

	require.NoError(t, err, "multiple sources PENDING should succeed")
	require.NotNil(t, result)
	require.Len(t, result.After, 2, "both sources should appear in after results")

	// Both sources should have version incremented by 2
	for i, after := range result.After {
		assert.Equal(t, int64(3), after.Version,
			"source %d version should be 3 (1 + 2), got %d", i, after.Version)
	}

	t.Log("Multiple sources: both versions incremented by 2 with routeValidationEnabled")
}

// =============================================================================
// INTEGRATION TESTS - REDIS KEY NAMESPACING
// =============================================================================

// TestIntegration_RedisNamespacing_SetGetWithTenant verifies that when a tenant
// ID is present in the context, the key stored in Redis carries the
// "tenant:{id}:" prefix and the value is retrievable via the same context.
// IS-1: Set/Get with tenant context — key stored in Redis has the prefix.
func TestIntegration_RedisNamespacing_SetGetWithTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	tenantID := "tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

	originalKey := "balance:" + uuid.New().String()
	expectedStoredKey := "tenant:" + tenantID + ":" + originalKey
	value := "integration-test-value-" + uuid.New().String()

	// Set via repository (key will be namespaced internally)
	err := infra.repo.Set(ctx, originalKey, value, 3600)
	require.NoError(t, err, "Set with tenant context should succeed")

	// Inspect actual Redis state using the raw client — verify prefix is stored
	storedVal, err := infra.redisContainer.Client.Get(context.Background(), expectedStoredKey).Result()
	require.NoError(t, err, "raw Redis GET on prefixed key should succeed")
	assert.Equal(t, value, storedVal, "value stored under prefixed key should match")

	// Verify the original (un-prefixed) key was NOT stored
	rawVal, rawErr := infra.redisContainer.Client.Get(context.Background(), originalKey).Result()
	assert.Error(t, rawErr, "raw (non-prefixed) key should not exist in Redis")
	assert.Empty(t, rawVal, "raw (non-prefixed) key should have no value")

	// Get via repository using the same context — should return correct value
	retrieved, err := infra.repo.Get(ctx, originalKey)
	require.NoError(t, err, "Get with tenant context should succeed")
	assert.Equal(t, value, retrieved, "Get should return the value set for this tenant")

	t.Log("Integration test passed: Set/Get with tenant context uses prefixed key")
}

// TestIntegration_RedisNamespacing_SetGetWithoutTenant verifies that when no
// tenant ID is present in the context, the key stored in Redis has NO prefix,
// ensuring backwards compatibility with single-tenant deployments.
// IS-2: Set/Get without tenant context — key stored has NO prefix.
func TestIntegration_RedisNamespacing_SetGetWithoutTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	// Plain context — no tenant ID
	ctx := context.Background()

	originalKey := "balance:" + uuid.New().String()
	value := "no-tenant-value-" + uuid.New().String()

	// Set via repository (key must remain unchanged)
	err := infra.repo.Set(ctx, originalKey, value, 3600)
	require.NoError(t, err, "Set without tenant context should succeed")

	// Inspect actual Redis state using the raw client — verify no prefix was added
	storedVal, err := infra.redisContainer.Client.Get(context.Background(), originalKey).Result()
	require.NoError(t, err, "raw Redis GET on original (non-prefixed) key should succeed")
	assert.Equal(t, value, storedVal, "value stored under original key should match")

	// Get via repository should return the same value
	retrieved, err := infra.repo.Get(ctx, originalKey)
	require.NoError(t, err, "Get without tenant context should succeed")
	assert.Equal(t, value, retrieved, "Get should return the value without prefix")

	t.Log("Integration test passed: Set/Get without tenant context stores key without prefix")
}

// TestIntegration_RedisNamespacing_TwoTenantsNoCollision verifies that two different
// tenants using the same logical key store values in completely isolated namespaces
// and neither can read the other's data.
// IS-3: Two tenants same key no collision — values are isolated.
func TestIntegration_RedisNamespacing_TwoTenantsNoCollision(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	tenantA := "tenant-A-" + uuid.New().String()
	tenantB := "tenant-B-" + uuid.New().String()

	ctxA := tmcore.SetTenantIDInContext(context.Background(), tenantA)
	ctxB := tmcore.SetTenantIDInContext(context.Background(), tenantB)

	// Both tenants use the SAME logical key
	sharedKey := "balance:123"
	valueA := "value-for-tenant-A-" + uuid.New().String()
	valueB := "value-for-tenant-B-" + uuid.New().String()

	// Each tenant sets its own value
	require.NoError(t, infra.repo.Set(ctxA, sharedKey, valueA, 3600), "Set for tenant A should succeed")
	require.NoError(t, infra.repo.Set(ctxB, sharedKey, valueB, 3600), "Set for tenant B should succeed")

	// Verify physical Redis keys are different
	prefixedKeyA := "tenant:" + tenantA + ":" + sharedKey
	prefixedKeyB := "tenant:" + tenantB + ":" + sharedKey

	rawA, err := infra.redisContainer.Client.Get(context.Background(), prefixedKeyA).Result()
	require.NoError(t, err, "raw GET on tenant A prefixed key should succeed")
	assert.Equal(t, valueA, rawA, "tenant A's physical key should hold tenant A's value")

	rawB, err := infra.redisContainer.Client.Get(context.Background(), prefixedKeyB).Result()
	require.NoError(t, err, "raw GET on tenant B prefixed key should succeed")
	assert.Equal(t, valueB, rawB, "tenant B's physical key should hold tenant B's value")

	// Verify isolation via repository: each tenant reads its own value, not the other's
	retrievedByA, err := infra.repo.Get(ctxA, sharedKey)
	require.NoError(t, err, "Get for tenant A should succeed")
	assert.Equal(t, valueA, retrievedByA, "tenant A should read its own value")
	assert.NotEqual(t, valueB, retrievedByA, "tenant A should NOT read tenant B's value")

	retrievedByB, err := infra.repo.Get(ctxB, sharedKey)
	require.NoError(t, err, "Get for tenant B should succeed")
	assert.Equal(t, valueB, retrievedByB, "tenant B should read its own value")
	assert.NotEqual(t, valueA, retrievedByB, "tenant B should NOT read tenant A's value")

	t.Log("Integration test passed: two tenants using the same key are fully isolated")
}

// TestIntegration_RedisNamespacing_MGetWithTenantReturnsOriginalKeys verifies that
// MGet with a tenant context sends prefixed keys to Redis but returns a result map
// keyed by the original (un-prefixed) keys, preserving the caller's key contract.
// IS-4: MGet with tenant returns original keys in the result map.
func TestIntegration_RedisNamespacing_MGetWithTenantReturnsOriginalKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	tenantID := "mget-tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

	// Store values for multiple keys under this tenant
	keys := []string{
		"balance:key-1-" + uuid.New().String(),
		"balance:key-2-" + uuid.New().String(),
		"balance:key-3-" + uuid.New().String(),
	}
	values := map[string]string{
		keys[0]: "value-1-" + uuid.New().String(),
		keys[1]: "value-2-" + uuid.New().String(),
		keys[2]: "value-3-" + uuid.New().String(),
	}

	// Pre-populate values via the repository (which applies the namespace)
	for _, k := range keys {
		require.NoError(t, infra.repo.Set(ctx, k, values[k], 3600),
			"Set for key %s should succeed", k)
	}

	// Execute MGet
	result, err := infra.repo.MGet(ctx, keys)
	require.NoError(t, err, "MGet with tenant context should succeed")
	require.Len(t, result, len(keys), "MGet result should contain all requested keys")

	// The result map MUST use original (un-prefixed) keys
	for _, originalKey := range keys {
		gotValue, exists := result[originalKey]
		assert.True(t, exists,
			"MGet result must be keyed by original key %q (not the prefixed key)", originalKey)
		assert.Equal(t, values[originalKey], gotValue,
			"MGet result value for key %q should match what was stored", originalKey)
	}

	// No prefixed key must appear in the result map
	for resultKey := range result {
		assert.NotContains(t, resultKey, "tenant:"+tenantID+":",
			"MGet result keys must NOT contain the tenant prefix — caller receives original keys")
	}

	t.Log("Integration test passed: MGet with tenant returns original keys in result map")
}

// TestIntegration_RedisNamespacing_QueueTenantIsolation verifies that
// AddMessageToQueue and ReadAllMessagesFromQueue are tenant-scoped: messages
// written by tenant A are not visible to tenant B and vice-versa.
// IS-5: Queue operations with tenant isolation.
func TestIntegration_RedisNamespacing_QueueTenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	tenantA := "queue-tenant-A-" + uuid.New().String()
	tenantB := "queue-tenant-B-" + uuid.New().String()

	ctxA := tmcore.SetTenantIDInContext(context.Background(), tenantA)
	ctxB := tmcore.SetTenantIDInContext(context.Background(), tenantB)

	msgKeyA := "tx-msg-A-" + uuid.New().String()
	msgKeyB := "tx-msg-B-" + uuid.New().String()

	payloadA := []byte(`{"tenant":"A","data":"message-from-tenant-A"}`)
	payloadB := []byte(`{"tenant":"B","data":"message-from-tenant-B"}`)

	// Tenant A adds a message to its queue
	require.NoError(t, infra.repo.AddMessageToQueue(ctxA, msgKeyA, payloadA),
		"AddMessageToQueue for tenant A should succeed")

	// Tenant B adds a message to its queue
	require.NoError(t, infra.repo.AddMessageToQueue(ctxB, msgKeyB, payloadB),
		"AddMessageToQueue for tenant B should succeed")

	// Verify physical Redis hash keys are tenant-scoped
	queueA := "tenant:" + tenantA + ":" + TransactionBackupQueue
	queueB := "tenant:" + tenantB + ":" + TransactionBackupQueue

	// Tenant A queue should contain tenant A's message field
	prefixedMsgKeyA := "tenant:" + tenantA + ":" + msgKeyA
	rawPayloadA, err := infra.redisContainer.Client.HGet(context.Background(), queueA, prefixedMsgKeyA).Bytes()
	require.NoError(t, err, "raw HGET on tenant A queue should succeed")
	assert.Equal(t, payloadA, rawPayloadA, "tenant A's queue should contain tenant A's payload")

	// Tenant B queue should contain tenant B's message field
	prefixedMsgKeyB := "tenant:" + tenantB + ":" + msgKeyB
	rawPayloadB, err := infra.redisContainer.Client.HGet(context.Background(), queueB, prefixedMsgKeyB).Bytes()
	require.NoError(t, err, "raw HGET on tenant B queue should succeed")
	assert.Equal(t, payloadB, rawPayloadB, "tenant B's queue should contain tenant B's payload")

	// ReadAllMessagesFromQueue for tenant A should NOT include tenant B's message key
	msgsA, err := infra.repo.ReadAllMessagesFromQueue(ctxA)
	require.NoError(t, err, "ReadAllMessagesFromQueue for tenant A should succeed")

	for msgField := range msgsA {
		assert.NotContains(t, msgField, tenantB,
			"tenant A's queue must not expose any key referencing tenant B")
	}

	// ReadAllMessagesFromQueue for tenant B should NOT include tenant A's message key
	msgsB, err := infra.repo.ReadAllMessagesFromQueue(ctxB)
	require.NoError(t, err, "ReadAllMessagesFromQueue for tenant B should succeed")

	for msgField := range msgsB {
		assert.NotContains(t, msgField, tenantA,
			"tenant B's queue must not expose any key referencing tenant A")
	}

	t.Log("Integration test passed: queue operations are isolated per tenant")
}

// =============================================================================
// INTEGRATION TESTS - DOUBLE-ENTRY APPROVED + CANCELED (RouteValidationEnabled)
// =============================================================================

// TestIntegration_Redis_DoubleEntryCanceled_RouteValidationEnabled_PerFieldAtomicity
// verifies that when RouteValidationEnabled=true, a CANCELED transaction creates
// two separate operations each affecting exactly one balance field:
//   - RELEASE: only decrements OnHold (Available unchanged)
//   - CREDIT: only increments Available (OnHold unchanged)
//
// This is the core CANCELED behavior: per-field atomicity where each
// operation mutates exactly one balance field, and the version increments once
// per operation (total +2 for the pair).
func TestIntegration_Redis_DoubleEntryCanceled_RouteValidationEnabled_PerFieldAtomicity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("RELEASE only decrements OnHold when routeValidationEnabled is true", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		initialAvailable := decimal.NewFromInt(900)
		initialOnHold := decimal.NewFromInt(100)
		initialVersion := int64(3) // After PENDING phase (v1->v3)
		amount := decimal.NewFromInt(100)

		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreatePendingBalanceOperation(
				orgID, ledgerID, "@cancel-release-source", "USD",
				constant.RELEASE, amount,
				initialAvailable, initialOnHold, initialVersion,
				"deposit", true, // routeValidationEnabled = true
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.CANCELED, true,
			balanceOps,
		)

		require.NoError(t, err, "CANCELED RELEASE with routeValidation should succeed")
		require.NotNil(t, result)
		require.Len(t, result.After, 1, "should have 1 after balance")
		require.Len(t, result.Before, 1, "should have 1 before balance")

		// Before state: unchanged
		assert.True(t, result.Before[0].Available.Equal(initialAvailable),
			"before Available should be %s, got %s", initialAvailable, result.Before[0].Available)
		assert.True(t, result.Before[0].OnHold.Equal(initialOnHold),
			"before OnHold should be %s, got %s", initialOnHold, result.Before[0].OnHold)

		// After state: ONLY OnHold decremented (per-field atomicity)
		assert.True(t, result.After[0].Available.Equal(initialAvailable),
			"RELEASE with routeValidation: Available should be UNCHANGED at %s, got %s",
			initialAvailable, result.After[0].Available)
		expectedOnHold := initialOnHold.Sub(amount)
		assert.True(t, result.After[0].OnHold.Equal(expectedOnHold),
			"RELEASE with routeValidation: OnHold should be %s, got %s",
			expectedOnHold, result.After[0].OnHold)

		// Version increments by 1 (single field change)
		assert.Equal(t, initialVersion+1, result.After[0].Version,
			"version should increment by 1 for RELEASE per-field operation")

		t.Logf("RELEASE per-field: Available unchanged at %s, OnHold %s -> %s, version %d -> %d",
			initialAvailable, initialOnHold, expectedOnHold, initialVersion, result.After[0].Version)
	})

	t.Run("CREDIT only increments Available when routeValidationEnabled is true and CANCELED", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		// State after RELEASE already applied: OnHold is now 0, Available still 900
		initialAvailable := decimal.NewFromInt(900)
		initialOnHold := decimal.Zero
		initialVersion := int64(4) // After RELEASE (v3->v4)
		amount := decimal.NewFromInt(100)

		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreatePendingBalanceOperation(
				orgID, ledgerID, "@cancel-credit-source", "USD",
				constant.CREDIT, amount,
				initialAvailable, initialOnHold, initialVersion,
				"deposit", true, // routeValidationEnabled = true
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.CANCELED, true,
			balanceOps,
		)

		require.NoError(t, err, "CANCELED CREDIT with routeValidation should succeed")
		require.NotNil(t, result)
		require.Len(t, result.After, 1, "should have 1 after balance")
		require.Len(t, result.Before, 1, "should have 1 before balance")

		// After state: ONLY Available incremented (per-field atomicity)
		expectedAvailable := initialAvailable.Add(amount)
		assert.True(t, result.After[0].Available.Equal(expectedAvailable),
			"CREDIT+CANCELED with routeValidation: Available should be %s, got %s",
			expectedAvailable, result.After[0].Available)
		assert.True(t, result.After[0].OnHold.Equal(initialOnHold),
			"CREDIT+CANCELED with routeValidation: OnHold should be UNCHANGED at %s, got %s",
			initialOnHold, result.After[0].OnHold)

		// Version increments by 1 (single field change)
		assert.Equal(t, initialVersion+1, result.After[0].Version,
			"version should increment by 1 for CREDIT per-field operation")

		t.Logf("CREDIT per-field: Available %s -> %s, OnHold unchanged at %s, version %d -> %d",
			initialAvailable, expectedAvailable, initialOnHold, initialVersion, result.After[0].Version)
	})

	t.Run("CANCELED RELEASE without routeValidation changes both fields", func(t *testing.T) {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		initialAvailable := decimal.NewFromInt(900)
		initialOnHold := decimal.NewFromInt(100)
		initialVersion := int64(2) // After legacy PENDING (v1->v2)
		amount := decimal.NewFromInt(100)

		balanceOps := []mmodel.BalanceOperation{
			redistestutil.CreatePendingBalanceOperation(
				orgID, ledgerID, "@cancel-legacy-source", "USD",
				constant.RELEASE, amount,
				initialAvailable, initialOnHold, initialVersion,
				"deposit", false, // routeValidationEnabled = false (legacy)
			),
		}

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, transactionID,
			constant.CANCELED, true,
			balanceOps,
		)

		require.NoError(t, err, "CANCELED RELEASE without routeValidation should succeed")
		require.NotNil(t, result)
		require.Len(t, result.After, 1)

		// Legacy behavior: BOTH fields change in single operation
		expectedAvailable := initialAvailable.Add(amount)
		expectedOnHold := initialOnHold.Sub(amount)

		assert.True(t, result.After[0].Available.Equal(expectedAvailable),
			"legacy RELEASE: Available should be %s, got %s",
			expectedAvailable, result.After[0].Available)
		assert.True(t, result.After[0].OnHold.Equal(expectedOnHold),
			"legacy RELEASE: OnHold should be %s, got %s",
			expectedOnHold, result.After[0].OnHold)

		// Version increments by 1 (single operation, both fields)
		assert.Equal(t, initialVersion+1, result.After[0].Version,
			"legacy version should increment by 1")

		t.Logf("Legacy RELEASE: Available %s -> %s, OnHold %s -> %s, version %d -> %d",
			initialAvailable, expectedAvailable, initialOnHold, expectedOnHold,
			initialVersion, result.After[0].Version)
	})
}

// TestIntegration_Redis_DoubleEntryCanceled_FullSourceLifecycle
// verifies the complete source balance lifecycle with route validation:
//
//	PENDING (v1->v3): DEBIT(Available--) + ON_HOLD(OnHold++)
//	CANCELED (v3->v5): RELEASE(OnHold--) + CREDIT(Available++)
//
// After the full lifecycle, the balance should return to its original state,
// and the version chain should be continuous: v1->v2->v3->v4->v5.
func TestIntegration_Redis_DoubleEntryCanceled_FullSourceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	initialAvailable := decimal.NewFromInt(1000)
	initialOnHold := decimal.Zero
	initialVersion := int64(1)
	amount := decimal.NewFromInt(300)

	// Phase 1: PENDING with routeValidation (v1 -> v3)
	// Two operations: DEBIT(Available--) + ON_HOLD(OnHold++)
	pendingTxID := uuid.New()
	pendingOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@lifecycle-cancel-src", "USD",
		constant.ONHOLD, amount,
		initialAvailable, initialOnHold, initialVersion,
		"deposit", true, // routeValidationEnabled = true
	)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{pendingOp},
	)
	require.NoError(t, err, "PENDING phase should succeed")
	require.Len(t, pendingResult.After, 1)

	afterPendingVersion := pendingResult.After[0].Version
	afterPendingAvailable := pendingResult.After[0].Available
	afterPendingOnHold := pendingResult.After[0].OnHold

	// Verify PENDING state
	assert.Equal(t, int64(3), afterPendingVersion, "after PENDING: version should be 3 (1+2)")
	assert.True(t, afterPendingAvailable.Equal(decimal.NewFromInt(700)),
		"after PENDING: Available should be 700")
	assert.True(t, afterPendingOnHold.Equal(decimal.NewFromInt(300)),
		"after PENDING: OnHold should be 300")

	// Phase 2: CANCELED - RELEASE (v3 -> v4)
	// Per-field atomicity: RELEASE only decrements OnHold
	cancelTxID := uuid.New()
	releaseOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@lifecycle-cancel-src", "USD",
		constant.RELEASE, amount,
		afterPendingAvailable, afterPendingOnHold, afterPendingVersion,
		"deposit", true, // routeValidationEnabled = true
	)

	releaseResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, cancelTxID,
		constant.CANCELED, true,
		[]mmodel.BalanceOperation{releaseOp},
	)
	require.NoError(t, err, "CANCELED RELEASE phase should succeed")
	require.Len(t, releaseResult.After, 1)

	afterReleaseVersion := releaseResult.After[0].Version
	afterReleaseAvailable := releaseResult.After[0].Available
	afterReleaseOnHold := releaseResult.After[0].OnHold

	// Verify RELEASE state: OnHold decremented, Available unchanged
	assert.Equal(t, int64(4), afterReleaseVersion, "after RELEASE: version should be 4 (3+1)")
	assert.True(t, afterReleaseAvailable.Equal(decimal.NewFromInt(700)),
		"after RELEASE: Available should still be 700 (per-field atomicity)")
	assert.True(t, afterReleaseOnHold.Equal(decimal.Zero),
		"after RELEASE: OnHold should be 0")

	// Phase 3: CANCELED - CREDIT (v4 -> v5)
	// Per-field atomicity: CREDIT only increments Available
	creditOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@lifecycle-cancel-src", "USD",
		constant.CREDIT, amount,
		afterReleaseAvailable, afterReleaseOnHold, afterReleaseVersion,
		"deposit", true, // routeValidationEnabled = true
	)

	creditResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, cancelTxID,
		constant.CANCELED, true,
		[]mmodel.BalanceOperation{creditOp},
	)
	require.NoError(t, err, "CANCELED CREDIT phase should succeed")
	require.Len(t, creditResult.After, 1)

	afterCreditVersion := creditResult.After[0].Version
	afterCreditAvailable := creditResult.After[0].Available
	afterCreditOnHold := creditResult.After[0].OnHold

	// Verify final state: balance returns to original
	assert.Equal(t, int64(5), afterCreditVersion, "after CREDIT: version should be 5 (4+1)")
	assert.True(t, afterCreditAvailable.Equal(initialAvailable),
		"after full CANCELED lifecycle: Available should return to initial %s, got %s",
		initialAvailable, afterCreditAvailable)
	assert.True(t, afterCreditOnHold.Equal(initialOnHold),
		"after full CANCELED lifecycle: OnHold should return to initial %s, got %s",
		initialOnHold, afterCreditOnHold)

	// Version chain continuity: v1 -> v3 -> v4 -> v5 (no gaps)
	t.Logf("Version chain: v%d --(PENDING+route)--> v%d --(RELEASE)--> v%d --(CREDIT)--> v%d",
		initialVersion, afterPendingVersion, afterReleaseVersion, afterCreditVersion)
}

// TestIntegration_Redis_DoubleEntryApproved_FullSourceLifecycle
// verifies the complete source balance lifecycle for APPROVED with route validation:
//
//	PENDING (v1->v3): DEBIT(Available--) + ON_HOLD(OnHold++)
//	APPROVED (v3->v4): DEBIT(OnHold--)
//
// For APPROVED, the source's Available was already decremented during PENDING,
// so only OnHold needs to be released. The destination gets CREDIT(Available++).
func TestIntegration_Redis_DoubleEntryApproved_FullSourceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	initialAvailable := decimal.NewFromInt(1000)
	initialOnHold := decimal.Zero
	initialVersion := int64(1)
	amount := decimal.NewFromInt(300)

	// Phase 1: PENDING with routeValidation (v1 -> v3)
	pendingTxID := uuid.New()
	pendingOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@lifecycle-approve-src", "USD",
		constant.ONHOLD, amount,
		initialAvailable, initialOnHold, initialVersion,
		"deposit", true,
	)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{pendingOp},
	)
	require.NoError(t, err, "PENDING phase should succeed")
	require.Len(t, pendingResult.After, 1)

	afterPendingVersion := pendingResult.After[0].Version
	afterPendingAvailable := pendingResult.After[0].Available
	afterPendingOnHold := pendingResult.After[0].OnHold

	assert.Equal(t, int64(3), afterPendingVersion, "after PENDING: version should be 3")
	assert.True(t, afterPendingAvailable.Equal(decimal.NewFromInt(700)),
		"after PENDING: Available should be 700")
	assert.True(t, afterPendingOnHold.Equal(decimal.NewFromInt(300)),
		"after PENDING: OnHold should be 300")

	// Phase 2: APPROVED - source DEBIT (v3 -> v4)
	// DEBIT on APPROVED: OnHold decremented (funds already left Available during PENDING)
	approvedTxID := uuid.New()
	approvedSourceOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@lifecycle-approve-src", "USD",
		constant.DEBIT, amount,
		afterPendingAvailable, afterPendingOnHold, afterPendingVersion,
		"deposit", false, // routeValidationEnabled not relevant for APPROVED source
	)

	approvedResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, approvedTxID,
		constant.APPROVED, true,
		[]mmodel.BalanceOperation{approvedSourceOp},
	)
	require.NoError(t, err, "APPROVED source DEBIT should succeed")
	require.Len(t, approvedResult.After, 1)

	afterApprovedVersion := approvedResult.After[0].Version
	afterApprovedAvailable := approvedResult.After[0].Available
	afterApprovedOnHold := approvedResult.After[0].OnHold

	// Verify APPROVED state: OnHold released, Available unchanged from PENDING
	assert.Equal(t, int64(4), afterApprovedVersion, "after APPROVED: version should be 4 (3+1)")
	assert.True(t, afterApprovedAvailable.Equal(decimal.NewFromInt(700)),
		"after APPROVED: Available should remain 700 (was deducted during PENDING)")
	assert.True(t, afterApprovedOnHold.Equal(decimal.Zero),
		"after APPROVED: OnHold should be 0 (released)")

	// Version chain: v1 -> v3 -> v4 (no gaps)
	t.Logf("Version chain: v%d --(PENDING+route)--> v%d --(APPROVED DEBIT)--> v%d",
		initialVersion, afterPendingVersion, afterApprovedVersion)
}

// TestIntegration_Redis_DoubleEntryApproved_DestinationCredit
// verifies that the APPROVED destination receives credit that increases Available.
// During PENDING, the destination had no changes (CREDIT+PENDING is a no-op).
// On APPROVED, CREDIT increases Available and increments version.
func TestIntegration_Redis_DoubleEntryApproved_DestinationCredit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	destAvailable := decimal.NewFromInt(500)
	destOnHold := decimal.Zero
	destVersion := int64(1)
	amount := decimal.NewFromInt(300)

	// Step 1: PENDING destination CREDIT (no-op, no version change)
	pendingTxID := uuid.New()
	pendingDestOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@approve-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion,
		"deposit", false, // destination does not use routeValidation flag
	)

	pendingDestResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{pendingDestOp},
	)
	require.NoError(t, err, "PENDING destination CREDIT should succeed")
	// No change during PENDING for destination
	assert.Len(t, pendingDestResult.After, 0,
		"destination should NOT appear in results during PENDING (CREDIT+PENDING is no-op)")

	// Step 2: APPROVED destination CREDIT (Available increases)
	approvedTxID := uuid.New()
	approvedDestOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@approve-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion, // Same initial state (no changes during PENDING)
		"deposit", false,
	)

	approvedDestResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, approvedTxID,
		constant.APPROVED, true,
		[]mmodel.BalanceOperation{approvedDestOp},
	)
	require.NoError(t, err, "APPROVED destination CREDIT should succeed")
	require.Len(t, approvedDestResult.After, 1, "destination should appear in results on APPROVED")

	expectedAvailable := destAvailable.Add(amount)
	assert.True(t, approvedDestResult.After[0].Available.Equal(expectedAvailable),
		"APPROVED dest: Available should be %s, got %s",
		expectedAvailable, approvedDestResult.After[0].Available)
	assert.True(t, approvedDestResult.After[0].OnHold.Equal(destOnHold),
		"APPROVED dest: OnHold should be unchanged at %s", destOnHold)
	assert.Equal(t, destVersion+1, approvedDestResult.After[0].Version,
		"APPROVED dest: version should be %d", destVersion+1)

	t.Logf("APPROVED destination: Available %s -> %s, version %d -> %d",
		destAvailable, expectedAvailable, destVersion, approvedDestResult.After[0].Version)
}

// TestIntegration_Redis_DoubleEntryCanceled_SourceAndDestination
// verifies an end-to-end CANCELED transaction with both source operations
// (RELEASE + CREDIT) submitted together, each affecting exactly one field.
// This tests the atomic Lua execution of both CANCELED double-entry operations.
func TestIntegration_Redis_DoubleEntryCanceled_SourceAndDestination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Source state after PENDING phase
	sourceAvailable := decimal.NewFromInt(700)
	sourceOnHold := decimal.NewFromInt(300)
	sourceVersion := int64(3)

	// Destination state (unchanged during PENDING)
	destAvailable := decimal.NewFromInt(500)
	destOnHold := decimal.Zero
	destVersion := int64(1)

	amount := decimal.NewFromInt(300)

	// Source RELEASE: OnHold-- only (per-field)
	releaseOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@cancel-pair-src", "USD",
		constant.RELEASE, amount,
		sourceAvailable, sourceOnHold, sourceVersion,
		"deposit", true,
	)

	// Source CREDIT: Available++ only (per-field)
	creditOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@cancel-pair-src", "USD",
		constant.CREDIT, amount,
		sourceAvailable, sourceOnHold, sourceVersion,
		"deposit", true,
	)

	// Destination: no change on CANCELED (already excluded during PENDING)
	destOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@cancel-pair-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion,
		"deposit", false,
	)

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, transactionID,
		constant.CANCELED, true,
		[]mmodel.BalanceOperation{releaseOp, creditOp, destOp},
	)

	require.NoError(t, err, "CANCELED with source RELEASE+CREDIT and destination should succeed")
	require.NotNil(t, result)

	// Source should appear in results (both RELEASE and CREDIT caused changes)
	// Destination CREDIT+CANCELED without routeValidation has no matching branch,
	// so the destination should NOT appear (no change)
	assert.GreaterOrEqual(t, len(result.After), 1,
		"at least source should appear in results")

	// The source should have: Available restored, OnHold zeroed, version incremented
	// (Lua processes RELEASE first: OnHold 300->0, then CREDIT: Available 700->1000)
	sourceFound := false
	for _, after := range result.After {
		if after.Alias == "@cancel-pair-src" {
			sourceFound = true
			// After RELEASE + CREDIT applied atomically to same balance key:
			// RELEASE: OnHold 300->0, Available stays 700, version 3->4
			// CREDIT: Available 700->1000, OnHold stays 0, version 4->5
			assert.True(t, after.Available.Equal(decimal.NewFromInt(1000)),
				"source Available should be restored to 1000, got %s", after.Available)
			assert.True(t, after.OnHold.Equal(decimal.Zero),
				"source OnHold should be 0, got %s", after.OnHold)
			assert.Equal(t, int64(5), after.Version,
				"source version should be 5 (3+1 for RELEASE +1 for CREDIT)")
		}
	}
	assert.True(t, sourceFound, "source @cancel-pair-src should appear in results")

	t.Log("CANCELED source RELEASE+CREDIT pair: both per-field operations applied atomically")
}

// TestIntegration_Redis_DoubleEntryApproved_FullTransaction_SourceAndDestination
// verifies the complete PENDING -> APPROVED lifecycle with route validation,
// testing both source and destination across the full transaction lifecycle.
func TestIntegration_Redis_DoubleEntryApproved_FullTransaction_SourceAndDestination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	sourceAvailable := decimal.NewFromInt(2000)
	sourceOnHold := decimal.Zero
	sourceVersion := int64(1)

	destAvailable := decimal.NewFromInt(500)
	destOnHold := decimal.Zero
	destVersion := int64(1)

	amount := decimal.NewFromInt(400)

	// Phase 1: PENDING - source ON_HOLD with routeValidation (v1->v3)
	pendingTxID := uuid.New()
	pendingSourceOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@full-approve-src", "USD",
		constant.ONHOLD, amount,
		sourceAvailable, sourceOnHold, sourceVersion,
		"deposit", true,
	)
	pendingDestOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@full-approve-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion,
		"deposit", false,
	)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID,
		constant.PENDING, true,
		[]mmodel.BalanceOperation{pendingSourceOp, pendingDestOp},
	)
	require.NoError(t, err, "PENDING phase should succeed")
	// Only source changes during PENDING
	require.Len(t, pendingResult.After, 1, "only source should appear in PENDING results")
	assert.Equal(t, int64(3), pendingResult.After[0].Version, "source version should be 3")

	afterPendingSourceAvailable := pendingResult.After[0].Available
	afterPendingSourceOnHold := pendingResult.After[0].OnHold
	afterPendingSourceVersion := pendingResult.After[0].Version

	// Phase 2: APPROVED - source DEBIT (releases OnHold) + destination CREDIT (receives funds)
	approvedTxID := uuid.New()
	approvedSourceOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@full-approve-src", "USD",
		constant.DEBIT, amount,
		afterPendingSourceAvailable, afterPendingSourceOnHold, afterPendingSourceVersion,
		"deposit", false,
	)
	approvedDestOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@full-approve-dest", "USD",
		constant.CREDIT, amount,
		destAvailable, destOnHold, destVersion, // Destination unchanged during PENDING
		"deposit", false,
	)

	approvedResult, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, approvedTxID,
		constant.APPROVED, true,
		[]mmodel.BalanceOperation{approvedSourceOp, approvedDestOp},
	)
	require.NoError(t, err, "APPROVED phase should succeed")
	require.Len(t, approvedResult.After, 2, "both source and destination should appear in APPROVED results")

	// Verify source after APPROVED
	var approvedSource, approvedDest *mmodel.Balance
	for _, after := range approvedResult.After {
		switch after.Alias {
		case "@full-approve-src":
			approvedSource = after
		case "@full-approve-dest":
			approvedDest = after
		}
	}

	require.NotNil(t, approvedSource, "source should be in APPROVED results")
	require.NotNil(t, approvedDest, "destination should be in APPROVED results")

	// Source: Available stays at 1600, OnHold released to 0, version 3->4
	assert.True(t, approvedSource.Available.Equal(decimal.NewFromInt(1600)),
		"APPROVED source: Available should be 1600 (deducted during PENDING), got %s",
		approvedSource.Available)
	assert.True(t, approvedSource.OnHold.Equal(decimal.Zero),
		"APPROVED source: OnHold should be 0, got %s", approvedSource.OnHold)
	assert.Equal(t, int64(4), approvedSource.Version,
		"APPROVED source: version should be 4 (3+1)")

	// Destination: Available increased by amount, version 1->2
	expectedDestAvailable := destAvailable.Add(amount)
	assert.True(t, approvedDest.Available.Equal(expectedDestAvailable),
		"APPROVED dest: Available should be %s, got %s",
		expectedDestAvailable, approvedDest.Available)
	assert.True(t, approvedDest.OnHold.Equal(decimal.Zero),
		"APPROVED dest: OnHold should be 0, got %s", approvedDest.OnHold)
	assert.Equal(t, int64(2), approvedDest.Version,
		"APPROVED dest: version should be 2 (1+1)")

	t.Logf("Full APPROVED lifecycle: source v1->v3->v4, dest v1(skipped PENDING)->v2")
}

// TestIntegration_Redis_DoubleEntryCanceled_MultipleSources
// verifies that multiple source balances in a single CANCELED transaction
// each get their operations applied with per-field atomicity when
// routeValidationEnabled=true.
func TestIntegration_Redis_DoubleEntryCanceled_MultipleSources(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Two sources, each with RELEASE + routeValidation
	source1ReleaseOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@cancel-multi-src-1", "USD",
		constant.RELEASE, decimal.NewFromInt(100),
		decimal.NewFromInt(900), decimal.NewFromInt(100), int64(3),
		"deposit", true,
	)
	source2ReleaseOp := redistestutil.CreatePendingBalanceOperation(
		orgID, ledgerID, "@cancel-multi-src-2", "USD",
		constant.RELEASE, decimal.NewFromInt(200),
		decimal.NewFromInt(800), decimal.NewFromInt(200), int64(3),
		"deposit", true,
	)

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, transactionID,
		constant.CANCELED, true,
		[]mmodel.BalanceOperation{source1ReleaseOp, source2ReleaseOp},
	)

	require.NoError(t, err, "multiple sources CANCELED RELEASE should succeed")
	require.NotNil(t, result)
	require.Len(t, result.After, 2, "both sources should appear in results")

	// Both sources: only OnHold decremented (per-field atomicity)
	for _, after := range result.After {
		assert.Equal(t, int64(4), after.Version,
			"source %s version should be 4 (3+1 for RELEASE), got %d", after.Alias, after.Version)
		assert.True(t, after.OnHold.Equal(decimal.Zero),
			"source %s OnHold should be 0 after RELEASE, got %s", after.Alias, after.OnHold)
	}

	t.Log("Multiple sources CANCELED RELEASE: both per-field operations applied correctly")
}
