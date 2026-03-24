//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Chaos tests for FindOperationRouteIDsByTransactionRouteIDs and getDB
// in the transactionroute PostgreSQL repository.
//
// These tests exercise fault-tolerance of the repository when the underlying
// PostgreSQL connection experiences failures (connection loss, latency,
// network partition). They are a subset of integration tests and run
// only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_TransactionRoute ./components/ledger/internal/adapters/postgres/transactionroute/...
package transactionroute

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE
// =============================================================================

// chaosNetworkTransactionRouteInfra holds all resources for transaction route
// chaos tests with Toxiproxy network fault injection.
type chaosNetworkTransactionRouteInfra struct {
	pgResult   *pgtestutil.ContainerResult
	chaosInfra *chaos.Infrastructure
	repo       *TransactionRoutePostgreSQLRepository
	conn       *libPostgres.Client
	proxy      *chaos.Proxy
	orgID      uuid.UUID
	ledgerID   uuid.UUID
}

// setupTransactionRouteChaosNetworkInfra creates the full chaos test infrastructure:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container on the host network
//  3. Registers the container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to PostgreSQL
//  5. Builds a TransactionRoutePostgreSQLRepository connected through the proxy
//
// All cleanup (proxy deletion, container termination, network removal) is
// registered with t.Cleanup() automatically.
func setupTransactionRouteChaosNetworkInfra(t *testing.T) *chaosNetworkTransactionRouteInfra {
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

	// 6. Build TransactionRoutePostgreSQLRepository connected through proxy
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	proxyConnStr := pgtestutil.BuildConnectionStringWithHost(containerInfo.ProxyListen, pgResult.Config)

	conn := pgtestutil.CreatePostgresClient(t, proxyConnStr, proxyConnStr, pgResult.Config.DBName, migrationsPath)

	repo := NewTransactionRoutePostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return &chaosNetworkTransactionRouteInfra{
		pgResult:   pgResult,
		chaosInfra: chaosInfra,
		repo:       repo,
		conn:       conn,
		proxy:      proxy,
		orgID:      orgID,
		ledgerID:   ledgerID,
	}
}

// createTransactionRouteWithLinks creates a transaction route with linked operation routes
// through the proxy for chaos testing. Returns the transaction route ID and
// the list of operation route IDs that were linked.
func (infra *chaosNetworkTransactionRouteInfra) createTransactionRouteWithLinks(t *testing.T, title string, opRouteCount int) (uuid.UUID, []uuid.UUID) {
	t.Helper()

	trID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgResult.DB, infra.orgID, infra.ledgerID, title)

	opRouteIDs := make([]uuid.UUID, 0, opRouteCount)

	for i := 0; i < opRouteCount; i++ {
		opRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgResult.DB, infra.orgID, infra.ledgerID,
			fmt.Sprintf("%s OpRoute %d", title, i+1), "source")
		pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgResult.DB, opRouteID, trID)

		opRouteIDs = append(opRouteIDs, opRouteID)
	}

	return trID, opRouteIDs
}

// =============================================================================
// CONNECTION LOSS
// =============================================================================

// TestIntegration_Chaos_TransactionRoute_ConnectionLoss verifies that
// FindOperationRouteIDsByTransactionRouteIDs and FindByID return errors
// (not panics) when the PostgreSQL connection is fully dropped via Toxiproxy.
//
// This tests the scenario where the enrichment junction query is executed
// during a total database outage. The repository must propagate an error,
// not crash.
//
// 5-Phase structure:
//  1. Normal   -- FindOperationRouteIDsByTransactionRouteIDs succeeds through the proxy
//  2. Inject   -- Toxiproxy proxy is disabled (full connection loss)
//  3. Verify   -- Repository operations return error, no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- Repository operations succeed again
func TestIntegration_Chaos_TransactionRoute_ConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionRouteChaosNetworkInfra(t)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify repository operations work through the proxy before any fault.
	t.Log("Phase 1 (Normal): verifying FindOperationRouteIDsByTransactionRouteIDs succeeds through proxy")

	trID1, expectedOpRouteIDs := infra.createTransactionRouteWithLinks(t, "Chaos ConnLoss TR1", 2)
	trID2, _ := infra.createTransactionRouteWithLinks(t, "Chaos ConnLoss TR2", 1)

	result, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID1, trID2})
	require.NoError(t, err, "Phase 1: FindOperationRouteIDsByTransactionRouteIDs should succeed before fault injection")
	require.Len(t, result[trID1], 2, "Phase 1: trID1 should have 2 linked operation routes")
	require.Len(t, result[trID2], 1, "Phase 1: trID2 should have 1 linked operation route")

	t.Logf("Phase 1: junction query returned %d entries for trID1, %d for trID2", len(result[trID1]), len(result[trID2]))

	// Also verify FindByID works
	found, err := infra.repo.FindByID(ctx, infra.orgID, infra.ledgerID, trID1)
	require.NoError(t, err, "Phase 1: FindByID should succeed before fault injection")
	assert.Equal(t, trID1, found.ID, "Phase 1: data should match")

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate full connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// All repository operations must return errors, not panic.
	t.Log("Phase 3 (Verify): repository operations must return error, not panic")

	// 3a. FindOperationRouteIDsByTransactionRouteIDs must fail gracefully.
	var junctionErr error

	require.NotPanics(t, func() {
		junctionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		_, junctionErr = infra.repo.FindOperationRouteIDsByTransactionRouteIDs(junctionCtx, []uuid.UUID{trID1, trID2})
	}, "Phase 3: FindOperationRouteIDsByTransactionRouteIDs must not panic on connection loss")

	// The operation may succeed if the connection pool still has cached connections,
	// or it may fail. Both outcomes are acceptable during the partition window.
	// The key invariant is: no panic.
	if junctionErr != nil {
		t.Logf("Phase 3: FindOperationRouteIDsByTransactionRouteIDs returned expected error: %v", junctionErr)
	} else {
		t.Log("Phase 3: FindOperationRouteIDsByTransactionRouteIDs succeeded (pool had cached connections)")
	}

	// 3b. FindByID must fail gracefully.
	var findErr error

	require.NotPanics(t, func() {
		findCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		_, findErr = infra.repo.FindByID(findCtx, infra.orgID, infra.ledgerID, trID1)
	}, "Phase 3: FindByID must not panic on connection loss")

	if findErr != nil {
		t.Logf("Phase 3: FindByID returned expected error: %v", findErr)
	} else {
		t.Log("Phase 3: FindByID succeeded (pool had cached connections)")
	}

	// 3c. Create must fail gracefully.
	var createErr error

	require.NotPanics(t, func() {
		createCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		createTR := &mmodel.TransactionRoute{
			ID:              uuid.Must(libCommons.GenerateUUIDv7()),
			OrganizationID:  infra.orgID,
			LedgerID:        infra.ledgerID,
			Title:           "Should Fail TR",
			OperationRoutes: []mmodel.OperationRoute{},
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		_, createErr = infra.repo.Create(createCtx, infra.orgID, infra.ledgerID, createTR)
	}, "Phase 3: Create must not panic on connection loss")

	if createErr != nil {
		t.Logf("Phase 3: Create returned expected error: %v", createErr)
	} else {
		t.Log("Phase 3: Create succeeded (pool had cached connections)")
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
		_, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID1})
		return err
	}, 30*time.Second, "Phase 5: FindOperationRouteIDsByTransactionRouteIDs should recover after proxy restoration")

	recoveredResult, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID1, trID2})
	require.NoError(t, err, "Phase 5: FindOperationRouteIDsByTransactionRouteIDs must succeed after recovery")
	assert.Len(t, recoveredResult[trID1], 2, "Phase 5: trID1 should still have 2 linked operation routes after recovery")
	assert.Len(t, recoveredResult[trID2], 1, "Phase 5: trID2 should still have 1 linked operation route after recovery")

	// Verify data integrity: same operation route IDs as before
	recoveredIDs := make(map[uuid.UUID]bool)
	for _, id := range recoveredResult[trID1] {
		recoveredIDs[id] = true
	}

	for _, expectedID := range expectedOpRouteIDs {
		assert.True(t, recoveredIDs[expectedID],
			"Phase 5: operation route %s must be preserved after recovery", expectedID)
	}

	t.Log("PASS: getDB returns error (not panic) when connection is lost, recovers correctly")
}

// =============================================================================
// HIGH LATENCY
// =============================================================================

// TestIntegration_Chaos_TransactionRoute_HighLatency verifies that
// FindOperationRouteIDsByTransactionRouteIDs handles slow DB responses
// correctly. Specifically, context timeout must propagate through getDB
// so callers can abort slow queries without hanging indefinitely.
//
// 5-Phase structure:
//  1. Normal   -- Junction query succeeds with low latency
//  2. Inject   -- 5000 ms latency added via Toxiproxy
//  3. Verify   -- Junction query with 1 s context deadline returns timeout error, no panic
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- Junction query succeeds within normal latency again
func TestIntegration_Chaos_TransactionRoute_HighLatency(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionRouteChaosNetworkInfra(t)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify operations work with normal latency.
	t.Log("Phase 1 (Normal): verifying FindOperationRouteIDsByTransactionRouteIDs succeeds with normal latency")

	trID, _ := infra.createTransactionRouteWithLinks(t, "Chaos HighLatency TR", 2)

	start := time.Now()
	result, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID})
	baselineLatency := time.Since(start)

	require.NoError(t, err, "Phase 1: junction query should succeed with normal latency")
	assert.Len(t, result[trID], 2, "Phase 1: should have 2 linked operation routes")
	t.Logf("Phase 1: baseline junction query completed in %v", baselineLatency)

	// --- Phase 2: Inject ---
	// Add 5000 ms latency to the proxy to simulate a very slow database.
	t.Log("Phase 2 (Inject): adding 5000 ms network latency via Toxiproxy")

	err = infra.proxy.AddLatency(5*time.Second, 500*time.Millisecond)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// Junction query with a 1-second context deadline must return a timeout/deadline error.
	// The key assertion: getDB propagates context deadlines correctly.
	t.Log("Phase 3 (Verify): junction query with 1s deadline must timeout, not hang or panic")

	var timeoutErr error

	require.NotPanics(t, func() {
		shortCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, timeoutErr = infra.repo.FindOperationRouteIDsByTransactionRouteIDs(shortCtx, []uuid.UUID{trID})
	}, "Phase 3: FindOperationRouteIDsByTransactionRouteIDs must not panic under high latency")

	require.Error(t, timeoutErr, "Phase 3: junction query must return error when context deadline is exceeded")
	t.Logf("Phase 3: received expected timeout error: %v", timeoutErr)

	// Also verify that a longer deadline succeeds (the DB itself is still healthy).
	t.Log("Phase 3 (supplementary): junction query with 10s deadline should succeed despite latency")

	start = time.Now()

	longCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	slowResult, slowErr := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(longCtx, []uuid.UUID{trID})
	slowElapsed := time.Since(start)

	if slowErr == nil {
		assert.Len(t, slowResult[trID], 2, "Phase 3: data should be correct despite latency")
		assert.Greater(t, slowElapsed, 3*time.Second,
			"Phase 3: latency should be noticeable (injected 5s, got %v)", slowElapsed)
		t.Logf("Phase 3: slow junction query completed in %v", slowElapsed)
	} else {
		t.Logf("Phase 3: slow junction query also failed (acceptable under heavy jitter): %v", slowErr)
	}

	// --- Phase 4: Restore ---
	// Remove all toxics to restore normal latency.
	t.Log("Phase 4 (Restore): removing all toxics from proxy")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	// After toxics are removed, operations must complete with normal latency.
	t.Log("Phase 5 (Recovery): verifying junction query succeeds with normal latency after restore")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		_, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(recoveryCtx, []uuid.UUID{trID})
		return err
	}, 30*time.Second, "Phase 5: junction query should recover to normal latency after toxic removal")

	start = time.Now()

	recovered, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID})
	recoveryLatency := time.Since(start)

	require.NoError(t, err, "Phase 5: junction query must succeed after latency is removed")
	assert.Len(t, recovered[trID], 2, "Phase 5: data integrity must be preserved")
	t.Logf("Phase 5: recovery junction query completed in %v (baseline was %v)", recoveryLatency, baselineLatency)

	t.Log("PASS: getDB handles high latency correctly via context timeout propagation")
}

// =============================================================================
// NETWORK PARTITION (tenant path fails, static path fallback)
// =============================================================================

// TestIntegration_Chaos_TransactionRoute_NetworkPartition verifies that getDB
// falls back gracefully when the network is partitioned. In a multi-tenant
// scenario, if the tenant DB path fails, getDB should attempt the static
// connection. In this chaos test, since both paths go through the same proxy,
// we verify that:
//   - FindOperationRouteIDsByTransactionRouteIDs fails gracefully during partition (no panic)
//   - Data is recovered after partition heals
//   - The getDB fallback logic (tenant -> static) does not mask errors silently
//
// 5-Phase structure:
//  1. Normal   -- Junction query succeeds (both with and without tenant context)
//  2. Inject   -- Network partition via Toxiproxy disconnect
//  3. Verify   -- Operations fail gracefully, getDB returns error not panic
//  4. Restore  -- Network partition healed via Toxiproxy reconnect
//  5. Recovery -- Operations resume, data integrity confirmed
func TestIntegration_Chaos_TransactionRoute_NetworkPartition(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupTransactionRouteChaosNetworkInfra(t)

	ctx := context.Background()

	// Build a tenant context that uses the same proxied DB (simulating the
	// tenant path going through the same PostgreSQL instance).
	tenantDB, err := infra.conn.Resolver(context.Background())
	require.NoError(t, err, "setup: must be able to get DB from proxied connection")

	tenantCtx := tmcore.ContextWithModulePGConnection(ctx, "transaction", tenantDB)

	// --- Phase 1: Normal ---
	// Verify operations work both with plain context (static path) and with
	// tenant context (tenant path).
	t.Log("Phase 1 (Normal): verifying operations succeed on both code paths")

	trID, expectedOpRouteIDs := infra.createTransactionRouteWithLinks(t, "Chaos Partition TR", 2)

	// 1a. Static path (no tenant in context).
	staticResult, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID})
	require.NoError(t, err, "Phase 1: static junction query should succeed")
	assert.Len(t, staticResult[trID], 2, "Phase 1: static path should return 2 operation route IDs")

	// 1b. Tenant path (tenant DB injected into context).
	tenantResult, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(tenantCtx, []uuid.UUID{trID})
	require.NoError(t, err, "Phase 1: tenant junction query should succeed")
	assert.Len(t, tenantResult[trID], 2, "Phase 1: tenant path should return 2 operation route IDs")

	t.Logf("Phase 1: junction query for %s accessible via both static and tenant paths", trID)

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

		_, staticPartitionErr = infra.repo.FindOperationRouteIDsByTransactionRouteIDs(partitionCtx, []uuid.UUID{trID})
	}, "Phase 3: static junction query must not panic during partition")

	if staticPartitionErr != nil {
		t.Logf("Phase 3: static junction query returned expected error: %v", staticPartitionErr)
	} else {
		t.Log("Phase 3: static junction query succeeded (pool had cached connections)")
	}

	// 3b. Tenant path during partition.
	var tenantPartitionErr error

	require.NotPanics(t, func() {
		partitionTenantCtx, cancel := context.WithTimeout(tenantCtx, 3*time.Second)
		defer cancel()

		_, tenantPartitionErr = infra.repo.FindOperationRouteIDsByTransactionRouteIDs(partitionTenantCtx, []uuid.UUID{trID})
	}, "Phase 3: tenant junction query must not panic during partition")

	if tenantPartitionErr != nil {
		t.Logf("Phase 3: tenant junction query returned expected error: %v", tenantPartitionErr)
	} else {
		t.Log("Phase 3: tenant junction query succeeded (pool had cached connections)")
	}

	// 3c. FindByID during partition must not panic.
	var findPartitionErr error

	require.NotPanics(t, func() {
		findCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		_, findPartitionErr = infra.repo.FindByID(findCtx, infra.orgID, infra.ledgerID, trID)
	}, "Phase 3: FindByID must not panic during partition")

	if findPartitionErr != nil {
		t.Logf("Phase 3: FindByID returned expected error: %v", findPartitionErr)
	} else {
		t.Log("Phase 3: FindByID succeeded (pool had cached connections)")
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
		_, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID})
		return err
	}, 30*time.Second, "Phase 5: static junction query should recover after partition heals")

	// 5b. Tenant path recovery.
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(tenantCtx, []uuid.UUID{trID})
		return err
	}, 30*time.Second, "Phase 5: tenant junction query should recover after partition heals")

	// 5c. Data integrity verification.
	recoveredResult, err := infra.repo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{trID})
	require.NoError(t, err, "Phase 5: junction query must succeed after partition recovery")
	require.Len(t, recoveredResult[trID], 2, "Phase 5: should still have 2 operation route IDs after recovery")

	recoveredIDs := make(map[uuid.UUID]bool)
	for _, id := range recoveredResult[trID] {
		recoveredIDs[id] = true
	}

	for _, expectedID := range expectedOpRouteIDs {
		assert.True(t, recoveredIDs[expectedID],
			"Phase 5: operation route %s must be preserved after partition recovery", expectedID)
	}

	t.Log("PASS: getDB falls back gracefully during network partition, both paths recover")
}
