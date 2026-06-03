// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

func TestMigratorIntegration(t *testing.T) {
	// Use the testcontainers database URL (automatically configured by test suite)
	dbURL := testutil.GetTestDSN()

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	// Cleanup any previous test state (functions_migrations table may exist from previous runs)
	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+functionsMigrationsTable)
	require.NoError(t, err, "failed to drop migrations table")

	_, err = db.ExecContext(ctx, "DROP FUNCTION IF EXISTS test_func()")
	require.NoError(t, err, "failed to drop test function")

	tempDir := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(tempDir, "000001_test_function.up.sql"),
		[]byte("CREATE OR REPLACE FUNCTION test_func() RETURNS INTEGER AS $$ BEGIN RETURN 42; END; $$ LANGUAGE plpgsql;"),
		0o644,
	); err != nil {
		t.Fatalf("failed to write migration: %v", err)
	}

	migrator := NewFunctionMigrator(db, tempDir, nil)

	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+functionsMigrationsTable)
		assert.NoError(t, cleanupErr, "cleanup: failed to drop migrations table")

		_, cleanupErr = db.ExecContext(ctx, "DROP FUNCTION IF EXISTS test_func()")
		assert.NoError(t, cleanupErr, "cleanup: failed to drop test function")

		_ = db.Close()
	})

	version, dirty, err := migrator.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, version, "initial version")
	assert.False(t, dirty, "initial dirty")

	require.NoError(t, migrator.Up(ctx), "Up()")

	version, dirty, err = migrator.Version(ctx)
	require.NoError(t, err, "Version() after up")
	assert.Equal(t, 1, version, "version after up")
	assert.False(t, dirty, "dirty after up")

	var result int
	err = db.QueryRowContext(ctx, "SELECT test_func()").Scan(&result)
	require.NoError(t, err, "failed to call test function")
	assert.Equal(t, 42, result, "test_func() result")

	require.NoError(t, migrator.Up(ctx), "second Up() (idempotent)")

	version, dirty, err = migrator.Version(ctx)
	require.NoError(t, err, "Version() after second up")
	assert.Equal(t, 1, version, "version after second up (idempotent)")
	assert.False(t, dirty, "dirty after second up")
}
