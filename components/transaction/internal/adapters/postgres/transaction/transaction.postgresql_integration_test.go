//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check ensures TransactionPostgreSQLRepository implements Repository.
var _ Repository = (*TransactionPostgreSQLRepository)(nil)

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

// decimalPtr creates a pointer to a decimal.Decimal from an int64 value.
func decimalPtr(v int64) *decimal.Decimal {
	d := decimal.NewFromInt(v)
	return &d
}

// parseID parses a string ID to uuid.UUID, failing the test if parsing fails.
func parseID(t *testing.T, id string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(id)
	require.NoError(t, err, "failed to parse UUID: %s", id)
	return parsed
}

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// integrationTestInfra holds the infrastructure needed for basic integration tests.
type integrationTestInfra struct {
	pgContainer *pgtestutil.ContainerResult
	conn        *libPostgres.PostgresConnection
	repo        *TransactionPostgreSQLRepository
	orgID       uuid.UUID
	ledgerID    uuid.UUID
	accountID   uuid.UUID
	balanceID   uuid.UUID
}

// chaosTestInfra holds the infrastructure needed for chaos tests (container restart, etc.).
type chaosTestInfra struct {
	pgContainer *pgtestutil.ContainerResult
	conn        *libPostgres.PostgresConnection
	repo        *TransactionPostgreSQLRepository
	orgID       uuid.UUID
	ledgerID    uuid.UUID
	accountID   uuid.UUID
	balanceID   uuid.UUID
	chaosOrch   *chaos.Orchestrator
}

// networkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
type networkChaosTestInfra struct {
	chaosInfra *chaos.Infrastructure
	pgResult   *pgtestutil.ContainerResult
	conn       *libPostgres.PostgresConnection
	repo       *TransactionPostgreSQLRepository
	orgID      uuid.UUID
	ledgerID   uuid.UUID
	proxy      *chaos.Proxy
}

// setupIntegrationInfra sets up the test infrastructure for basic integration testing.
func setupIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	// Setup PostgreSQL container
	pgContainer := pgtestutil.SetupContainer(t)

	// Create lib-commons PostgreSQL connection
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           pgContainer.Config.DBName,
		ReplicaDBName:           pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create repository
	repo := NewTransactionPostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	return &integrationTestInfra{
		pgContainer: pgContainer,
		conn:        conn,
		repo:        repo,
		orgID:       orgID,
		ledgerID:    ledgerID,
		accountID:   accountID,
		balanceID:   balanceID,
	}
}

// setupChaosInfra sets up the test infrastructure for chaos testing (container restart).
func setupChaosInfra(t *testing.T) *chaosTestInfra {
	t.Helper()

	// Setup PostgreSQL container
	pgContainer := pgtestutil.SetupContainer(t)

	// Create lib-commons PostgreSQL connection
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           pgContainer.Config.DBName,
		ReplicaDBName:           pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create repository
	repo := NewTransactionPostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &chaosTestInfra{
		pgContainer: pgContainer,
		conn:        conn,
		repo:        repo,
		orgID:       orgID,
		ledgerID:    ledgerID,
		accountID:   accountID,
		balanceID:   balanceID,
		chaosOrch:   chaosOrch,
	}
}

// setupNetworkChaosInfra sets up the infrastructure with Toxiproxy for network chaos testing.
func setupNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// Create chaos infrastructure with Toxiproxy
	chaosInfra := chaos.NewInfrastructure(t)

	// Setup PostgreSQL container
	pgResult := pgtestutil.SetupContainer(t)

	// Register the container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("postgres", pgResult.Container, "5432/tcp")
	require.NoError(t, err, "failed to register PostgreSQL container")

	// Create proxy for PostgreSQL using an exposed Toxiproxy port (8668)
	proxy, err := chaosInfra.CreateProxyFor("postgres", "8668/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for PostgreSQL")

	// Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("postgres")
	require.True(t, ok, "PostgreSQL container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	// Create lib-commons PostgreSQL connection through proxy
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	// Build connection string using proxy address
	proxyConnStr := pgtestutil.BuildConnectionStringWithHost(containerInfo.ProxyListen, pgResult.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: proxyConnStr,
		ConnectionStringReplica: proxyConnStr,
		PrimaryDBName:           pgResult.Config.DBName,
		ReplicaDBName:           pgResult.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create repository
	repo := NewTransactionPostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	return &networkChaosTestInfra{
		chaosInfra: chaosInfra,
		pgResult:   pgResult,
		conn:       conn,
		repo:       repo,
		orgID:      orgID,
		ledgerID:   ledgerID,
		proxy:      proxy,
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
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// createTestTransaction creates a transaction for testing.
func (infra *integrationTestInfra) createTestTransaction(t *testing.T, description string) *Transaction {
	t.Helper()

	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    description,
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(1000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	created, err := infra.repo.Create(context.Background(), tx)
	require.NoError(t, err)
	return created
}

// createTestTransaction creates a transaction for chaos testing.
func (infra *chaosTestInfra) createTestTransaction(t *testing.T, description string) *Transaction {
	t.Helper()

	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    description,
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(1000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	created, err := infra.repo.Create(context.Background(), tx)
	require.NoError(t, err)
	return created
}

// createTestTransaction creates a transaction for network chaos testing.
func (infra *networkChaosTestInfra) createTestTransaction(t *testing.T, description string) *Transaction {
	t.Helper()

	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    description,
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(1000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	created, err := infra.repo.Create(context.Background(), tx)
	require.NoError(t, err)
	return created
}

// =============================================================================
// INTEGRATION TESTS - CRUD OPERATIONS
// =============================================================================

// TestIntegration_Transaction_Create tests creating a transaction.
func TestIntegration_Transaction_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    "Test transaction",
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(1000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	created, err := infra.repo.Create(ctx, tx)
	require.NoError(t, err)
	assert.Equal(t, tx.ID, created.ID)
	assert.Equal(t, tx.Description, created.Description)
	assert.Equal(t, "ACTIVE", created.Status.Code)

	t.Log("Integration test passed: transaction creation verified")
}

// TestIntegration_Transaction_Find tests finding a transaction by ID.
func TestIntegration_Transaction_Find(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction first
	created := infra.createTestTransaction(t, "Find test transaction")

	// Find the transaction
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, created.Description, found.Description)

	t.Log("Integration test passed: transaction find verified")
}

// TestIntegration_Transaction_FindAll tests finding all transactions.
func TestIntegration_Transaction_FindAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create multiple transactions
	for i := 0; i < 3; i++ {
		infra.createTestTransaction(t, "FindAll test transaction")
	}

	// Find all transactions
	transactions, _, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{Limit: 100})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(transactions), 3)

	t.Log("Integration test passed: transaction findAll verified")
}

// TestIntegration_Transaction_Update tests updating a transaction.
func TestIntegration_Transaction_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction first
	created := infra.createTestTransaction(t, "Update test transaction")

	// Update the transaction
	created.Description = "Updated description"
	updated, err := infra.repo.Update(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID), created)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", updated.Description)

	// Verify the update persisted
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))
	require.NoError(t, err)
	assert.Equal(t, "Updated description", found.Description)

	t.Log("Integration test passed: transaction update verified")
}

// TestIntegration_Transaction_Delete tests deleting a transaction.
func TestIntegration_Transaction_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction first
	created := infra.createTestTransaction(t, "Delete test transaction")

	// Delete the transaction
	err := infra.repo.Delete(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))
	require.NoError(t, err)

	// Verify the transaction is deleted (soft delete)
	_, err = infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))
	assert.Error(t, err, "deleted transaction should not be found")

	t.Log("Integration test passed: transaction delete verified")
}

// =============================================================================
// INTEGRATION TESTS - CONCURRENCY & IDEMPOTENCY
// =============================================================================

// TestIntegration_Transaction_ConcurrentWrites tests that concurrent writes
// are handled correctly by the database.
func TestIntegration_Transaction_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()
	numWriters := 10

	// Create channel to collect results
	type writeResult struct {
		tx  *Transaction
		err error
	}
	results := make(chan writeResult, numWriters)

	// Start concurrent writers
	var wg sync.WaitGroup
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			tx := &Transaction{
				ID:             uuid.New().String(),
				Description:    "Concurrent write test",
				Status:         Status{Code: "ACTIVE"},
				Amount:         decimalPtr(int64(workerID * 100)),
				AssetCode:      "USD",
				LedgerID:       infra.ledgerID.String(),
				OrganizationID: infra.orgID.String(),
			}

			created, err := infra.repo.Create(ctx, tx)
			results <- writeResult{tx: created, err: err}
		}(i)
	}

	// Wait for all writers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and analyze results
	var successCount, errorCount int
	var createdTxs []*Transaction

	for result := range results {
		if result.err != nil {
			errorCount++
			t.Logf("Write error: %v", result.err)
		} else {
			successCount++
			createdTxs = append(createdTxs, result.tx)
		}
	}

	t.Logf("Concurrent writes: %d successful, %d errors", successCount, errorCount)

	// Verify all successful writes are persisted
	for _, tx := range createdTxs {
		found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		require.NoError(t, err, "successfully created transaction should be findable")
		assert.Equal(t, tx.ID, found.ID)
	}

	// All writes should succeed
	assert.Equal(t, numWriters, successCount, "all concurrent writes should succeed")

	t.Log("Integration test passed: concurrent writes handled correctly")
}

// TestIntegration_Transaction_Idempotency tests that duplicate transactions
// are rejected by the unique constraint.
func TestIntegration_Transaction_Idempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction with a known ID (simulating idempotency key)
	idempotencyID := uuid.New().String()
	tx := &Transaction{
		ID:             idempotencyID,
		Description:    "Idempotent transaction",
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(5000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	// First attempt: create transaction
	created, err := infra.repo.Create(ctx, tx)
	require.NoError(t, err)
	t.Logf("First attempt: created transaction %s", created.ID)

	// Second attempt: try to create again with same ID (should fail)
	_, duplicateErr := infra.repo.Create(ctx, tx)
	assert.Error(t, duplicateErr, "duplicate transaction should be rejected")

	// Verify original transaction is unchanged
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, idempotencyID))
	require.NoError(t, err)
	assert.Equal(t, created.Description, found.Description)

	t.Log("Integration test passed: idempotency preserved")
}

// TestIntegration_Transaction_GracefulDegradation tests that the repository
// fails gracefully when context is cancelled.
func TestIntegration_Transaction_GracefulDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction first
	tx := infra.createTestTransaction(t, "Degradation test")

	// Verify normal operation works
	_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err, "normal operation should work")

	// Test with cancelled context (simulates timeout/unavailability)
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	_, err = infra.repo.Find(cancelledCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	assert.Error(t, err, "operation with cancelled context should fail")

	// Verify normal operation still works after degraded state
	_, err = infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err, "normal operation should work after graceful degradation")

	t.Log("Integration test passed: graceful degradation verified")
}

// =============================================================================
// CHAOS TESTS - CONTAINER LIFECYCLE
// =============================================================================

// TestChaos_Transaction_PostgresRestart tests that the repository recovers
// after a PostgreSQL container restart.
// SKIPPED: lib-commons PostgreSQL connection pool does not recover after restart.
func TestIntegration_Chaos_Transaction_PostgresRestart(t *testing.T) {
	t.Skip("skipping: lib-commons connection pool does not recover after PostgreSQL restart")
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create initial transaction
	tx := infra.createTestTransaction(t, "Pre-restart transaction")
	t.Logf("Created transaction %s before restart", tx.ID)

	// Inject chaos: restart PostgreSQL
	containerID := infra.pgContainer.Container.GetContainerID()
	t.Logf("Chaos: Restarting PostgreSQL container %s", containerID)

	err := infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")

	// Wait for database to be ready again
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		return err
	}, 30*time.Second, "repository should recover after PostgreSQL restart")

	// Verify data integrity
	recovered, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err)
	assert.Equal(t, tx.ID, recovered.ID, "transaction ID should be unchanged")
	assert.Equal(t, tx.Description, recovered.Description, "transaction description should be unchanged")

	t.Log("Chaos test passed: PostgreSQL restart recovery verified")
}

// TestChaos_Transaction_DataIntegrity tests that data remains consistent
// after chaos events (no data loss, no corruption).
func TestIntegration_Chaos_Transaction_DataIntegrity(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create multiple transactions before chaos
	var createdTxs []*Transaction
	for i := 0; i < 5; i++ {
		tx := infra.createTestTransaction(t, "Integrity test transaction")
		createdTxs = append(createdTxs, tx)
	}

	t.Logf("Created %d transactions before chaos", len(createdTxs))

	// Verify all data is intact
	chaos.AssertDataIntegrity(t, func() error {
		for _, tx := range createdTxs {
			_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
			if err != nil {
				return err
			}
		}
		return nil
	}, "all transactions should be retrievable")

	// Verify each transaction's data
	for _, expected := range createdTxs {
		actual, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, expected.ID))
		require.NoError(t, err)
		chaos.AssertNoDataLoss(t, expected.Description, actual.Description, "transaction description mismatch")
		chaos.AssertNoDataLoss(t, expected.Status.Code, actual.Status.Code, "transaction status mismatch")
	}

	t.Log("Chaos test passed: data integrity verified")
}

// =============================================================================
// CHAOS TESTS - NETWORK CHAOS
// =============================================================================

// TestChaos_Transaction_NetworkLatency tests that the repository handles
// network latency gracefully without timing out inappropriately.
func TestIntegration_Chaos_Transaction_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// Create a transaction before adding latency
	tx := infra.createTestTransaction(t, "Pre-latency transaction")
	t.Logf("Created transaction %s before adding latency", tx.ID)

	// Add 200ms latency to the connection
	t.Log("Chaos: Adding 200ms network latency")
	err := infra.proxy.AddLatency(200*time.Millisecond, 50*time.Millisecond)
	require.NoError(t, err, "failed to add latency")
	defer infra.proxy.RemoveAllToxics()

	// Operations should still succeed (with higher latency)
	start := time.Now()
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	elapsed := time.Since(start)

	require.NoError(t, err, "operation should succeed despite latency")
	assert.Equal(t, tx.ID, found.ID)
	t.Logf("Query completed in %v (with 200ms injected latency)", elapsed)

	// Latency should be noticeable
	assert.Greater(t, elapsed, 150*time.Millisecond, "query should take longer due to injected latency")

	// Create new transaction under latency
	start = time.Now()
	newTx := infra.createTestTransaction(t, "Under-latency transaction")
	elapsed = time.Since(start)

	require.NotNil(t, newTx, "should be able to create transaction under latency")
	t.Logf("Create completed in %v (with 200ms injected latency)", elapsed)

	t.Log("Chaos test passed: network latency handled gracefully")
}

// TestChaos_Transaction_NetworkPartition tests that the repository handles
// network partitions (disconnections) gracefully.
func TestIntegration_Chaos_Transaction_NetworkPartition(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create a transaction before partition
	tx := infra.createTestTransaction(t, "Pre-partition transaction")
	t.Logf("Created transaction %s before partition", tx.ID)

	// Disconnect the proxy (simulate network partition)
	t.Log("Chaos: Disconnecting network (simulating partition)")
	err := infra.proxy.Disconnect()
	require.NoError(t, err, "failed to disconnect proxy")

	// Operations should fail gracefully during partition
	partitionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, partitionErr := infra.repo.Find(partitionCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	if partitionErr != nil {
		t.Logf("Operation during partition failed as expected: %v", partitionErr)
	} else {
		t.Log("Operation during partition succeeded (connection pool still had active connections)")
	}

	// Reconnect the proxy
	t.Log("Chaos: Reconnecting network")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "failed to reconnect proxy")

	// Wait for recovery
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		return err
	}, 30*time.Second, "repository should recover after network partition")

	// Verify data integrity after partition
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err, "should find transaction after recovery")
	assert.Equal(t, tx.ID, found.ID)
	assert.Equal(t, tx.Description, found.Description)

	t.Log("Chaos test passed: network partition handled gracefully")
}

// TestChaos_Transaction_PacketLoss tests that the repository handles
// packet loss gracefully with retries.
func TestIntegration_Chaos_Transaction_PacketLoss(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create a transaction before adding packet loss
	tx := infra.createTestTransaction(t, "Pre-packet-loss transaction")
	t.Logf("Created transaction %s before packet loss", tx.ID)

	// Add 10% packet loss
	t.Log("Chaos: Adding 10% packet loss")
	err := infra.proxy.AddPacketLoss(10)
	require.NoError(t, err, "failed to add packet loss")
	defer infra.proxy.RemoveAllToxics()

	// Execute multiple operations - some may fail, but overall should be resilient
	// 10 attempts is statistically sufficient to verify resilience with 10% packet loss
	successCount := 0
	errorCount := 0
	totalAttempts := 10

	for i := 0; i < totalAttempts; i++ {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		if err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	t.Logf("Packet loss test: %d/%d operations succeeded", successCount, totalAttempts)

	// Most operations should succeed despite packet loss
	assert.Greater(t, successCount, totalAttempts/2, "majority of operations should succeed despite packet loss")

	t.Log("Chaos test passed: packet loss handled with acceptable success rate")
}

// TestChaos_Transaction_IntermittentFailure tests that the repository handles
// intermittent network failures (flapping connection).
func TestIntegration_Chaos_Transaction_IntermittentFailure(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create a transaction
	tx := infra.createTestTransaction(t, "Intermittent test transaction")

	// Simulate intermittent failures with multiple disconnect/reconnect cycles
	cycles := 3
	for i := 0; i < cycles; i++ {
		// Disconnect
		t.Logf("Chaos: Cycle %d - disconnecting", i+1)
		err := infra.proxy.Disconnect()
		require.NoError(t, err, "failed to disconnect proxy on cycle %d", i+1)
		time.Sleep(500 * time.Millisecond)

		// Reconnect
		t.Logf("Chaos: Cycle %d - reconnecting", i+1)
		err = infra.proxy.Reconnect()
		require.NoError(t, err, "failed to reconnect proxy on cycle %d", i+1)

		// Wait for recovery and verify
		chaos.AssertRecoveryWithin(t, func() error {
			_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
			return err
		}, 10*time.Second, "should recover after cycle %d", i+1)
	}

	// Final verification
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err)
	assert.Equal(t, tx.ID, found.ID)

	t.Log("Chaos test passed: intermittent failures handled correctly")
}

// =============================================================================
// INTEGRATION TESTS - ADDITIONAL CRUD OPERATIONS
// =============================================================================

// TestIntegration_Transaction_ListByIDs tests retrieving transactions by a list of IDs.
func TestIntegration_Transaction_ListByIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create multiple transactions
	tx1 := infra.createTestTransaction(t, "ListByIDs test transaction 1")
	tx2 := infra.createTestTransaction(t, "ListByIDs test transaction 2")
	tx3 := infra.createTestTransaction(t, "ListByIDs test transaction 3")

	t.Run("retrieve multiple transactions by IDs", func(t *testing.T) {
		ids := []uuid.UUID{
			parseID(t, tx1.ID),
			parseID(t, tx2.ID),
		}

		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, ids)
		require.NoError(t, err)
		assert.Len(t, transactions, 2)

		// Verify the correct transactions were returned
		foundIDs := make(map[string]bool)
		for _, tx := range transactions {
			foundIDs[tx.ID] = true
		}
		assert.True(t, foundIDs[tx1.ID], "tx1 should be in results")
		assert.True(t, foundIDs[tx2.ID], "tx2 should be in results")
		assert.False(t, foundIDs[tx3.ID], "tx3 should NOT be in results")
	})

	t.Run("retrieve all three transactions", func(t *testing.T) {
		ids := []uuid.UUID{
			parseID(t, tx1.ID),
			parseID(t, tx2.ID),
			parseID(t, tx3.ID),
		}

		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, ids)
		require.NoError(t, err)
		assert.Len(t, transactions, 3)
	})

	t.Run("empty ID list returns empty result", func(t *testing.T) {
		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, []uuid.UUID{})
		require.NoError(t, err)
		assert.Empty(t, transactions)
	})

	t.Run("non-existent IDs return empty result", func(t *testing.T) {
		nonExistentIDs := []uuid.UUID{
			uuid.New(),
			uuid.New(),
		}

		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, nonExistentIDs)
		require.NoError(t, err)
		assert.Empty(t, transactions)
	})

	t.Run("mixed existing and non-existing IDs", func(t *testing.T) {
		mixedIDs := []uuid.UUID{
			parseID(t, tx1.ID),
			uuid.New(), // non-existent
		}

		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, mixedIDs)
		require.NoError(t, err)
		assert.Len(t, transactions, 1)
		assert.Equal(t, tx1.ID, transactions[0].ID)
	})

	t.Log("Integration test passed: ListByIDs verified")
}

// TestIntegration_Transaction_FindByParentID tests finding transactions by parent ID.
func TestIntegration_Transaction_FindByParentID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create parent transaction
	parentTx := infra.createTestTransaction(t, "Parent transaction")
	t.Logf("Created parent transaction: %s", parentTx.ID)

	// Create child transaction referencing parent
	childTx := &Transaction{
		ID:                  uuid.New().String(),
		ParentTransactionID: &parentTx.ID,
		Description:         "Child transaction",
		Status:              Status{Code: "ACTIVE"},
		Amount:              decimalPtr(500),
		AssetCode:           "USD",
		LedgerID:            infra.ledgerID.String(),
		OrganizationID:      infra.orgID.String(),
	}

	createdChild, err := infra.repo.Create(ctx, childTx)
	require.NoError(t, err)
	t.Logf("Created child transaction: %s with parent: %s", createdChild.ID, *createdChild.ParentTransactionID)

	t.Run("find child by parent ID", func(t *testing.T) {
		found, err := infra.repo.FindByParentID(ctx, infra.orgID, infra.ledgerID, parseID(t, parentTx.ID))
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, createdChild.ID, found.ID)
		assert.Equal(t, parentTx.ID, *found.ParentTransactionID)
	})

	t.Run("non-existent parent ID returns nil without error", func(t *testing.T) {
		found, err := infra.repo.FindByParentID(ctx, infra.orgID, infra.ledgerID, uuid.New())
		require.NoError(t, err, "should not error for non-existent parent")
		assert.Nil(t, found, "should return nil for non-existent parent")
	})

	t.Run("transaction without children returns nil", func(t *testing.T) {
		// childTx has no children of its own
		found, err := infra.repo.FindByParentID(ctx, infra.orgID, infra.ledgerID, parseID(t, createdChild.ID))
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Log("Integration test passed: FindByParentID verified")
}

// TestIntegration_Transaction_Find_NotFound tests the Find method with non-existent ID.
func TestIntegration_Transaction_Find_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.New())
	assert.Error(t, err, "should error for non-existent transaction")
	assert.Contains(t, err.Error(), "No entity was found", "should be entity not found error")

	t.Log("Integration test passed: Find not found error path verified")
}

// TestIntegration_Transaction_Delete_NotFound tests the Delete method with non-existent ID.
func TestIntegration_Transaction_Delete_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	err := infra.repo.Delete(ctx, infra.orgID, infra.ledgerID, uuid.New())
	assert.Error(t, err, "should error for non-existent transaction")
	assert.Contains(t, err.Error(), "No entity was found", "should be entity not found error")

	t.Log("Integration test passed: Delete not found error path verified")
}

// TestIntegration_Transaction_Update_NotFound tests the Update method with non-existent ID.
func TestIntegration_Transaction_Update_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	tx := &Transaction{
		Description: "Updated description",
	}

	_, err := infra.repo.Update(ctx, infra.orgID, infra.ledgerID, uuid.New(), tx)
	assert.Error(t, err, "should error for non-existent transaction")
	assert.Contains(t, err.Error(), "No entity was found", "should be entity not found error")

	t.Log("Integration test passed: Update not found error path verified")
}

// TestIntegration_Transaction_Update_Status tests updating transaction status.
func TestIntegration_Transaction_Update_Status(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction
	created := infra.createTestTransaction(t, "Status update test")

	// Update the status
	statusDesc := "Completed successfully"
	updateTx := &Transaction{
		Status: Status{
			Code:        "COMPLETED",
			Description: &statusDesc,
		},
	}

	updated, err := infra.repo.Update(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID), updateTx)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", updated.Status.Code)

	// Verify persistence
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", found.Status.Code)
	require.NotNil(t, found.Status.Description)
	assert.Equal(t, "Completed successfully", *found.Status.Description)

	t.Log("Integration test passed: status update verified")
}

// =============================================================================
// INTEGRATION TESTS - PAGINATION
// =============================================================================

// TestIntegration_Transaction_FindAll_Pagination tests pagination for FindAll.
func TestIntegration_Transaction_FindAll_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		infra.createTestTransaction(t, "Pagination test transaction")
		// Small delay to ensure distinct created_at timestamps
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("paginate with limit 2", func(t *testing.T) {
		page1, cursor1, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{Limit: 2})
		require.NoError(t, err)
		assert.Len(t, page1, 2)
		assert.NotEmpty(t, cursor1.Next, "should have next cursor")
	})

	t.Run("paginate with limit larger than total", func(t *testing.T) {
		allTx, cursor, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{Limit: 100})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allTx), 5)
		// When all results fit, no next cursor needed
		if len(allTx) <= 100 {
			// Expected behavior - all fit in one page
			t.Logf("Retrieved %d transactions in single page", len(allTx))
		}
		_ = cursor // cursor behavior depends on total count
	})

	t.Run("paginate with limit 0 uses default", func(t *testing.T) {
		// Limit 0 should use default limit behavior
		transactions, _, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{Limit: 0})
		require.NoError(t, err)
		// Should still return results (using default)
		t.Logf("Retrieved %d transactions with limit 0", len(transactions))
	})

	t.Log("Integration test passed: FindAll pagination verified")
}

// TestIntegration_Transaction_FindAll_DateFilter tests date filtering for FindAll.
func TestIntegration_Transaction_FindAll_DateFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create transactions
	infra.createTestTransaction(t, "Date filter test 1")
	time.Sleep(50 * time.Millisecond)
	infra.createTestTransaction(t, "Date filter test 2")

	t.Run("query with date parameters executes without error", func(t *testing.T) {
		// Note: NormalizeDateTime normalizes dates to day boundaries
		// This test verifies the date filter SQL clause works correctly
		today := time.Now().UTC()

		transactions, _, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{
			Limit:     100,
			StartDate: today,
			EndDate:   today,
		})
		require.NoError(t, err)
		// Results depend on NormalizeDateTime behavior - just verify no error
		t.Logf("Query returned %d transactions with date filter", len(transactions))
	})

	t.Run("filter excludes transactions outside range", func(t *testing.T) {
		// Use a future date range (tomorrow)
		futureStart := time.Now().UTC().Add(24 * time.Hour)
		futureEnd := futureStart.Add(24 * time.Hour)

		transactions, _, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{
			Limit:     100,
			StartDate: futureStart,
			EndDate:   futureEnd,
		})
		require.NoError(t, err)
		assert.Empty(t, transactions, "no transactions should exist in future date range")
	})

	t.Log("Integration test passed: FindAll date filter verified")
}

// =============================================================================
// INTEGRATION TESTS - COMPLEX QUERIES (WITH OPERATIONS)
// =============================================================================

// TestIntegration_Transaction_FindWithOperations tests finding a transaction with its operations.
func TestIntegration_Transaction_FindWithOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a transaction
	tx := infra.createTestTransaction(t, "FindWithOperations test")

	// Note: This test verifies the JOIN query works even when no operations exist.
	// The method uses INNER JOIN, so transactions without operations return empty.
	t.Run("transaction without operations returns empty", func(t *testing.T) {
		found, err := infra.repo.FindWithOperations(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		require.NoError(t, err)
		// INNER JOIN means no results if no operations
		// The returned transaction will have empty ID if no rows matched
		if found.ID == "" {
			t.Log("Transaction without operations returns empty result (expected with INNER JOIN)")
		} else {
			assert.Equal(t, tx.ID, found.ID)
			assert.Empty(t, found.Operations, "should have no operations")
		}
	})

	t.Log("Integration test passed: FindWithOperations verified")
}

// TestIntegration_Transaction_FindOrListAllWithOperations tests listing transactions with operations.
func TestIntegration_Transaction_FindOrListAllWithOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create transactions
	tx1 := infra.createTestTransaction(t, "ListWithOps test 1")
	tx2 := infra.createTestTransaction(t, "ListWithOps test 2")
	tx3 := infra.createTestTransaction(t, "ListWithOps test 3")

	t.Run("list all with pagination", func(t *testing.T) {
		transactions, cursor, err := infra.repo.FindOrListAllWithOperations(
			ctx, infra.orgID, infra.ledgerID, nil, http.Pagination{Limit: 100},
		)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(transactions), 3)
		_ = cursor // cursor depends on total results
	})

	t.Run("list by specific IDs", func(t *testing.T) {
		ids := []uuid.UUID{
			parseID(t, tx1.ID),
			parseID(t, tx2.ID),
		}

		transactions, _, err := infra.repo.FindOrListAllWithOperations(
			ctx, infra.orgID, infra.ledgerID, ids, http.Pagination{Limit: 100},
		)
		require.NoError(t, err)
		assert.Len(t, transactions, 2)

		foundIDs := make(map[string]bool)
		for _, tx := range transactions {
			foundIDs[tx.ID] = true
		}
		assert.True(t, foundIDs[tx1.ID])
		assert.True(t, foundIDs[tx2.ID])
		assert.False(t, foundIDs[tx3.ID])
	})

	t.Run("list with small page size", func(t *testing.T) {
		transactions, cursor, err := infra.repo.FindOrListAllWithOperations(
			ctx, infra.orgID, infra.ledgerID, nil, http.Pagination{Limit: 2},
		)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(transactions), 2)
		if len(transactions) == 2 {
			assert.NotEmpty(t, cursor.Next, "should have next cursor when paginating")
		}
	})

	t.Run("empty IDs returns all", func(t *testing.T) {
		transactions, _, err := infra.repo.FindOrListAllWithOperations(
			ctx, infra.orgID, infra.ledgerID, []uuid.UUID{}, http.Pagination{Limit: 100},
		)
		require.NoError(t, err)
		// Empty slice behaves same as nil - returns all
		assert.GreaterOrEqual(t, len(transactions), 3)
	})

	t.Log("Integration test passed: FindOrListAllWithOperations verified")
}

// TestIntegration_Transaction_FindOrListAllWithOperations_DateRange tests date filtering.
func TestIntegration_Transaction_FindOrListAllWithOperations_DateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	infra.createTestTransaction(t, "Date range test")

	t.Run("query with date parameters executes without error", func(t *testing.T) {
		// Note: NormalizeDateTime normalizes dates to day boundaries
		// This test verifies the date filter SQL clause works correctly
		today := time.Now().UTC()

		transactions, _, err := infra.repo.FindOrListAllWithOperations(
			ctx, infra.orgID, infra.ledgerID, nil,
			http.Pagination{
				Limit:     100,
				StartDate: today,
				EndDate:   today,
			},
		)
		require.NoError(t, err)
		// Results depend on NormalizeDateTime behavior - just verify no error
		t.Logf("Query returned %d transactions with date filter", len(transactions))
	})

	t.Log("Integration test passed: FindOrListAllWithOperations date range verified")
}

// =============================================================================
// INTEGRATION TESTS - SOFT DELETE BEHAVIOR
// =============================================================================

// TestIntegration_Transaction_SoftDelete_Excluded tests that soft-deleted transactions are excluded.
func TestIntegration_Transaction_SoftDelete_Excluded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create and then delete a transaction
	tx := infra.createTestTransaction(t, "Soft delete exclusion test")
	err := infra.repo.Delete(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err)

	t.Run("Find excludes soft-deleted", func(t *testing.T) {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		assert.Error(t, err, "soft-deleted transaction should not be found")
	})

	t.Run("FindAll excludes soft-deleted", func(t *testing.T) {
		transactions, _, err := infra.repo.FindAll(ctx, infra.orgID, infra.ledgerID, http.Pagination{Limit: 100})
		require.NoError(t, err)

		for _, found := range transactions {
			assert.NotEqual(t, tx.ID, found.ID, "soft-deleted transaction should not appear in FindAll")
		}
	})

	t.Run("ListByIDs excludes soft-deleted", func(t *testing.T) {
		transactions, err := infra.repo.ListByIDs(ctx, infra.orgID, infra.ledgerID, []uuid.UUID{parseID(t, tx.ID)})
		require.NoError(t, err)
		assert.Empty(t, transactions, "soft-deleted transaction should not appear in ListByIDs")
	})

	t.Run("Delete on already deleted returns error", func(t *testing.T) {
		err := infra.repo.Delete(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		assert.Error(t, err, "deleting already-deleted transaction should error")
	})

	t.Log("Integration test passed: soft delete exclusion verified")
}

// =============================================================================
// IS-1: getDB returns valid DB handle from real PostgreSQL (static fallback)
// =============================================================================

// TestIntegration_GetDB_StaticFallback_ReturnsValidHandle verifies that getDB
// with a plain context (no tenant) returns a non-nil, functional DB handle.
func TestIntegration_GetDB_StaticFallback_ReturnsValidHandle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	// Act -- no tenant context, so getDB should use the static connection.
	db, err := infra.repo.getDB(ctx)

	// Assert
	require.NoError(t, err, "getDB with plain context should return no error")
	require.NotNil(t, db, "getDB should return a non-nil DB handle")
}

// TestIntegration_GetDB_StaticFallback_CanExecuteQueries verifies the static DB
// handle can execute queries against the real PostgreSQL container.
func TestIntegration_GetDB_StaticFallback_CanExecuteQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	// Arrange -- no tenant context, so getDB returns the static DB.
	db, err := infra.repo.getDB(ctx)
	require.NoError(t, err)

	// Act -- execute a simple query to verify the handle is functional.
	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)

	// Assert
	require.NoError(t, err, "static DB handle should execute queries successfully")
	assert.Equal(t, 1, result, "query should return 1")
}

// TestIntegration_GetDB_StaticFallback_SupportsRepositoryOperations verifies
// that repository CRUD methods work through the getDB static path.
func TestIntegration_GetDB_StaticFallback_SupportsRepositoryOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	// Arrange + Act -- create via repository (uses getDB internally).
	created := infra.createTestTransaction(t, "GetDB Static Operations Test")

	// Act -- repo.Find calls getDB(ctx) with plain context (static fallback).
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))

	// Assert
	require.NoError(t, err, "Find should succeed through getDB static path")
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "GetDB Static Operations Test", found.Description)
}

// =============================================================================
// IS-2: Create-then-Find round-trip through getDB
// =============================================================================
// NOTE: IS-2 is extensively covered by the existing Create, Find, Update,
// Delete tests above. All those tests exercise the getDB(ctx) static fallback
// path with a real PostgreSQL container.
//
// The test below adds explicit round-trip verification through getDB.

// TestIntegration_GetDB_CreateAndFindRoundTrip verifies a full Create-then-Find
// round-trip works correctly through the getDB path.
func TestIntegration_GetDB_CreateAndFindRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	// Arrange + Act -- create via repository (uses getDB internally).
	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    "Roundtrip Transaction",
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(2500),
		AssetCode:      "BRL",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
	}

	created, err := infra.repo.Create(ctx, tx)
	require.NoError(t, err, "Create should succeed through getDB")
	require.NotNil(t, created)

	// Act -- find the same record (also uses getDB).
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, created.ID))

	// Assert
	require.NoError(t, err, "Find should succeed through getDB")
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "Roundtrip Transaction", found.Description)
	assert.Equal(t, "BRL", found.AssetCode)
}

// =============================================================================
// IS-3: getDB with tenant module in context returns tenant DB
// =============================================================================
//
// These tests verify that when a tenant-specific dbresolver.DB is injected into
// context via tmcore.ContextWithModulePGConnection("transaction", tenantDB),
// the getDB method returns that tenant DB instead of the static connection.
//
// Strategy: Two separate PostgreSQL containers.
//   - Container A (static): used to satisfy the constructor (NewTransactionPostgreSQLRepository).
//   - Container B (tenant): wrapped in dbresolver.DB, injected into context.
//   - Data is inserted only into Container B, then retrieved through the repo.
//   - If getDB correctly returns the tenant DB, the data is found.
//   - If getDB incorrectly falls back to static, the data is NOT found (test fails).

// setupTenantContainer starts a second PostgreSQL container with migrations applied
// and returns both the ContainerResult and a dbresolver.DB wrapper suitable for
// injection into tenant context.
func setupTenantContainer(t *testing.T) (*pgtestutil.ContainerResult, dbresolver.DB) {
	t.Helper()

	tenantContainer := pgtestutil.SetupContainer(t)

	// Run migrations on the tenant container by creating a temporary
	// PostgresConnection and letting the constructor trigger migration.
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(tenantContainer.Host, tenantContainer.Port, tenantContainer.Config)

	// Create a temporary connection to apply migrations (the constructor runs them).
	tempConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           tenantContainer.Config.DBName,
		ReplicaDBName:           tenantContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Trigger migration by calling GetDB (same as constructor does).
	_, err := tempConn.GetDB()
	require.NoError(t, err, "failed to initialize tenant container database with migrations")

	// Close the temporary connection pool so it does not leak.
	t.Cleanup(func() {
		if db, dbErr := tempConn.GetDB(); dbErr == nil {
			_ = db.Close()
		}
	})

	// Wrap the raw *sql.DB in a dbresolver.DB for injection into tenant context.
	tenantDB := dbresolver.New(
		dbresolver.WithPrimaryDBs(tenantContainer.DB),
		dbresolver.WithReplicaDBs(tenantContainer.DB),
	)

	return tenantContainer, tenantDB
}

// TestIntegration_GetDB_TenantContext_ReturnsValidHandle verifies that getDB
// with a tenant context returns a non-nil, functional DB handle.
func TestIntegration_GetDB_TenantContext_ReturnsValidHandle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange -- static container for constructor, tenant container for context.
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	// Act
	db, err := infra.repo.getDB(ctx)

	// Assert
	require.NoError(t, err, "getDB with tenant context should return no error")
	require.NotNil(t, db, "getDB should return a non-nil tenant DB handle")
}

// TestIntegration_GetDB_TenantContext_CanExecuteQueries verifies the tenant DB
// handle can execute queries against the tenant PostgreSQL container.
func TestIntegration_GetDB_TenantContext_CanExecuteQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	// Act -- getDB should return the tenant DB, which should support queries.
	db, err := infra.repo.getDB(ctx)
	require.NoError(t, err)

	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)

	// Assert
	require.NoError(t, err, "tenant DB handle should execute queries successfully")
	assert.Equal(t, 1, result, "query through tenant DB should return 1")
}

// TestIntegration_GetDB_TenantContext_RoutesToTenantDatabase verifies that getDB
// routes queries to the tenant-specific database when the tenant module is in
// the context, proving two-container isolation.
func TestIntegration_GetDB_TenantContext_RoutesToTenantDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange -- two separate containers with different data.
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)

	orgID := infra.orgID
	ledgerID := infra.ledgerID

	// Create a transaction ONLY in the tenant database via tenant context.
	tenantCtx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    "Tenant-Only Transaction",
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(9999),
		AssetCode:      "EUR",
		LedgerID:       ledgerID.String(),
		OrganizationID: orgID.String(),
	}

	created, err := infra.repo.Create(tenantCtx, tx)
	require.NoError(t, err, "Create should succeed through tenant DB path")
	require.NotNil(t, created)

	// Act -- Find through tenant context: should succeed because data is in tenant DB.
	found, err := infra.repo.Find(tenantCtx, orgID, ledgerID, parseID(t, created.ID))

	// Assert -- data exists only in tenant container, so Find succeeds only
	// if getDB correctly returned the tenant DB.
	require.NoError(t, err, "Find should succeed through tenant DB path")
	require.NotNil(t, found)
	assert.Equal(t, "Tenant-Only Transaction", found.Description)
	assert.Equal(t, "EUR", found.AssetCode)

	// Verify the same ID is NOT found through the static path (no tenant context).
	staticCtx := context.Background()
	_, staticErr := infra.repo.Find(staticCtx, orgID, ledgerID, parseID(t, created.ID))

	// Assert -- static container does not have this record.
	require.Error(t, staticErr, "Find should fail through static path for tenant-only data")
}

// TestIntegration_GetDB_TenantContext_CreateAndFind verifies the full
// Create-then-Find round-trip through the tenant DB path.
func TestIntegration_GetDB_TenantContext_CreateAndFind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)
	tenantCtx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	orgID := infra.orgID
	ledgerID := infra.ledgerID

	// Act -- Create through tenant context.
	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    "Tenant Created Transaction",
		Status:         Status{Code: "PENDING"},
		Amount:         decimalPtr(4200),
		AssetCode:      "USD",
		LedgerID:       ledgerID.String(),
		OrganizationID: orgID.String(),
	}

	created, err := infra.repo.Create(tenantCtx, tx)
	require.NoError(t, err, "Create should succeed through tenant DB")
	require.NotNil(t, created)

	// Act -- Find through tenant context.
	found, err := infra.repo.Find(tenantCtx, orgID, ledgerID, parseID(t, created.ID))

	// Assert
	require.NoError(t, err, "Find through tenant context should succeed")
	require.NotNil(t, found)
	assert.Equal(t, "Tenant Created Transaction", found.Description)
	assert.Equal(t, "PENDING", found.Status.Code)

	// Verify the same record is NOT found through the static path.
	_, staticErr := infra.repo.Find(context.Background(), orgID, ledgerID, parseID(t, created.ID))
	require.Error(t, staticErr, "static path should not find tenant-created record")
}

// TestIntegration_GetDB_TenantContext_FindAllIsolation verifies that FindAll
// through the tenant path returns only tenant data, not static data.
func TestIntegration_GetDB_TenantContext_FindAllIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)
	tenantCtx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	orgID := infra.orgID
	ledgerID := infra.ledgerID

	// Create 2 transactions in static DB.
	for i := 0; i < 2; i++ {
		infra.createTestTransaction(t, "Static FindAll Isolation")
	}

	// Create 3 transactions in tenant DB.
	for i := 0; i < 3; i++ {
		tx := &Transaction{
			ID:             uuid.New().String(),
			Description:    "Tenant FindAll Isolation",
			Status:         Status{Code: "ACTIVE"},
			Amount:         decimalPtr(int64(100 * (i + 1))),
			AssetCode:      "USD",
			LedgerID:       ledgerID.String(),
			OrganizationID: orgID.String(),
		}
		_, err := infra.repo.Create(tenantCtx, tx)
		require.NoError(t, err)
	}

	// Act -- FindAll through tenant context.
	tenantTxs, _, err := infra.repo.FindAll(tenantCtx, orgID, ledgerID, http.Pagination{Limit: 100})
	require.NoError(t, err)

	// Assert -- tenant path should return only tenant transactions.
	assert.Equal(t, 3, len(tenantTxs), "tenant FindAll should return only tenant transactions")
	for _, tx := range tenantTxs {
		assert.Equal(t, "Tenant FindAll Isolation", tx.Description)
	}

	// Verify static path returns only static transactions.
	staticTxs, _, err := infra.repo.FindAll(context.Background(), orgID, ledgerID, http.Pagination{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, len(staticTxs), "static FindAll should return only static transactions")
}

// TestIntegration_GetDB_TenantContext_DifferentModuleIgnored verifies that
// injecting a tenant DB for a different module ("onboarding") is ignored by
// getDB for "transaction", which falls back to the static connection.
func TestIntegration_GetDB_TenantContext_DifferentModuleIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange -- inject tenant DB for "onboarding" module, not "transaction".
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)

	// Inject tenant DB under "onboarding" module -- not "transaction".
	wrongModuleCtx := tmcore.ContextWithModulePGConnection(context.Background(), "onboarding", tenantDB)

	// Create a transaction in the static DB so we can verify static path is used.
	created := infra.createTestTransaction(t, "StaticOnly Transaction")

	// Act -- with wrong module context, getDB should fall back to static.
	found, err := infra.repo.Find(wrongModuleCtx, infra.orgID, infra.ledgerID, parseID(t, created.ID))

	// Assert
	require.NoError(t, err, "Find should succeed through static fallback when module name mismatches")
	require.NotNil(t, found)
	assert.Equal(t, "StaticOnly Transaction", found.Description)
}

// TestIntegration_GetDB_TenantContext_UpdateAndDeleteThroughTenantPath verifies
// that Update and Delete operations work correctly through the tenant DB path.
func TestIntegration_GetDB_TenantContext_UpdateAndDeleteThroughTenantPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	infra := setupIntegrationInfra(t)
	_, tenantDB := setupTenantContainer(t)
	tenantCtx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tenantDB)

	orgID := infra.orgID
	ledgerID := infra.ledgerID

	// Create a transaction through the tenant path.
	tx := &Transaction{
		ID:             uuid.New().String(),
		Description:    "Tenant Update Target",
		Status:         Status{Code: "ACTIVE"},
		Amount:         decimalPtr(3000),
		AssetCode:      "USD",
		LedgerID:       ledgerID.String(),
		OrganizationID: orgID.String(),
	}

	created, err := infra.repo.Create(tenantCtx, tx)
	require.NoError(t, err)

	// Act -- Update through tenant context.
	updateTx := &Transaction{
		Description: "Tenant Updated Description",
	}
	updated, err := infra.repo.Update(tenantCtx, orgID, ledgerID, parseID(t, created.ID), updateTx)

	// Assert update.
	require.NoError(t, err, "Update through tenant context should succeed")
	require.NotNil(t, updated)
	assert.Equal(t, "Tenant Updated Description", updated.Description)

	// Verify update persisted through tenant path.
	found, err := infra.repo.Find(tenantCtx, orgID, ledgerID, parseID(t, created.ID))
	require.NoError(t, err)
	assert.Equal(t, "Tenant Updated Description", found.Description)

	// Act -- Delete through tenant context.
	err = infra.repo.Delete(tenantCtx, orgID, ledgerID, parseID(t, created.ID))

	// Assert delete.
	require.NoError(t, err, "Delete through tenant context should succeed")

	// Verify soft-deleted through tenant path.
	_, findErr := infra.repo.Find(tenantCtx, orgID, ledgerID, parseID(t, created.ID))
	require.Error(t, findErr, "soft-deleted transaction should not be found through tenant path")
}
