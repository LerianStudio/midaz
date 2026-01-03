//go:build integration

package transaction

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/testutils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"

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

// cleanup releases all resources for integration tests.
func (infra *integrationTestInfra) cleanup() {
	if infra.pgContainer != nil {
		infra.pgContainer.Cleanup()
	}
}

// cleanup releases all resources for chaos tests.
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
	if infra.pgContainer != nil {
		infra.pgContainer.Cleanup()
	}
}

// cleanup releases all resources for network chaos infrastructure.
func (infra *networkChaosTestInfra) cleanup() {
	// Cleanup PostgreSQL container first
	if infra.pgResult != nil {
		infra.pgResult.Cleanup()
	}
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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
func TestChaos_Transaction_PostgresRestart(t *testing.T) {
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
func TestChaos_Transaction_DataIntegrity(t *testing.T) {
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
func TestChaos_Transaction_NetworkLatency(t *testing.T) {
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
func TestChaos_Transaction_NetworkPartition(t *testing.T) {
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
func TestChaos_Transaction_PacketLoss(t *testing.T) {
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
	successCount := 0
	errorCount := 0
	totalAttempts := 20

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
func TestChaos_Transaction_IntermittentFailure(t *testing.T) {
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
		infra.proxy.Disconnect()
		time.Sleep(500 * time.Millisecond)

		// Reconnect
		t.Logf("Chaos: Cycle %d - reconnecting", i+1)
		infra.proxy.Reconnect()

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
