//go:build integration
// +build integration

package mmigration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestDatabaseURL returns a PostgreSQL connection URL for integration tests.
// It first tries to load credentials from components/infra/.env, then falls back
// to the TEST_DATABASE_URL environment variable.
func getTestDatabaseURL(t *testing.T) string {
	t.Helper()

	// Try to load from .env file first
	if dbURL := loadFromEnvFile(t); dbURL != "" {
		return dbURL
	}

	// Fall back to environment variable
	if dbURL := os.Getenv("TEST_DATABASE_URL"); dbURL != "" {
		return dbURL
	}

	t.Skip("No database configuration found: set TEST_DATABASE_URL or ensure components/infra/.env exists")
	return ""
}

// loadFromEnvFile attempts to load database credentials from components/infra/.env
func loadFromEnvFile(t *testing.T) string {
	t.Helper()

	// Find repo root by walking up from current directory looking for go.mod
	repoRoot := findRepoRoot()
	if repoRoot == "" {
		return ""
	}

	envPath := filepath.Join(repoRoot, "components", "infra", ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return ""
	}

	// Load .env file (doesn't override existing env vars)
	if err := godotenv.Load(envPath); err != nil {
		t.Logf("Warning: could not load %s: %v", envPath, err)
		return ""
	}

	// Build connection URL from env vars
	host := getEnvOrDefault("DB_HOST", "localhost")
	port := getEnvOrDefault("DB_PORT", "5701")
	user := getEnvOrDefault("DB_USER", "midaz")
	password := getEnvOrDefault("DB_PASSWORD", "lerian")
	dbName := getEnvOrDefault("DB_NAME", "midaz")

	// For local testing, use localhost instead of Docker container name
	if host == "midaz-postgres-primary" {
		host = "localhost"
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbName)
}

// findRepoRoot walks up the directory tree to find the repository root (contains go.mod)
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestIntegration_DirtyRecovery tests the full dirty recovery workflow against a real database.
// Run with: go test -tags=integration -v ./pkg/mmigration/...
//
// Database credentials are loaded automatically from components/infra/.env
// or can be overridden with TEST_DATABASE_URL environment variable.
func TestIntegration_DirtyRecovery(t *testing.T) {
	dbURL := getTestDatabaseURL(t)

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Ensure clean state
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS schema_migrations")

	// Create schema_migrations table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint NOT NULL PRIMARY KEY,
			dirty boolean NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert dirty migration state
	_, err = db.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, dirty) VALUES (15, true)
		ON CONFLICT (version) DO UPDATE SET dirty = true
	`)
	require.NoError(t, err)

	// Create temp directory with a dummy migration file for recovery
	migrationsDir := t.TempDir()
	dummyMigrationFile := filepath.Join(migrationsDir, "000015_dummy.up.sql")
	err = os.WriteFile(dummyMigrationFile, []byte("-- dummy migration for test"), 0o644)
	require.NoError(t, err)

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "integration-test",
		MigrationsPath:   migrationsDir,
		AutoRecoverDirty: true,
		MaxRetries:       3,
		RetryBackoff:     100 * time.Millisecond,
		LockTimeout:      5 * time.Second,
	})
	defer ctrl.Finish()

	// Test 1: Preflight should detect dirty state
	status, err := wrapper.PreflightCheck(ctx, db)
	assert.ErrorIs(t, err, ErrMigrationDirty)
	assert.Equal(t, 15, status.Version)
	assert.True(t, status.Dirty)

	// Test 2: Recovery should clear dirty flag
	err = wrapper.recoverDirtyMigration(ctx, db, status.Version)
	assert.NoError(t, err)

	// Test 3: Preflight should now succeed
	status, err = wrapper.PreflightCheck(ctx, db)
	assert.NoError(t, err)
	assert.Equal(t, 15, status.Version)
	assert.False(t, status.Dirty)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DROP TABLE schema_migrations")
}

// TestIntegration_AdvisoryLock tests advisory lock behavior against a real database.
func TestIntegration_AdvisoryLock(t *testing.T) {
	dbURL := getTestDatabaseURL(t)

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:   "integration-test-lock",
		LockTimeout: 5 * time.Second,
	})
	defer ctrl.Finish()

	// Test 1: Acquire lock
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Test 2: Same session can re-acquire (PostgreSQL behavior)
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Test 3: Release lock
	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Release second acquisition
	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	assert.NoError(t, err)
}

// TestIntegration_ConcurrentLock tests that two different sessions cannot hold the same lock.
func TestIntegration_ConcurrentLock(t *testing.T) {
	dbURL := getTestDatabaseURL(t)

	// Connection 1
	db1, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db1.Close()

	// Connection 2
	db2, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db2.Close()

	ctx := context.Background()

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:   "concurrent-lock-test",
		LockTimeout: 100 * time.Millisecond, // Short timeout for test
	})
	defer ctrl.Finish()

	// Session 1 acquires lock
	err = wrapper.AcquireAdvisoryLock(ctx, db1)
	require.NoError(t, err)

	// Session 2 should fail to acquire (lock held by session 1)
	err = wrapper.AcquireAdvisoryLock(ctx, db2)
	assert.ErrorIs(t, err, ErrMigrationLockFailed)

	// Session 1 releases lock
	err = wrapper.ReleaseAdvisoryLock(ctx, db1)
	require.NoError(t, err)

	// Now session 2 can acquire
	err = wrapper.AcquireAdvisoryLock(ctx, db2)
	assert.NoError(t, err)

	// Cleanup
	_ = wrapper.ReleaseAdvisoryLock(ctx, db2)
}
