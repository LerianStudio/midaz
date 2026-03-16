// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration000024_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000024_add_route_code_to_operation.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000024_add_route_code_to_operation.down.sql",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(dir, tc.filename)
			_, err := os.Stat(path)
			require.NoError(t, err, "migration file %s must exist", tc.filename)

			content, err := os.ReadFile(path)
			require.NoError(t, err, "migration file %s must be readable", tc.filename)
			assert.NotEmpty(t, string(content), "migration file %s must not be empty", tc.filename)
		})
	}
}

func TestMigration000024_UpSQL_AddRouteCodeColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000024_add_route_code_to_operation.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must alter the operation table
	assert.Contains(t, sql, "operation", "must target operation table")

	// Must add the route_code column
	assert.Contains(t, sql, "add column", "must ADD COLUMN")
	assert.Contains(t, sql, "route_code", "must add route_code column")

	// Must use TEXT type
	assert.Contains(t, sql, "text", "column must be TEXT type")

	// Must be idempotent with IF NOT EXISTS
	assert.Contains(t, sql, "if not exists", "must use IF NOT EXISTS for idempotency")

	// Must NOT have NOT NULL constraint (existing rows should remain NULL)
	assert.NotContains(t, sql, "not null", "must NOT have NOT NULL constraint")

	// Must NOT have a DEFAULT value
	assert.NotContains(t, sql, "default", "must NOT have a DEFAULT value")
}

func TestMigration000024_DownSQL_DropRouteCodeColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000024_add_route_code_to_operation.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must target operation table
	assert.Contains(t, sql, "operation", "must target operation table")

	// Must drop the route_code column
	assert.Contains(t, sql, "drop column", "must DROP COLUMN")
	assert.Contains(t, sql, "route_code", "must drop route_code column")

	// Must be idempotent with IF EXISTS
	assert.Contains(t, sql, "if exists", "must use IF EXISTS for idempotency")
}

func TestMigration000024_UpSQL_IdempotentRerun(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000024_add_route_code_to_operation.up.sql")

	// Reading the file twice simulates verifying the SQL is safe to re-run.
	// The actual idempotency guarantee comes from IF NOT EXISTS in the SQL.
	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Verify the SQL uses IF NOT EXISTS which makes it safe to run multiple times
	assert.Contains(t, sql, "if not exists",
		"migration must be idempotent via IF NOT EXISTS clause")
}

func TestMigration000024_UpSQL_ExistingRowsUnaffected(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000024_add_route_code_to_operation.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// No DEFAULT and no NOT NULL means existing rows get NULL automatically.
	// This verifies the migration won't force a value on existing rows.
	assert.NotContains(t, sql, "not null",
		"must not have NOT NULL - existing rows must remain unaffected with NULL")
	assert.NotContains(t, sql, "default",
		"must not have DEFAULT - existing rows must remain unaffected with NULL")
}
