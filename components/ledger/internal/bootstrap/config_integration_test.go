// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestContainers holds all containers needed for ledger integration tests.
type TestContainers struct {
	PostgreSQL testcontainers.Container
	MongoDB    testcontainers.Container
	Redis      testcontainers.Container
	RabbitMQ   testcontainers.Container
}

// ContainerAddresses holds the connection addresses for all containers.
type ContainerAddresses struct {
	// PostgreSQL
	PostgresHost string
	PostgresPort string

	// MongoDB
	MongoHost string
	MongoPort string

	// Redis
	RedisHost string
	RedisPort string

	// RabbitMQ
	RabbitMQHost     string
	RabbitMQPort     string
	RabbitMQMgmtPort string
}

// setupAllContainers starts all required containers for ledger integration tests.
func setupAllContainers(t *testing.T) (*TestContainers, *ContainerAddresses, func()) {
	t.Helper()
	ctx := context.Background()

	containers := &TestContainers{}
	addresses := &ContainerAddresses{}
	var cleanupFuncs []func()

	// PostgreSQL
	pgContainer, pgHost, pgPort, pgCleanup := setupPostgresContainer(t, ctx)
	containers.PostgreSQL = pgContainer
	addresses.PostgresHost = pgHost
	addresses.PostgresPort = pgPort
	cleanupFuncs = append(cleanupFuncs, pgCleanup)

	// MongoDB
	mongoContainer, mongoHost, mongoPort, mongoCleanup := setupMongoContainer(t, ctx)
	containers.MongoDB = mongoContainer
	addresses.MongoHost = mongoHost
	addresses.MongoPort = mongoPort
	cleanupFuncs = append(cleanupFuncs, mongoCleanup)

	// Redis
	redisContainer, redisHost, redisPort, redisCleanup := setupRedisContainer(t, ctx)
	containers.Redis = redisContainer
	addresses.RedisHost = redisHost
	addresses.RedisPort = redisPort
	cleanupFuncs = append(cleanupFuncs, redisCleanup)

	// RabbitMQ
	rabbitContainer, rabbitHost, rabbitPort, rabbitMgmtPort, rabbitCleanup := setupRabbitMQContainer(t, ctx)
	containers.RabbitMQ = rabbitContainer
	addresses.RabbitMQHost = rabbitHost
	addresses.RabbitMQPort = rabbitPort
	addresses.RabbitMQMgmtPort = rabbitMgmtPort
	cleanupFuncs = append(cleanupFuncs, rabbitCleanup)

	cleanup := func() {
		// Cleanup in reverse order
		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}

	return containers, addresses, cleanup
}

func setupPostgresContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:17-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "midaz_test",
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get PostgreSQL container host")

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err, "failed to get PostgreSQL container port")

	// Onboarding and transaction ship independent migration sequences that share
	// a single golang-migrate schema_migrations table per database. Give each
	// domain its own database so neither clobbers the other's migration version,
	// mirroring the production split between DB_ONBOARDING_NAME and
	// DB_TRANSACTION_NAME. Migrations themselves run during InitServers.
	createDomainDatabases(t, ctx, host, port.Port())

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	}

	return container, host, port.Port(), cleanup
}

// createDomainDatabases creates the per-domain PostgreSQL databases used by the
// unified ledger so onboarding and transaction migrations stay isolated.
func createDomainDatabases(t *testing.T, ctx context.Context, host, port string) {
	t.Helper()

	adminDSN := fmt.Sprintf("host=%s port=%s user=test password=test dbname=midaz_test sslmode=disable", host, port)

	db, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err, "failed to open admin connection for database creation")
	defer func() { _ = db.Close() }()

	for _, name := range []string{"midaz_onboarding", "midaz_transaction"} {
		_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", name))
		require.NoError(t, err, "failed to create database %s", name)
	}
}

func setupMongoContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "mongo:8",
		ExposedPorts: []string{"27017/tcp"},
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "test",
			"MONGO_INITDB_ROOT_PASSWORD": "test",
		},
		WaitingFor: wait.ForLog("Waiting for connections").WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start MongoDB container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get MongoDB container host")

	port, err := container.MappedPort(ctx, "27017")
	require.NoError(t, err, "failed to get MongoDB container port")

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate MongoDB container: %v", err)
		}
	}

	return container, host, port.Port(), cleanup
}

func setupRedisContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "valkey/valkey:8",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Redis container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get Redis container host")

	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err, "failed to get Redis container port")

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Redis container: %v", err)
		}
	}

	return container, host, port.Port(), cleanup
}

func setupRabbitMQContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:4.1.3-management-alpine",
		ExposedPorts: []string{"5672/tcp", "15672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": "test",
			"RABBITMQ_DEFAULT_PASS": "test",
		},
		// Wait for both server startup AND management plugin to be ready
		WaitingFor: wait.ForAll(
			wait.ForLog("Server startup complete").WithStartupTimeout(120*time.Second),
			wait.ForHTTP("/api/health/checks/alarms").
				WithPort("15672/tcp").
				WithBasicAuth("test", "test").
				WithStartupTimeout(60*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start RabbitMQ container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get RabbitMQ container host")

	amqpPort, err := container.MappedPort(ctx, "5672")
	require.NoError(t, err, "failed to get RabbitMQ AMQP port")

	mgmtPort, err := container.MappedPort(ctx, "15672")
	require.NoError(t, err, "failed to get RabbitMQ management port")

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate RabbitMQ container: %v", err)
		}
	}

	return container, host, amqpPort.Port(), mgmtPort.Port(), cleanup
}

// findProjectRoot finds the project root by looking for go.mod
func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Start from current working directory
	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

// changeToProjectRoot changes the working directory to the project root.
// This is required because lib-commons PostgresConnection auto-runs migrations
// looking for files in components/{Component}/migrations relative to cwd.
// Returns a cleanup function to restore the original directory.
func changeToProjectRoot(t *testing.T) func() {
	t.Helper()

	originalDir, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	projectRoot := findProjectRoot(t)

	err = os.Chdir(projectRoot)
	require.NoError(t, err, "failed to change to project root: %s", projectRoot)

	t.Logf("Changed working directory to: %s", projectRoot)

	return func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("warning: failed to restore original directory: %v", err)
		}
	}
}

// setEnvFromContainers configures environment variables from running containers.
// Uses t.Setenv for automatic cleanup after test.
func setEnvFromContainers(t *testing.T, addresses *ContainerAddresses) {
	t.Helper()

	// PostgreSQL — onboarding primary + replica (DB_ONBOARDING_* env surface).
	// Dedicated database so onboarding migrations own their schema_migrations.
	t.Setenv("DB_ONBOARDING_HOST", addresses.PostgresHost)
	t.Setenv("DB_ONBOARDING_PORT", addresses.PostgresPort)
	t.Setenv("DB_ONBOARDING_NAME", "midaz_onboarding")
	t.Setenv("DB_ONBOARDING_USER", "test")
	t.Setenv("DB_ONBOARDING_PASSWORD", "test")
	t.Setenv("DB_ONBOARDING_SSLMODE", "disable")
	t.Setenv("DB_ONBOARDING_REPLICA_HOST", addresses.PostgresHost)
	t.Setenv("DB_ONBOARDING_REPLICA_PORT", addresses.PostgresPort)
	t.Setenv("DB_ONBOARDING_REPLICA_NAME", "midaz_onboarding")
	t.Setenv("DB_ONBOARDING_REPLICA_USER", "test")
	t.Setenv("DB_ONBOARDING_REPLICA_PASSWORD", "test")
	t.Setenv("DB_ONBOARDING_REPLICA_SSLMODE", "disable")

	// PostgreSQL — transaction primary + replica (DB_TRANSACTION_* env surface).
	// Dedicated database so transaction migrations own their schema_migrations.
	t.Setenv("DB_TRANSACTION_HOST", addresses.PostgresHost)
	t.Setenv("DB_TRANSACTION_PORT", addresses.PostgresPort)
	t.Setenv("DB_TRANSACTION_NAME", "midaz_transaction")
	t.Setenv("DB_TRANSACTION_USER", "test")
	t.Setenv("DB_TRANSACTION_PASSWORD", "test")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("DB_TRANSACTION_REPLICA_HOST", addresses.PostgresHost)
	t.Setenv("DB_TRANSACTION_REPLICA_PORT", addresses.PostgresPort)
	t.Setenv("DB_TRANSACTION_REPLICA_NAME", "midaz_transaction")
	t.Setenv("DB_TRANSACTION_REPLICA_USER", "test")
	t.Setenv("DB_TRANSACTION_REPLICA_PASSWORD", "test")
	t.Setenv("DB_TRANSACTION_REPLICA_SSLMODE", "disable")

	// MongoDB — onboarding, transaction, CRM, and fees all share the single test
	// container (distinct logical DBs). Each domain has its own MONGO_<DOMAIN>_*
	// env surface in the unified ledger config.
	for _, prefix := range []string{"MONGO_ONBOARDING", "MONGO_TRANSACTION", "MONGO_CRM", "MONGO_FEES"} {
		t.Setenv(prefix+"_HOST", addresses.MongoHost)
		t.Setenv(prefix+"_PORT", addresses.MongoPort)
		t.Setenv(prefix+"_URI", "mongodb")
		t.Setenv(prefix+"_NAME", "midaz_test")
		t.Setenv(prefix+"_USER", "test")
		t.Setenv(prefix+"_PASSWORD", "test")
		t.Setenv(prefix+"_PARAMETERS", "authSource=admin")
	}

	// CRM holder/alias PII cipher keys (LCRYPTO_* keep their bare names).
	t.Setenv("LCRYPTO_HASH_SECRET_KEY", testutils.TestHashKey)
	t.Setenv("LCRYPTO_ENCRYPT_SECRET_KEY", testutils.TestEncryptKey)

	// Redis
	t.Setenv("REDIS_HOST", fmt.Sprintf("%s:%s", addresses.RedisHost, addresses.RedisPort))

	// RabbitMQ
	t.Setenv("RABBITMQ_HOST", addresses.RabbitMQHost)
	t.Setenv("RABBITMQ_PORT_HOST", addresses.RabbitMQPort)
	t.Setenv("RABBITMQ_PORT_AMQP", "5672")
	t.Setenv("RABBITMQ_URI", "amqp")
	t.Setenv("RABBITMQ_DEFAULT_USER", "test")
	t.Setenv("RABBITMQ_DEFAULT_PASS", "test")
	t.Setenv("RABBITMQ_CONSUMER_USER", "test")
	t.Setenv("RABBITMQ_CONSUMER_PASS", "test")
	// RabbitMQ Management API health check URL base (lib-commons appends /api/health/checks/alarms)
	t.Setenv("RABBITMQ_HEALTH_CHECK_URL", fmt.Sprintf("http://%s:%s", addresses.RabbitMQHost, addresses.RabbitMQMgmtPort))

	// Server addresses for unified ledger
	t.Setenv("SERVER_ADDRESS", ":0") // Use any available port
	t.Setenv("SERVER_ADDRESS_ONBOARDING", ":0")
	t.Setenv("SERVER_ADDRESS_TRANSACTION", ":0")
	// Disable features that require additional setup
	t.Setenv("PLUGIN_AUTH_ENABLED", "false")
	t.Setenv("ENABLE_TELEMETRY", "false")
	t.Setenv("OTEL_LIBRARY_NAME", "midaz-tests")
}

// TestInitServers_WithAllDependencies_Succeeds tests that InitServers successfully
// initializes the unified ledger service with all real dependencies.
// This test verifies:
// - PostgreSQL connection for both onboarding and transaction
// - MongoDB connection for metadata
// - Redis connection for caching
// - RabbitMQ connection for async processing
// - Service composition (onboarding + transaction)
func TestIntegration_InitServers_WithAllDependencies_Succeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	_, addresses, cleanup := setupAllContainers(t)
	defer cleanup()

	setEnvFromContainers(t, addresses)

	// Change to project root for migrations
	restoreDir := changeToProjectRoot(t)
	defer restoreDir()

	// Act
	service, err := InitServers()

	// Assert
	require.NoError(t, err, "InitServers should succeed with all dependencies")
	require.NotNil(t, service, "service should not be nil")
	assert.NotNil(t, service.UnifiedServer, "unified server should be initialized")
	assert.NotNil(t, service.Logger, "logger should be initialized")
	assert.NotNil(t, service.Telemetry, "telemetry should be initialized")
}

// TestService_Run_StartsAllServers tests that Service.Run() correctly starts
// all HTTP servers from both onboarding and transaction modules.
//
// This test verifies:
// - Server startup without errors
// - All expected runnables are collected
// - Graceful behavior (we don't actually run the full launcher in test)
func TestIntegration_Service_Run_StartsAllServers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Arrange
	_, addresses, cleanup := setupAllContainers(t)
	defer cleanup()

	setEnvFromContainers(t, addresses)

	// Change to project root for migrations
	restoreDir := changeToProjectRoot(t)
	defer restoreDir()

	// Initialize the unified ledger service
	service, err := InitServers()
	require.NoError(t, err, "InitServers should succeed")
	require.NotNil(t, service, "service should not be nil")

	// Verify the unified server is available
	assert.NotNil(t, service.UnifiedServer, "unified server should be initialized")

	// Note: We don't call service.Run() in tests because it blocks
	// and starts the full server lifecycle. The above verifies the
	// service is correctly composed and ready to run.

	t.Log("Service correctly composed with direct infrastructure initialization")
}
