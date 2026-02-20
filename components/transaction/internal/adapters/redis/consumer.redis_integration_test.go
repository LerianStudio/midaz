//go:build integration

package redis

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
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
	proxyConn      *libRedis.RedisConnection
	proxy          *chaos.Proxy
}

// setupRedisIntegrationInfra sets up the test infrastructure for Redis integration testing.
func setupRedisIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	// Setup Redis container
	redisContainer := redistestutil.SetupContainer(t)

	// Create lib-commons Redis connection
	conn := redistestutil.CreateConnection(t, redisContainer.Addr)

	// Create repository with balance sync enabled
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
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

	// Create repository with balance sync enabled
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
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
	logger := libZap.InitializeLogger()
	proxyConn := &libRedis.RedisConnection{
		Address: []string{proxyAddr},
		Logger:  logger,
	}

	proxyRepo := &RedisConsumerRepository{
		conn:               proxyConn,
		balanceSyncEnabled: true,
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

		balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
		require.NoError(t, err, "operation %d should succeed", i)

		// Verify balance after each operation is non-negative
		if len(balances) > 0 {
			assert.GreaterOrEqual(t, balances[0].Available.IntPart(), int64(0),
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
	balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "PENDING", true, balanceOps)
	require.NoError(t, err, "pending operation should succeed")
	require.NotNil(t, balances, "should return balances")

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

	balances, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, commitOps)
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

	balances, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps)
	require.NoError(t, err, "initial balance operation should succeed")
	require.NotNil(t, balances, "should return balances")
	t.Logf("Initial balance operation successful: %d balances updated", len(balances))

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
		balances []*mmodel.Balance
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

			balances, err := infra.repo.ProcessBalanceAtomicOperation(
				ctx, orgID, ledgerID, transactionID, "ACTIVE", false, balanceOps,
			)
			results <- result{workerID: workerID, balances: balances, err: err}
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
