//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Chaos tests for PostgreSQL bootstrap initialization in the transaction module.
//
// These tests exercise fault-tolerance of initSingleTenantPostgres and
// initMultiTenantPostgres when the underlying PostgreSQL connection experiences
// failures (connection loss). They verify that bootstrap functions return errors
// gracefully (no panic) and that already-initialized repositories degrade
// gracefully when PG goes down after initialization.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_InitPostgres ./components/transaction/internal/bootstrap/...
package bootstrap

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE
// =============================================================================

// chaosBootstrapInfra holds all resources for bootstrap chaos tests with
// Toxiproxy network fault injection.
type chaosBootstrapInfra struct {
	pgResult   *pgtestutil.ContainerResult
	chaosInfra *chaos.Infrastructure
	proxy      *chaos.Proxy
	proxyHost  string // host:port for connecting through the proxy
}

// setupBootstrapChaosInfra creates the full chaos test infrastructure:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container
//  3. Registers the container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to PostgreSQL
//
// All cleanup is registered with t.Cleanup() automatically.
func setupBootstrapChaosInfra(t *testing.T) *chaosBootstrapInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start PostgreSQL container
	pgResult := pgtestutil.SetupContainer(t)

	// 3. Register the container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("postgres", pgResult.Container, "5432/tcp")
	require.NoError(t, err, "failed to register PostgreSQL container with chaos infrastructure")

	// 4. Create a Toxiproxy proxy for PostgreSQL
	proxy, err := chaosInfra.CreateProxyFor("postgres", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for PostgreSQL")

	// 5. Resolve the proxy listen address
	containerInfo, ok := chaosInfra.GetContainer("postgres")
	require.True(t, ok, "PostgreSQL container must be registered in chaos infrastructure")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy listen address must be non-empty")

	return &chaosBootstrapInfra{
		pgResult:   pgResult,
		chaosInfra: chaosInfra,
		proxy:      proxy,
		proxyHost:  containerInfo.ProxyListen,
	}
}

// buildProxiedConfig creates a Config that routes PG traffic through the
// Toxiproxy proxy so faults can be injected.
func (infra *chaosBootstrapInfra) buildProxiedConfig(t *testing.T) *Config {
	t.Helper()

	host, portStr, err := net.SplitHostPort(infra.proxyHost)
	require.NoError(t, err, "failed to split host:port")

	_, err = strconv.Atoi(portStr)
	require.NoError(t, err, "failed to parse port")

	return &Config{
		PrimaryDBHost:     host,
		PrimaryDBUser:     infra.pgResult.Config.DBUser,
		PrimaryDBPassword: infra.pgResult.Config.DBPassword,
		PrimaryDBName:     infra.pgResult.Config.DBName,
		PrimaryDBPort:     portStr,
		PrimaryDBSSLMode:  "disable",
		ReplicaDBHost:     host,
		ReplicaDBUser:     infra.pgResult.Config.DBUser,
		ReplicaDBPassword: infra.pgResult.Config.DBPassword,
		ReplicaDBName:     infra.pgResult.Config.DBName,
		ReplicaDBPort:     portStr,
		ReplicaDBSSLMode:  "disable",
	}
}

// =============================================================================
// CS-1: initSingleTenantPostgres when PG connection fails
// =============================================================================

// TestIntegration_Chaos_InitPostgres_SingleTenantConnectionLoss verifies that
// initSingleTenantPostgres returns an error (not a panic) when the PostgreSQL
// connection is unavailable due to a network fault.
//
// 5-Phase structure:
//  1. Normal   -- initSingleTenantPostgres succeeds through the proxy
//  2. Inject   -- Toxiproxy proxy is disabled (connection loss)
//  3. Verify   -- initSingleTenantPostgres returns error, no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- initSingleTenantPostgres succeeds again
func TestIntegration_Chaos_InitPostgres_SingleTenantConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupBootstrapChaosInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildProxiedConfig(t)

	// Use the real connector so connections go through the proxy.
	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	// --- Phase 1: Normal ---
	// Verify initSingleTenantPostgres succeeds through the proxy.
	t.Log("Phase 1 (Normal): verifying initSingleTenantPostgres succeeds through proxy")

	result, err := initSingleTenantPostgres(cfg, logger)
	require.NoError(t, err, "Phase 1: initSingleTenantPostgres must succeed through proxy")
	require.NotNil(t, result)
	assert.True(t, result.connection.Connected, "Phase 1: connection must be connected")

	// Verify a query works.
	db, dbErr := result.connection.GetDB()
	require.NoError(t, dbErr, "Phase 1: GetDB must succeed")

	var pingResult int
	err = db.QueryRowContext(context.Background(), "SELECT 1").Scan(&pingResult)
	require.NoError(t, err, "Phase 1: SELECT 1 must succeed through proxy")

	t.Log("Phase 1: initSingleTenantPostgres succeeded, connection is live")

	// Close the Phase 1 connection to avoid resource leaks.
	closePGConnection(result.connection)

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// initSingleTenantPostgres must return an error, not panic.
	t.Log("Phase 3 (Verify): initSingleTenantPostgres must return error, not panic")

	var initErr error

	require.NotPanics(t, func() {
		var failResult *postgresComponents
		failResult, initErr = initSingleTenantPostgres(cfg, logger)
		if failResult != nil {
			closePGConnection(failResult.connection)
		}
	}, "Phase 3: initSingleTenantPostgres must not panic on connection loss")

	require.Error(t, initErr, "Phase 3: initSingleTenantPostgres must return error when PG is unreachable")
	assert.Contains(t, initErr.Error(), "failed to connect to PostgreSQL (single-tenant)",
		"Phase 3: error must contain descriptive context")

	t.Logf("Phase 3: received expected error: %v", initErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After proxy restoration, initSingleTenantPostgres must succeed again.
	t.Log("Phase 5 (Recovery): verifying initSingleTenantPostgres succeeds after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryResult, recoveryErr := initSingleTenantPostgres(cfg, logger)
		if recoveryResult != nil {
			closePGConnection(recoveryResult.connection)
		}

		return recoveryErr
	}, 30*time.Second, "Phase 5: initSingleTenantPostgres should recover after proxy restoration")

	recoveredResult, err := initSingleTenantPostgres(cfg, logger)
	require.NoError(t, err, "Phase 5: initSingleTenantPostgres must succeed after recovery")
	require.NotNil(t, recoveredResult)

	t.Cleanup(func() { closePGConnection(recoveredResult.connection) })

	assert.True(t, recoveredResult.connection.Connected,
		"Phase 5: connection must be connected after recovery")

	t.Log("CS-1 PASS: initSingleTenantPostgres returns error (not panic) on connection loss, recovers correctly")
}

// =============================================================================
// CS-2: initMultiTenantPostgres when PG connection fails
// =============================================================================

// TestIntegration_Chaos_InitPostgres_MultiTenantConnectionLoss verifies that
// initMultiTenantPostgres returns an error (not a panic) when the PostgreSQL
// connection is unavailable due to a network fault.
//
// 5-Phase structure:
//  1. Normal   -- initMultiTenantPostgres succeeds through the proxy
//  2. Inject   -- Toxiproxy proxy is disabled (connection loss)
//  3. Verify   -- initMultiTenantPostgres returns error, no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- initMultiTenantPostgres succeeds again
func TestIntegration_Chaos_InitPostgres_MultiTenantConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupBootstrapChaosInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildProxiedConfig(t)

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	mockClient := tmclient.NewClient("http://localhost:0", logger)
	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       mockClient,
		TenantServiceName:  "transaction",
	}

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying initMultiTenantPostgres succeeds through proxy")

	result, err := initMultiTenantPostgres(opts, cfg, logger)
	require.NoError(t, err, "Phase 1: initMultiTenantPostgres must succeed through proxy")
	require.NotNil(t, result)
	assert.NotNil(t, result.pgManager, "Phase 1: pgManager must be set")
	assert.True(t, result.connection.Connected, "Phase 1: connection must be connected")

	db, dbErr := result.connection.GetDB()
	require.NoError(t, dbErr, "Phase 1: GetDB must succeed")

	var pingResult int
	err = db.QueryRowContext(context.Background(), "SELECT 1").Scan(&pingResult)
	require.NoError(t, err, "Phase 1: SELECT 1 must succeed through proxy")

	t.Log("Phase 1: initMultiTenantPostgres succeeded, connection is live")
	closePGConnection(result.connection)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): initMultiTenantPostgres must return error, not panic")

	var initErr error

	require.NotPanics(t, func() {
		var failResult *postgresComponents
		failResult, initErr = initMultiTenantPostgres(opts, cfg, logger)
		if failResult != nil {
			closePGConnection(failResult.connection)
		}
	}, "Phase 3: initMultiTenantPostgres must not panic on connection loss")

	require.Error(t, initErr, "Phase 3: initMultiTenantPostgres must return error when PG is unreachable")
	assert.Contains(t, initErr.Error(), "failed to connect to PostgreSQL (multi-tenant)",
		"Phase 3: error must contain descriptive context")

	t.Logf("Phase 3: received expected error: %v", initErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying initMultiTenantPostgres succeeds after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryResult, recoveryErr := initMultiTenantPostgres(opts, cfg, logger)
		if recoveryResult != nil {
			closePGConnection(recoveryResult.connection)
		}

		return recoveryErr
	}, 30*time.Second, "Phase 5: initMultiTenantPostgres should recover after proxy restoration")

	recoveredResult, err := initMultiTenantPostgres(opts, cfg, logger)
	require.NoError(t, err, "Phase 5: initMultiTenantPostgres must succeed after recovery")
	require.NotNil(t, recoveredResult)

	t.Cleanup(func() { closePGConnection(recoveredResult.connection) })

	assert.True(t, recoveredResult.connection.Connected,
		"Phase 5: connection must be connected after recovery")
	assert.NotNil(t, recoveredResult.pgManager,
		"Phase 5: pgManager must be set after recovery")

	t.Log("CS-2 PASS: initMultiTenantPostgres returns error (not panic) on connection loss, recovers correctly")
}

// =============================================================================
// CS-3: Init succeeds, then PG goes down -- repos degrade gracefully
// =============================================================================

// TestIntegration_Chaos_InitPostgres_PostInitConnectionLoss verifies that
// when initSingleTenantPostgres succeeds and then PostgreSQL goes down,
// subsequent GetDB/query operations on the repos return errors gracefully
// (no panic), and recover once PG comes back.
//
// 5-Phase structure:
//  1. Normal   -- Init succeeds, repos can query
//  2. Inject   -- PG goes down via Toxiproxy disconnect
//  3. Verify   -- GetDB and queries return errors, no panic
//  4. Restore  -- PG comes back via Toxiproxy reconnect
//  5. Recovery -- Repos can query again
func TestIntegration_Chaos_InitPostgres_PostInitConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupBootstrapChaosInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildProxiedConfig(t)

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): initializing single-tenant and verifying queries succeed")

	result, err := initSingleTenantPostgres(cfg, logger)
	require.NoError(t, err, "Phase 1: initSingleTenantPostgres must succeed through proxy")
	require.NotNil(t, result)

	t.Cleanup(func() { closePGConnection(result.connection) })

	db, dbErr := result.connection.GetDB()
	require.NoError(t, dbErr, "Phase 1: GetDB must succeed")

	var pingResult int
	err = db.QueryRowContext(context.Background(), "SELECT 1").Scan(&pingResult)
	require.NoError(t, err, "Phase 1: SELECT 1 must succeed")
	assert.Equal(t, 1, pingResult)

	t.Log("Phase 1: repos initialized and queries succeed")

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy (PG goes down after init)")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// Queries on the already-initialized connection must return errors, not panic.
	t.Log("Phase 3 (Verify): queries on existing connection must return error, not panic")

	// 3a. Query via connection must fail gracefully.
	var queryErr error

	require.NotPanics(t, func() {
		queryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// The existing db handle may still have cached connections in the pool.
		// We attempt a query; it may succeed on a cached conn or fail. Both are OK.
		// The key invariant: no panic.
		var dummy int
		queryErr = db.QueryRowContext(queryCtx, "SELECT 1").Scan(&dummy)
	}, "Phase 3: query via existing connection must not panic")

	if queryErr != nil {
		t.Logf("Phase 3: query returned expected error: %v", queryErr)
	} else {
		t.Log("Phase 3: query succeeded (pool had cached connections)")
	}

	// 3b. Getting a fresh DB handle must not panic.
	require.NotPanics(t, func() {
		freshDB, freshErr := result.connection.GetDB()
		if freshErr != nil {
			t.Logf("Phase 3: GetDB returned error: %v", freshErr)
			return
		}

		freshCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var dummy int
		_ = freshDB.QueryRowContext(freshCtx, "SELECT 1").Scan(&dummy)
	}, "Phase 3: GetDB + query must not panic when PG is down")

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying queries succeed again after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var dummy int
		return db.QueryRowContext(recoveryCtx, "SELECT 1").Scan(&dummy)
	}, 30*time.Second, "Phase 5: query should recover after proxy restoration")

	var recoveredResult int
	err = db.QueryRowContext(context.Background(), "SELECT 1").Scan(&recoveredResult)
	require.NoError(t, err, "Phase 5: query must succeed after recovery")
	assert.Equal(t, 1, recoveredResult, "Phase 5: SELECT 1 must return 1")

	t.Log("CS-3 PASS: repos degrade gracefully when PG goes down post-init, recover correctly")
}
