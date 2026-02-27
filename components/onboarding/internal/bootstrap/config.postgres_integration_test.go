//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Integration tests for PostgreSQL bootstrap initialization in the onboarding module.
//
// These tests verify that initSingleTenantPostgres, initMultiTenantPostgres, and
// buildPostgresConnection produce working components when connected to a real
// PostgreSQL instance via testcontainers.
//
// Run with:
//
//	go test -tags integration -v -run TestIntegration_InitPostgres ./components/onboarding/internal/bootstrap/...
package bootstrap

import (
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// INTEGRATION TEST INFRASTRUCTURE
// =============================================================================

// integrationTestInfra holds resources for bootstrap integration tests with a
// real PostgreSQL container.
type integrationTestInfra struct {
	pgResult *pgtestutil.ContainerResult
}

// setupBootstrapIntegrationInfra creates a PostgreSQL container and returns the
// infrastructure needed for bootstrap integration tests.
func setupBootstrapIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	pgResult := pgtestutil.SetupContainer(t)

	return &integrationTestInfra{
		pgResult: pgResult,
	}
}

// buildTestConfig creates a Config populated with the real container's connection
// details so that buildPostgresConnection and the init* functions connect to the
// test container instead of a production database.
func (infra *integrationTestInfra) buildTestConfig() *Config {
	return &Config{
		PrimaryDBHost:     infra.pgResult.Host,
		PrimaryDBUser:     infra.pgResult.Config.DBUser,
		PrimaryDBPassword: infra.pgResult.Config.DBPassword,
		PrimaryDBName:     infra.pgResult.Config.DBName,
		PrimaryDBPort:     infra.pgResult.Port,
		PrimaryDBSSLMode:  "disable",
		ReplicaDBHost:     infra.pgResult.Host,
		ReplicaDBUser:     infra.pgResult.Config.DBUser,
		ReplicaDBPassword: infra.pgResult.Config.DBPassword,
		ReplicaDBName:     infra.pgResult.Config.DBName,
		ReplicaDBPort:     infra.pgResult.Port,
		ReplicaDBSSLMode:  "disable",
	}
}

// =============================================================================
// IS-1: initSingleTenantPostgres with real PG container
// =============================================================================

// TestIntegration_InitPostgres_SingleTenantProducesWorkingRepos verifies that
// initSingleTenantPostgres with a real PostgreSQL container produces a
// postgresComponents with a connected connection and all repositories initialized.
func TestIntegration_InitPostgres_SingleTenantProducesWorkingRepos(t *testing.T) {
	infra := setupBootstrapIntegrationInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildTestConfig()

	// Restore the default connector so the real PostgreSQL container is used.
	// The unit test file overrides postgresConnector; here we need the real one.
	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	result, err := initSingleTenantPostgres(cfg, logger)
	require.NoError(t, err, "initSingleTenantPostgres must succeed with a real PG container")
	require.NotNil(t, result)

	t.Cleanup(func() { closePGConnection(result.connection) })

	// Verify connection is live.
	assert.NotNil(t, result.connection, "connection must be non-nil")
	assert.True(t, result.connection.Connected, "connection must be marked as connected")
	assert.Nil(t, result.pgManager, "single-tenant must not have a pgManager")

	// Verify all 7 repositories are initialized.
	assert.NotNil(t, result.organizationRepo, "organizationRepo must be initialized")
	assert.NotNil(t, result.ledgerRepo, "ledgerRepo must be initialized")
	assert.NotNil(t, result.accountRepo, "accountRepo must be initialized")
	assert.NotNil(t, result.assetRepo, "assetRepo must be initialized")
	assert.NotNil(t, result.portfolioRepo, "portfolioRepo must be initialized")
	assert.NotNil(t, result.segmentRepo, "segmentRepo must be initialized")
	assert.NotNil(t, result.accountTypeRepo, "accountTypeRepo must be initialized")

	// Verify the connection can actually execute queries (Ping via GetDB).
	db, dbErr := result.connection.GetDB()
	require.NoError(t, dbErr, "GetDB must succeed on a connected connection")

	err = db.PingContext(t.Context())
	assert.NoError(t, err, "Ping via GetDB must succeed against real PG container")
}

// =============================================================================
// IS-2: initMultiTenantPostgres with real PG container + mock tmclient
// =============================================================================

// TestIntegration_InitPostgres_MultiTenantProducesWorkingRepos verifies that
// initMultiTenantPostgres with a real PostgreSQL container and a mock
// TenantClient produces a postgresComponents with pgManager set and all
// repositories initialized.
func TestIntegration_InitPostgres_MultiTenantProducesWorkingRepos(t *testing.T) {
	infra := setupBootstrapIntegrationInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildTestConfig()

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	// Use a mock TenantClient pointing at a non-routable address (it will not
	// be called during init, only stored for later middleware resolution).
	mockClient := tmclient.NewClient("http://localhost:0", logger)

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       mockClient,
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantPostgres(opts, cfg, logger)
	require.NoError(t, err, "initMultiTenantPostgres must succeed with a real PG container")
	require.NotNil(t, result)

	t.Cleanup(func() { closePGConnection(result.connection) })

	// Verify connection is live.
	assert.NotNil(t, result.connection, "connection must be non-nil (placeholder)")
	assert.True(t, result.connection.Connected, "connection must be marked as connected")

	// Verify pgManager is set.
	assert.NotNil(t, result.pgManager, "multi-tenant mode must have a non-nil pgManager")

	// Verify all 7 repositories are initialized.
	assert.NotNil(t, result.organizationRepo, "organizationRepo must be initialized")
	assert.NotNil(t, result.ledgerRepo, "ledgerRepo must be initialized")
	assert.NotNil(t, result.accountRepo, "accountRepo must be initialized")
	assert.NotNil(t, result.assetRepo, "assetRepo must be initialized")
	assert.NotNil(t, result.portfolioRepo, "portfolioRepo must be initialized")
	assert.NotNil(t, result.segmentRepo, "segmentRepo must be initialized")
	assert.NotNil(t, result.accountTypeRepo, "accountTypeRepo must be initialized")

	// Verify the connection can actually execute queries.
	db, dbErr := result.connection.GetDB()
	require.NoError(t, dbErr, "GetDB must succeed on a connected connection")

	err = db.PingContext(t.Context())
	assert.NoError(t, err, "Ping via GetDB must succeed against real PG container")
}

// =============================================================================
// IS-3: buildPostgresConnection with real PG container env vars
// =============================================================================

// TestIntegration_InitPostgres_BuildConnectionProducesConnectable verifies that
// buildPostgresConnection creates a PostgresConnection whose connection string
// matches the real container and can be used to establish a live connection.
func TestIntegration_InitPostgres_BuildConnectionProducesConnectable(t *testing.T) {
	infra := setupBootstrapIntegrationInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildTestConfig()

	conn := buildPostgresConnection(cfg, logger)
	require.NotNil(t, conn, "buildPostgresConnection must return a non-nil connection")

	// Verify the connection string contains the container's host and port.
	assert.Contains(t, conn.ConnectionStringPrimary, infra.pgResult.Host,
		"primary connection string must contain the container host")
	assert.Contains(t, conn.ConnectionStringPrimary, infra.pgResult.Port,
		"primary connection string must contain the container port")
	assert.Contains(t, conn.ConnectionStringReplica, infra.pgResult.Host,
		"replica connection string must contain the container host")

	// Verify connection metadata.
	assert.Equal(t, infra.pgResult.Config.DBName, conn.PrimaryDBName,
		"PrimaryDBName must match the container config")
	assert.Equal(t, ApplicationName, conn.Component,
		"Component must be set to ApplicationName")
	assert.Equal(t, logger, conn.Logger,
		"Logger must be the one provided")

	// Verify the connection can actually connect to the real PG container.
	err := conn.Connect()
	require.NoError(t, err, "Connect must succeed against real PG container")

	t.Cleanup(func() { closePGConnection(conn) })

	assert.True(t, conn.Connected, "connection must be marked as connected after Connect()")

	// Verify a query can be executed.
	db, dbErr := conn.GetDB()
	require.NoError(t, dbErr, "GetDB must succeed after Connect()")

	var result int
	err = db.QueryRowContext(t.Context(), "SELECT 1").Scan(&result)
	require.NoError(t, err, "SELECT 1 must succeed against real PG container")
	assert.Equal(t, 1, result, "SELECT 1 must return 1")
}

// =============================================================================
// IS-4: initPostgres dispatcher routes correctly with real PG
// =============================================================================

// TestIntegration_InitPostgres_DispatcherRoutesCorrectly verifies that the
// top-level initPostgres function correctly routes to single-tenant or
// multi-tenant initialization based on Options, producing working components
// in both cases.
func TestIntegration_InitPostgres_DispatcherRoutesCorrectly(t *testing.T) {
	infra := setupBootstrapIntegrationInfra(t)

	logger := libZap.InitializeLogger()
	cfg := infra.buildTestConfig()

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	// Sub-test 1: nil opts -> single-tenant.
	t.Run("nil opts routes to single-tenant", func(t *testing.T) {
		result, err := initPostgres(nil, cfg, logger)
		require.NoError(t, err, "initPostgres with nil opts must succeed")
		require.NotNil(t, result)

		t.Cleanup(func() { closePGConnection(result.connection) })

		assert.Nil(t, result.pgManager, "nil opts must produce single-tenant (no pgManager)")
		assert.NotNil(t, result.connection, "connection must be non-nil")

		db, dbErr := result.connection.GetDB()
		require.NoError(t, dbErr)

		err = db.PingContext(t.Context())
		assert.NoError(t, err, "Ping must succeed")
	})

	// Sub-test 2: multi-tenant enabled -> multi-tenant.
	t.Run("multi-tenant opts routes to multi-tenant", func(t *testing.T) {
		// Need to reset the connector for the sub-test since the parent
		// cleanup may restore original before this runs.
		prev := postgresConnector
		postgresConnector = defaultPostgresConnector
		t.Cleanup(func() { postgresConnector = prev })

		mockClient := tmclient.NewClient("http://localhost:0", logger)
		opts := &Options{
			MultiTenantEnabled: true,
			TenantClient:       mockClient,
			TenantServiceName:  "onboarding",
		}

		result, err := initPostgres(opts, cfg, logger)
		require.NoError(t, err, "initPostgres with multi-tenant opts must succeed")
		require.NotNil(t, result)

		t.Cleanup(func() { closePGConnection(result.connection) })

		assert.NotNil(t, result.pgManager, "multi-tenant opts must produce pgManager")
		assert.NotNil(t, result.connection, "connection must be non-nil (placeholder)")

		db, dbErr := result.connection.GetDB()
		require.NoError(t, dbErr)

		err = db.PingContext(t.Context())
		assert.NoError(t, err, "Ping must succeed")
	})
}

// =============================================================================
// IS-5: buildPostgresConnection prefixed env var fallback (integration)
// =============================================================================

// TestIntegration_InitPostgres_PrefixedFallbackConnects verifies that when
// prefixed env vars are set, buildPostgresConnection uses them to connect to
// the real PG container. This complements the unit test by proving the
// connection actually works, not just that the string is formatted correctly.
func TestIntegration_InitPostgres_PrefixedFallbackConnects(t *testing.T) {
	infra := setupBootstrapIntegrationInfra(t)

	logger := libZap.InitializeLogger()

	// Use prefixed values pointing at the real container, with fallback values
	// pointing at an invalid host to prove prefixed takes priority.
	cfg := &Config{
		PrefixedPrimaryDBHost:     infra.pgResult.Host,
		PrefixedPrimaryDBUser:     infra.pgResult.Config.DBUser,
		PrefixedPrimaryDBPassword: infra.pgResult.Config.DBPassword,
		PrefixedPrimaryDBName:     infra.pgResult.Config.DBName,
		PrefixedPrimaryDBPort:     infra.pgResult.Port,
		PrefixedPrimaryDBSSLMode:  "disable",
		PrefixedReplicaDBHost:     infra.pgResult.Host,
		PrefixedReplicaDBUser:     infra.pgResult.Config.DBUser,
		PrefixedReplicaDBPassword: infra.pgResult.Config.DBPassword,
		PrefixedReplicaDBName:     infra.pgResult.Config.DBName,
		PrefixedReplicaDBPort:     infra.pgResult.Port,
		PrefixedReplicaDBSSLMode:  "disable",
		// Fallback values pointing at invalid host.
		PrimaryDBHost: "invalid-host-should-not-be-used",
		PrimaryDBPort: "9999",
		ReplicaDBHost: "invalid-host-should-not-be-used",
		ReplicaDBPort: "9999",
	}

	conn := buildPostgresConnection(cfg, logger)
	require.NotNil(t, conn)

	err := conn.Connect()
	require.NoError(t, err, "Connect must succeed using prefixed values against real PG container")

	t.Cleanup(func() { closePGConnection(conn) })

	db, dbErr := conn.GetDB()
	require.NoError(t, dbErr)

	var result int
	err = db.QueryRowContext(t.Context(), "SELECT 1").Scan(&result)
	require.NoError(t, err, "query must succeed using prefixed connection")
	assert.Equal(t, 1, result)
}

// =============================================================================
// IS-6: Connection failure with invalid config (negative/error path)
// =============================================================================

// TestIntegration_InitPostgres_InvalidConfigReturnsError verifies that
// initSingleTenantPostgres returns an error (not a panic) when given a Config
// that points to a non-existent PostgreSQL instance.
func TestIntegration_InitPostgres_InvalidConfigReturnsError(t *testing.T) {
	logger := libZap.InitializeLogger()

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	cfg := &Config{
		PrimaryDBHost:     "invalid-host-that-does-not-exist",
		PrimaryDBUser:     "nobody",
		PrimaryDBPassword: "nothing",
		PrimaryDBName:     "nonexistent",
		PrimaryDBPort:     "59999",
		PrimaryDBSSLMode:  "disable",
		ReplicaDBHost:     "invalid-host-that-does-not-exist",
		ReplicaDBUser:     "nobody",
		ReplicaDBPassword: "nothing",
		ReplicaDBName:     "nonexistent",
		ReplicaDBPort:     "59999",
		ReplicaDBSSLMode:  "disable",
	}

	result, err := initSingleTenantPostgres(cfg, logger)

	require.Error(t, err, "initSingleTenantPostgres must return error for invalid config")
	assert.Nil(t, result, "result must be nil when connection fails")
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (single-tenant)",
		"error must contain descriptive context")
}

// TestIntegration_InitPostgres_MultiTenantInvalidConfigReturnsError verifies
// that initMultiTenantPostgres returns an error when the PG connection fails.
func TestIntegration_InitPostgres_MultiTenantInvalidConfigReturnsError(t *testing.T) {
	logger := libZap.InitializeLogger()

	original := postgresConnector
	postgresConnector = defaultPostgresConnector
	t.Cleanup(func() { postgresConnector = original })

	cfg := &Config{
		PrimaryDBHost:     "invalid-host-that-does-not-exist",
		PrimaryDBUser:     "nobody",
		PrimaryDBPassword: "nothing",
		PrimaryDBName:     "nonexistent",
		PrimaryDBPort:     "59999",
		PrimaryDBSSLMode:  "disable",
		ReplicaDBHost:     "invalid-host-that-does-not-exist",
		ReplicaDBUser:     "nobody",
		ReplicaDBPassword: "nothing",
		ReplicaDBName:     "nonexistent",
		ReplicaDBPort:     "59999",
		ReplicaDBSSLMode:  "disable",
	}

	mockClient := tmclient.NewClient("http://localhost:0", logger)
	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       mockClient,
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantPostgres(opts, cfg, logger)

	require.Error(t, err, "initMultiTenantPostgres must return error for invalid config")
	assert.Nil(t, result, "result must be nil when connection fails")
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (multi-tenant)",
		"error must contain descriptive context")
}

// =============================================================================
// HELPERS (integration-specific)
// =============================================================================

// closePGConnection is a helper that safely closes a PostgresConnection's
// underlying DB handle. PostgresConnection does not have a Close method;
// we must close the ConnectionDB directly.
func closePGConnection(conn *libPostgres.PostgresConnection) {
	if conn != nil && conn.ConnectionDB != nil {
		_ = (*conn.ConnectionDB).Close()
	}
}
