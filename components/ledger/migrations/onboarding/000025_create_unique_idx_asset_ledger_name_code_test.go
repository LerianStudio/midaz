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

// TestMigration000025_FilesExist verifies that migration 000025 ships both up
// and down SQL files and that neither is empty.
func TestMigration000025_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000025_create_unique_idx_asset_ledger_name_code.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000025_create_unique_idx_asset_ledger_name_code.down.sql",
		},
	}

	for _, tc := range tests {
		tc := tc
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

// TestMigration000025_UpSQL_CreatesUniqueAssetIndexes verifies the up migration
// installs the per-ledger uniqueness guarantees over live asset rows: name
// case-insensitive, code exact.
func TestMigration000025_UpSQL_CreatesUniqueAssetIndexes(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000025_create_unique_idx_asset_ledger_name_code.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "creates a unique index", substring: "create unique index", description: "must create a UNIQUE index"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-runs"},
		{name: "names the index idx_asset_ledger_name_unique", substring: "idx_asset_ledger_name_unique", description: "must name the name index idx_asset_ledger_name_unique"},
		{name: "names the index idx_asset_ledger_code_unique", substring: "idx_asset_ledger_code_unique", description: "must name the code index idx_asset_ledger_code_unique"},
		{name: "targets the asset table", substring: "on asset", description: "must index the asset table"},
		{name: "keys the name index on organization_id, ledger_id then lower(name)", substring: "(organization_id, ledger_id, lower(name))", description: "must key the name index on organization_id, ledger_id then LOWER(name), in that order"},
		{name: "keys the code index on organization_id, ledger_id then code", substring: "(organization_id, ledger_id, code)", description: "must key the code index on organization_id, ledger_id then the exact code, in that order"},
		{name: "is partial on live rows", substring: "where deleted_at is null", description: "must be partial on live rows so a soft delete releases the name and code"},
		{name: "documents the duplicate detection query", substring: "having count(*) > 1", description: "must carry the duplicate detection queries operators need before deploying"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}

	assert.Equal(t, 2, strings.Count(sql, "create unique index"), "must create exactly the name and code indexes")
	assert.Equal(t, 2, strings.Count(sql, "where deleted_at is null;"), "both indexes must be partial on live rows")
}

// TestMigration000025_UpSQL_DoesNotBuildConcurrently guards the decision to
// build in-line: a CONCURRENTLY build that meets a duplicate leaves an INVALID
// index behind instead of aborting clean.
func TestMigration000025_UpSQL_DoesNotBuildConcurrently(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000025_create_unique_idx_asset_ledger_name_code.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	statement := strings.ToLower(string(content))
	statement = statement[strings.Index(statement, "create unique index"):]

	assert.NotContains(t, statement, "concurrently",
		"the indexes must be built in-line so a pre-existing duplicate aborts the migration cleanly")
}

// TestMigration000025_DownSQL_DropsIndexes verifies the down migration removes
// both indexes added by the up migration.
func TestMigration000025_DownSQL_DropsIndexes(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000025_create_unique_idx_asset_ledger_name_code.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "drops an index", substring: "drop index", description: "must drop an index"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "targets idx_asset_ledger_name_unique", substring: "idx_asset_ledger_name_unique", description: "must drop the idx_asset_ledger_name_unique index"},
		{name: "targets idx_asset_ledger_code_unique", substring: "idx_asset_ledger_code_unique", description: "must drop the idx_asset_ledger_code_unique index"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
