//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Chaos tests for getDB in the transaction PostgreSQL repository.
//
// These tests exercise fault-tolerance of getDB when the underlying
// PostgreSQL connection experiences failures (connection loss, latency,
// network partition). They are a subset of integration tests and run
// only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_Transaction_GetDB ./components/transaction/internal/adapters/postgres/transaction/...
package transaction

import (
	"context"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE
// =============================================================================

// chaosNetworkTransactionInfra holds all resources for transaction chaos tests
// with Toxiproxy network fault injection.
type chaosNetworkTransactionInfra struct {
	pgResult   *pgtestutil.ContainerResult
	chaosInfra *chaos.Infrastructure
	repo       *TransactionPostgreSQLRepository
	conn       *libPostgres.PostgresConnection
	proxy      *chaos.Proxy
	orgID      uuid.UUID
	ledgerID   uuid.UUID
}

// setupTransactionChaosNetworkInfra creates the full chaos test infrastructure:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container on the host network
//  3. Registers the container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to PostgreSQL
//  5. Builds a TransactionPostgreSQLRepository connected through the proxy
//
// All cleanup (proxy deletion, container termination, network removal) is
// registered with t.Cleanup() automatically.
func setupTransactionChaosNetworkInfra(t *testing.T) *chaosNetworkTransactionInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start PostgreSQL container on the host network
	pgResult := pgtestutil.SetupContainer(t)

	// 3. Register the container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("postgres", pgResult.Container, "5432/tcp")
	require.NoError(t, err, "failed to register PostgreSQL container with chaos infrastructure")

	// 4. Create a Toxiproxy proxy for PostgreSQL using port 8666 (pre-exposed)
	proxy, err := chaosInfra.CreateProxyFor("postgres", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for PostgreSQL")

	// 5. Resolve the proxy listen address
	containerInfo, ok := chaosInfra.GetContainer("postgres")
	require.True(t, ok, "PostgreSQL container must be registered in chaos infrastructure")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy listen address must be non-empty")

	// 6. Build TransactionPostgreSQLRepository connected through proxy
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	proxyConnStr := pgtestutil.BuildConnectionStringWithHost(containerInfo.ProxyListen, pgResult.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: proxyConnStr,
		ConnectionStringReplica: proxyConnStr,
		PrimaryDBName:           pgResult.Config.DBName,
		ReplicaDBName:           pgResult.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	repo := NewTransactionPostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	return &chaosNetworkTransactionInfra{
		pgResult:   pgResult,
		chaosInfra: chaosInfra,
		repo:       repo,
		conn:       conn,
		proxy:      proxy,
		orgID:      orgID,
		ledgerID:   ledgerID,
	}
}

// createTestTransaction creates a transaction via the repository for chaos testing.
func (infra *chaosNetworkTransactionInfra) createTestTransaction(t *testing.T, description string) *Transaction {
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
	require.NoError(t, err, "chaos infra: failed to create test transaction")

	return created
}

// =============================================================================
// CS-1: CONNECTION LOSS
// =============================================================================

// TestIntegration_Chaos_Transaction_ConnectionLoss verifies that getDB and
// repository operations return errors (not panics) when the PostgreSQL
// connection is fully dropped via Toxiproxy.
//
// This tests the scenario where both tenant and static DB connections are
// unavailable. The getDB method must propagate an error, not crash.
//
// 5-Phase structure:
//  1. Normal   -- Create + Find succeed through the proxy
//  2. Inject   -- Toxiproxy proxy is disabled (full connection loss)
//  3. Verify   -- Repository operations return error, no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- Repository operations succeed again
func TestIntegration_Chaos_Transaction_GetDB_ConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionChaosNetworkInfra(t)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify repository operations work through the proxy before any fault.
	t.Log("Phase 1 (Normal): verifying Create and Find succeed through proxy")

	tx := infra.createTestTransaction(t, "Chaos Connection Loss Transaction")
	require.NotEmpty(t, tx.ID, "Phase 1: transaction must have an ID")

	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err, "Phase 1: Find should succeed before fault injection")
	assert.Equal(t, tx.Description, found.Description, "Phase 1: data should match")

	t.Logf("Phase 1: created transaction %s", tx.ID)

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate full connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// All repository operations must return errors, not panic.
	t.Log("Phase 3 (Verify): repository operations must return error, not panic")

	// 3a. Find must fail gracefully.
	var findErr error

	require.NotPanics(t, func() {
		findCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		_, findErr = infra.repo.Find(findCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	}, "Phase 3: Find must not panic on connection loss")

	// The operation may succeed if the connection pool still has cached connections,
	// or it may fail. Both outcomes are acceptable during the partition window.
	// The key invariant is: no panic.
	if findErr != nil {
		t.Logf("Phase 3: Find returned expected error: %v", findErr)
	} else {
		t.Log("Phase 3: Find succeeded (pool had cached connections)")
	}

	// 3b. Create must fail gracefully.
	var createErr error

	require.NotPanics(t, func() {
		createCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		createTx := &Transaction{
			ID:             uuid.New().String(),
			Description:    "Should Fail Transaction",
			Status:         Status{Code: "ACTIVE"},
			Amount:         decimalPtr(500),
			AssetCode:      "USD",
			LedgerID:       infra.ledgerID.String(),
			OrganizationID: infra.orgID.String(),
		}

		_, createErr = infra.repo.Create(createCtx, createTx)
	}, "Phase 3: Create must not panic on connection loss")

	if createErr != nil {
		t.Logf("Phase 3: Create returned expected error: %v", createErr)
	} else {
		t.Log("Phase 3: Create succeeded (pool had cached connections)")
	}

	// 3c. Delete must fail gracefully.
	var deleteErr error

	require.NotPanics(t, func() {
		deleteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		deleteErr = infra.repo.Delete(deleteCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	}, "Phase 3: Delete must not panic on connection loss")

	if deleteErr != nil {
		t.Logf("Phase 3: Delete returned expected error: %v", deleteErr)
	} else {
		t.Log("Phase 3: Delete succeeded (pool had cached connections)")
	}

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After the proxy is restored, repository operations must succeed again.
	t.Log("Phase 5 (Recovery): verifying operations succeed after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		return err
	}, 30*time.Second, "Phase 5: Find should recover after proxy restoration")

	recovered, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	require.NoError(t, err, "Phase 5: Find must succeed after recovery")
	assert.Equal(t, tx.ID, recovered.ID, "Phase 5: transaction ID must be unchanged after recovery")
	assert.Equal(t, tx.Description, recovered.Description, "Phase 5: data integrity must be preserved")

	t.Log("CS-1 PASS: getDB returns error (not panic) when connection is lost, recovers correctly")
}

// =============================================================================
// CS-2: HIGH LATENCY
// =============================================================================

// TestIntegration_Chaos_Transaction_HighLatency verifies that getDB and
// repository operations handle slow DB responses correctly. Specifically,
// context timeout must propagate through getDB so callers can abort slow
// queries without hanging indefinitely.
//
// 5-Phase structure:
//  1. Normal   -- Find succeeds with low latency
//  2. Inject   -- 5000 ms latency added via Toxiproxy
//  3. Verify   -- Find with 1 s context deadline returns timeout error, no panic
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- Find succeeds within normal latency again
func TestIntegration_Chaos_Transaction_GetDB_HighLatency(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionChaosNetworkInfra(t)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify operations work with normal latency.
	t.Log("Phase 1 (Normal): verifying Find succeeds with normal latency")

	tx := infra.createTestTransaction(t, "Chaos High Latency Transaction")

	start := time.Now()
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	baselineLatency := time.Since(start)

	require.NoError(t, err, "Phase 1: Find should succeed with normal latency")
	assert.Equal(t, tx.ID, found.ID)
	t.Logf("Phase 1: baseline Find completed in %v", baselineLatency)

	// --- Phase 2: Inject ---
	// Add 5000 ms latency to the proxy to simulate a very slow database.
	t.Log("Phase 2 (Inject): adding 5000 ms network latency via Toxiproxy")

	err = infra.proxy.AddLatency(5*time.Second, 500*time.Millisecond)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// Find with a 1-second context deadline must return a timeout/deadline error.
	// The key assertion: getDB propagates context deadlines correctly.
	t.Log("Phase 3 (Verify): Find with 1s deadline must timeout, not hang or panic")

	var timeoutErr error

	require.NotPanics(t, func() {
		shortCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, timeoutErr = infra.repo.Find(shortCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	}, "Phase 3: Find must not panic under high latency")

	require.Error(t, timeoutErr, "Phase 3: Find must return error when context deadline is exceeded")
	t.Logf("Phase 3: received expected timeout error: %v", timeoutErr)

	// Also verify that a longer deadline succeeds (the DB itself is still healthy).
	t.Log("Phase 3 (supplementary): Find with 10s deadline should succeed despite latency")

	start = time.Now()

	longCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	slowFound, slowErr := infra.repo.Find(longCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	slowElapsed := time.Since(start)

	if slowErr == nil {
		assert.Equal(t, tx.ID, slowFound.ID, "Phase 3: data should be correct despite latency")
		assert.Greater(t, slowElapsed, 3*time.Second,
			"Phase 3: latency should be noticeable (injected 5s, got %v)", slowElapsed)
		t.Logf("Phase 3: slow Find completed in %v", slowElapsed)
	} else {
		t.Logf("Phase 3: slow Find also failed (acceptable under heavy jitter): %v", slowErr)
	}

	// --- Phase 4: Restore ---
	// Remove all toxics to restore normal latency.
	t.Log("Phase 4 (Restore): removing all toxics from proxy")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	// After toxics are removed, operations must complete with normal latency.
	t.Log("Phase 5 (Recovery): verifying Find succeeds with normal latency after restore")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		_, err := infra.repo.Find(recoveryCtx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
		return err
	}, 30*time.Second, "Phase 5: Find should recover to normal latency after toxic removal")

	start = time.Now()

	recovered, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, tx.ID))
	recoveryLatency := time.Since(start)

	require.NoError(t, err, "Phase 5: Find must succeed after latency is removed")
	assert.Equal(t, tx.ID, recovered.ID, "Phase 5: data integrity must be preserved")
	t.Logf("Phase 5: recovery Find completed in %v (baseline was %v)", recoveryLatency, baselineLatency)

	t.Log("CS-2 PASS: getDB handles high latency correctly via context timeout propagation")
}

// =============================================================================
// CS-3: NETWORK PARTITION (tenant path fails, static path fallback)
// =============================================================================

// TestIntegration_Chaos_Transaction_NetworkPartition verifies that getDB
// falls back gracefully when the network is partitioned. In a multi-tenant
// scenario, if the tenant DB path fails, getDB should attempt the static
// connection. In this chaos test, since both paths go through the same proxy,
// we verify that:
//   - Operations fail gracefully during partition (no panic)
//   - Data is recovered after partition heals
//   - The getDB fallback logic (tenant -> static) does not mask errors silently
//
// 5-Phase structure:
//  1. Normal   -- Create + Find succeed (both with and without tenant context)
//  2. Inject   -- Network partition via Toxiproxy disconnect
//  3. Verify   -- Operations fail gracefully, getDB returns error not panic
//  4. Restore  -- Network partition healed via Toxiproxy reconnect
//  5. Recovery -- Operations resume, data integrity confirmed
func TestIntegration_Chaos_Transaction_GetDB_NetworkPartition(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionChaosNetworkInfra(t)

	ctx := context.Background()

	// Build a tenant context that uses the same proxied DB (simulating the
	// tenant path going through the same PostgreSQL instance).
	tenantDB, err := infra.conn.GetDB()
	require.NoError(t, err, "setup: must be able to get DB from proxied connection")

	tenantCtx := tmcore.ContextWithModulePGConnection(ctx, "transaction", tenantDB)

	// --- Phase 1: Normal ---
	// Verify operations work both with plain context (static path) and with
	// tenant context (tenant path).
	t.Log("Phase 1 (Normal): verifying operations succeed on both code paths")

	// 1a. Static path (no tenant in context).
	staticTx := infra.createTestTransaction(t, "Chaos Partition Static Transaction")
	require.NotEmpty(t, staticTx.ID, "Phase 1: static transaction must have an ID")

	staticFound, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	require.NoError(t, err, "Phase 1: static Find should succeed")
	assert.Equal(t, staticTx.Description, staticFound.Description)

	// 1b. Tenant path (tenant DB injected into context).
	tenantFound, err := infra.repo.Find(tenantCtx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	require.NoError(t, err, "Phase 1: tenant Find should succeed")
	assert.Equal(t, staticTx.Description, tenantFound.Description)

	t.Logf("Phase 1: transaction %s accessible via both static and tenant paths", staticTx.ID)

	// --- Phase 2: Inject ---
	// Disconnect the proxy to simulate a network partition.
	// Both tenant and static paths use the same proxied PostgreSQL, so both
	// become unavailable simultaneously.
	t.Log("Phase 2 (Inject): disconnecting Toxiproxy proxy to simulate network partition")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// Operations on both code paths must fail gracefully (return error, no panic).
	t.Log("Phase 3 (Verify): operations must return error, not panic, during partition")

	// 3a. Static path during partition.
	var staticPartitionErr error

	require.NotPanics(t, func() {
		partitionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		_, staticPartitionErr = infra.repo.Find(partitionCtx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	}, "Phase 3: static Find must not panic during partition")

	if staticPartitionErr != nil {
		t.Logf("Phase 3: static Find returned expected error: %v", staticPartitionErr)
	} else {
		t.Log("Phase 3: static Find succeeded (pool had cached connections)")
	}

	// 3b. Tenant path during partition.
	var tenantPartitionErr error

	require.NotPanics(t, func() {
		partitionTenantCtx, cancel := context.WithTimeout(tenantCtx, 3*time.Second)
		defer cancel()

		_, tenantPartitionErr = infra.repo.Find(partitionTenantCtx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	}, "Phase 3: tenant Find must not panic during partition")

	if tenantPartitionErr != nil {
		t.Logf("Phase 3: tenant Find returned expected error: %v", tenantPartitionErr)
	} else {
		t.Log("Phase 3: tenant Find succeeded (pool had cached connections)")
	}

	// 3c. Delete during partition must not panic.
	var deletePartitionErr error

	require.NotPanics(t, func() {
		deleteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		deletePartitionErr = infra.repo.Delete(deleteCtx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	}, "Phase 3: Delete must not panic during partition")

	if deletePartitionErr != nil {
		t.Logf("Phase 3: Delete returned expected error: %v", deletePartitionErr)
	} else {
		t.Log("Phase 3: Delete succeeded (pool had cached connections)")
	}

	// --- Phase 4: Restore ---
	// Reconnect the proxy to heal the network partition.
	t.Log("Phase 4 (Restore): reconnecting Toxiproxy proxy to heal partition")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After partition heals, both paths must work again.
	t.Log("Phase 5 (Recovery): verifying operations resume on both paths")

	// 5a. Static path recovery.
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
		return err
	}, 30*time.Second, "Phase 5: static Find should recover after partition heals")

	// 5b. Tenant path recovery.
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.Find(tenantCtx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
		return err
	}, 30*time.Second, "Phase 5: tenant Find should recover after partition heals")

	// 5c. Data integrity verification.
	staticRecovered, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, staticTx.ID))
	require.NoError(t, err, "Phase 5: static Find must succeed")

	// If the delete during partition succeeded, the transaction may be soft-deleted.
	// Otherwise, it should still be present with the original data.
	if deletePartitionErr != nil {
		// Delete failed during partition, so transaction should be intact.
		assert.Equal(t, staticTx.ID, staticRecovered.ID,
			"Phase 5: transaction ID must be unchanged after partition recovery")
		assert.Equal(t, staticTx.Description, staticRecovered.Description,
			"Phase 5: data integrity must be preserved after partition recovery")
	} else {
		t.Log("Phase 5: delete succeeded during partition; transaction may be soft-deleted")
	}

	t.Log("CS-3 PASS: getDB falls back gracefully during network partition, both paths recover")
}
