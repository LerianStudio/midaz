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

func TestMigration000030_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000030_create_idx_operation_org_ledger_route_code.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000030_create_idx_operation_org_ledger_route_code.down.sql",
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

func TestMigration000030_UpSQL_CreatesPartialIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000030_create_idx_operation_org_ledger_route_code.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.Contains(t, sql, "create index concurrently", "must use CREATE INDEX CONCURRENTLY")
	assert.Contains(t, sql, "if not exists", "must use IF NOT EXISTS for idempotency")
	assert.Contains(t, sql, "idx_operation_org_ledger_route_code", "must use correct index name")
	assert.Contains(t, sql, "organization_id", "must include organization_id column")
	assert.Contains(t, sql, "ledger_id", "must include ledger_id column")
	assert.Contains(t, sql, "route_code", "must include route_code column")
	assert.Contains(t, sql, "where", "must have a WHERE clause for partial index")
	assert.Contains(t, sql, "deleted_at is null", "must filter deleted_at IS NULL")
	assert.Contains(t, sql, "route_code is not null", "must filter route_code IS NOT NULL")
}

func TestMigration000030_DownSQL_DropsIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000030_create_idx_operation_org_ledger_route_code.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.Contains(t, sql, "drop index concurrently", "must use DROP INDEX CONCURRENTLY")
	assert.Contains(t, sql, "if exists", "must use IF EXISTS for idempotency")
	assert.Contains(t, sql, "idx_operation_org_ledger_route_code", "must drop the correct index name")
}
