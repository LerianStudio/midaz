// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Package testutil_dbsuite provides a lightweight TestMain helper that starts a
// PostgreSQL testcontainer and optionally applies migrations.
//
// Unlike testutil_integration.SetupTestSuite, this package does NOT import
// bootstrap or start an HTTP server — it only provides a database. This avoids
// import cycles when used from packages that bootstrap itself depends on
// (e.g. internal/adapters/postgres).
package testutil_dbsuite

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Database name used for the throwaway Postgres container. Kept as a single
// constant so postgres.WithDatabase (container creation) and
// libPostgres.NewMigrator.DatabaseName (migration target) cannot drift.
const testDBName = "tracer_test"

// testPostgresMaxConnections mirrors
// testutil_integration.TestPostgresMaxConnections. It is NOT imported from
// that package because testutil_integration depends on bootstrap, and this
// package exists precisely to avoid that import cycle (see the package
// godoc). The canonical value and full rationale live in
// internal/testutil_integration/testcontainer.go — keep these two in sync.
const testPostgresMaxConnections = 300

// dbEnvVars lists environment variables managed by the suite.
var dbEnvVars = []string{
	"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
}

// suiteConfig holds configuration for SetupTestDBSuite.
type suiteConfig struct {
	migrationsPath string
}

// Option configures SetupTestDBSuite behavior.
type Option func(*suiteConfig)

// WithMigrations enables migrations from the given directory.
// The path should point to a directory of numbered migration files applied by
// golang-migrate in ascending order — the same set consumed by applyMigrations.
func WithMigrations(path string) Option {
	return func(cfg *suiteConfig) {
		cfg.migrationsPath = path
	}
}

// SetupTestDBSuite starts a PostgreSQL testcontainer, optionally applies
// migrations, runs m.Run(), then tears down. Returns the exit code for os.Exit.
func SetupTestDBSuite(m *testing.M, opts ...Option) int {
	var cfg suiteConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx := context.Background()

	saved := saveEnv()

	container, connStr, host, port, err := startPostgres(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		restoreEnv(saved)
		return 1
	}

	os.Setenv("DB_HOST", host)
	os.Setenv("DB_PORT", port)
	os.Setenv("DB_USER", "tracer")
	os.Setenv("DB_PASSWORD", "tracer")
	os.Setenv("DB_NAME", testDBName)

	if cfg.migrationsPath != "" {
		if err := applyMigrations(ctx, connStr, cfg.migrationsPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to apply migrations: %v\n", err)

			if termErr := container.Terminate(ctx); termErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to terminate container: %v\n", termErr)
			}

			restoreEnv(saved)

			return 1
		}
	}

	code := m.Run()

	if termErr := container.Terminate(ctx); termErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to terminate container: %v\n", termErr)
	}

	restoreEnv(saved)

	return code
}

// startPostgres creates a throwaway PostgreSQL container.
func startPostgres(ctx context.Context) (*postgres.PostgresContainer, string, string, string, error) {
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(testDBName),
		postgres.WithUsername("tracer"),
		postgres.WithPassword("tracer"),
		// Keep max_connections aligned with the shared integration suite
		// (internal/testutil_integration/testcontainer.go) so db-suite-driven
		// tests do not hit the default 100-connection ceiling under the same
		// pre-existing service-lifecycle pressure (orphan rule_sync_workers
		// that outlive programmatic Shutdown). The full rationale lives at
		// the canonical usage site referenced above.
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Cmd: []string{"-c", fmt.Sprintf("max_connections=%d", testPostgresMaxConnections)},
			},
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("start container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", "", "", fmt.Errorf("get host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", "", "", fmt.Errorf("get port: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", "", "", fmt.Errorf("get connstr: %w", err)
	}

	return container, connStr, host, mappedPort.Port(), nil
}

// applyMigrations applies all migrations via lib-commons's Migrator — the
// same runner the bootstrap path uses at startup. It is NOT a byte-for-byte
// reproduction of the bootstrap flow: the Logger wiring here is a minimal
// libZap instance configured at ERROR level (keeps the test suite quiet
// unless something actually breaks), rather than the full zap+OTel-resource
// logger bootstrap.initCoreInfra builds from env vars. For the purpose of
// this suite — exercising migration apply semantics — the runner instance
// that matters is libPostgres.Migrator, which is identical on both paths.
//
// AllowMultiStatements must stay false because migrations 000001-000003 install
// PL/pgSQL functions using dollar-quoted bodies ($$...$$); golang-migrate's
// multi-statement mode splits on semicolons and corrupts dollar-quoted blocks.
func applyMigrations(ctx context.Context, connectionString, migrationsPath string) error {
	// Minimal test-silence logger: routes structured migration events through
	// the same libLog.Logger contract bootstrap uses (so the Migrator exercises
	// the real logging path), but defaults to ERROR-level so successful runs
	// don't clutter `make test-integration` output. Any real failure still
	// surfaces on stderr.
	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.EnvironmentProduction,
		Level:           "ERROR",
		OTelLibraryName: "tracer-test",
	})
	if err != nil {
		return fmt.Errorf("create test logger: %w", err)
	}

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:           connectionString,
		DatabaseName:         testDBName,
		MigrationsPath:       migrationsPath,
		AllowMultiStatements: false,
		Logger:               logger,
	})
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	// Bound the migration run so a wedged advisory lock (two concurrent
	// suites racing, or an orphaned lock from a killed test process) surfaces
	// as a clean timeout error rather than hanging the test binary past any
	// CI timeout. Mirrors the production bootstrap path in
	// internal/bootstrap/config.go so migration lifecycle behavior is symmetric
	// between runtime boot and db-suite tests.
	migCtx, migCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer migCancel()

	if err := migrator.Up(migCtx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

// saveEnv captures current values of DB env vars for later restoration.
func saveEnv() map[string]*string {
	saved := make(map[string]*string, len(dbEnvVars))
	for _, name := range dbEnvVars {
		if val, ok := os.LookupEnv(name); ok {
			saved[name] = &val
		} else {
			saved[name] = nil
		}
	}
	return saved
}

// restoreEnv restores env vars to their original values.
func restoreEnv(saved map[string]*string) {
	for _, name := range dbEnvVars {
		ptr, existed := saved[name]
		if !existed || ptr == nil {
			os.Unsetenv(name)
		} else {
			os.Setenv(name, *ptr)
		}
	}
}
