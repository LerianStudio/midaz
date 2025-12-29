//go:build integration
// +build integration

package mmigration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_DirtyRecovery tests the full dirty recovery workflow against a real database.
// Run with: go test -tags=integration -v ./pkg/mmigration/...
//
// Requires environment variable:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost:5432/testdb?sslmode=disable
func TestIntegration_DirtyRecovery(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

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

	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "integration-test",
		AutoRecoverDirty: true,
		MaxRetries:       3,
		RetryBackoff:     100 * time.Millisecond,
		LockTimeout:      5 * time.Second,
	})

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
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	wrapper := newTestWrapper(t, MigrationConfig{
		Component:   "integration-test-lock",
		LockTimeout: 5 * time.Second,
	})

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
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	// Connection 1
	db1, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db1.Close()

	// Connection 2
	db2, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db2.Close()

	ctx := context.Background()

	wrapper := newTestWrapper(t, MigrationConfig{
		Component:   "concurrent-lock-test",
		LockTimeout: 100 * time.Millisecond, // Short timeout for test
	})

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
