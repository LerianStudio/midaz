// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrationsDir returns the absolute path to the migrations directory
// where this test file lives.
func migrationsDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller information")

	return filepath.Dir(filename)
}

func TestMigration000023_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000023_add_action_to_operation_transaction_route.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000023_add_action_to_operation_transaction_route.down.sql",
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

func TestMigration000023_UpSQL_AddActionColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_add_action_to_operation_transaction_route.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must add the action column with NOT NULL and DEFAULT 'direct'
	assert.Contains(t, sql, "add column action", "must ADD COLUMN action")
	assert.Contains(t, sql, "not null", "action column must be NOT NULL")
	assert.Contains(t, sql, "default 'direct'", "action column must DEFAULT to 'direct'")
}

func TestMigration000023_UpSQL_CheckConstraint(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_add_action_to_operation_transaction_route.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must add CHECK constraint for valid action values
	assert.Contains(t, sql, "chk_otr_action", "must name the constraint chk_otr_action")
	assert.Contains(t, sql, "direct", "CHECK must include 'direct'")
	assert.Contains(t, sql, "hold", "CHECK must include 'hold'")
	assert.Contains(t, sql, "commit", "CHECK must include 'commit'")
	assert.Contains(t, sql, "cancel", "CHECK must include 'cancel'")
	assert.Contains(t, sql, "revert", "CHECK must include 'revert'")
}

func TestMigration000023_UpSQL_UniqueIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_add_action_to_operation_transaction_route.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must drop old unique index
	assert.Contains(t, sql, "drop index", "must DROP the old unique index")
	assert.Contains(t, sql, "idx_operation_transaction_route_unique", "must reference the old unique index name")

	// Must create new unique index including action column
	assert.Contains(t, sql, "create unique index", "must CREATE UNIQUE INDEX")
	assert.Contains(t, sql, "operation_route_id", "new unique index must include operation_route_id")
	assert.Contains(t, sql, "transaction_route_id", "new unique index must include transaction_route_id")
	assert.Contains(t, sql, "action", "new unique index must include action")
	assert.Contains(t, sql, "where deleted_at is null", "new unique index must be partial on deleted_at IS NULL")
}

func TestMigration000023_UpSQL_ActionLookupIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_add_action_to_operation_transaction_route.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must create action lookup index
	assert.Contains(t, sql, "idx_operation_transaction_route_action", "must create action lookup index")
	assert.Contains(t, sql, "transaction_route_id", "action lookup index must include transaction_route_id")
	assert.Contains(t, sql, "action", "action lookup index must include action")
}

func TestMigration000023_DownSQL_ReverseChanges(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_add_action_to_operation_transaction_route.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	// Must drop the action lookup index
	assert.Contains(t, sql, "idx_operation_transaction_route_action", "must drop action lookup index")

	// Must drop the new unique index
	assert.Contains(t, sql, "idx_operation_transaction_route_unique", "must drop the unique index")

	// Must drop the CHECK constraint
	assert.Contains(t, sql, "chk_otr_action", "must drop the CHECK constraint")

	// Must drop the action column
	assert.Contains(t, sql, "drop column", "must DROP COLUMN")
	assert.Contains(t, sql, "action", "must drop the action column")

	// Must recreate the original unique index (without action)
	assert.Contains(t, sql, "create unique index", "must recreate original unique index")
	assert.Contains(t, sql, "operation_route_id", "recreated index must include operation_route_id")
	assert.Contains(t, sql, "transaction_route_id", "recreated index must include transaction_route_id")
}
