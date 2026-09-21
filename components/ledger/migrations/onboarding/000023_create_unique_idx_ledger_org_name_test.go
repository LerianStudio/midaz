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

// TestMigration000023_FilesExist verifies that migration 000023 ships both up
// and down SQL files and that neither is empty.
func TestMigration000023_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000023_create_unique_idx_ledger_org_name.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000023_create_unique_idx_ledger_org_name.down.sql",
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

// TestMigration000023_UpSQL_CreatesUniqueLedgerNameIndex verifies the up
// migration installs the per-organization, case-insensitive uniqueness
// guarantee over live ledger rows.
func TestMigration000023_UpSQL_CreatesUniqueLedgerNameIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_create_unique_idx_ledger_org_name.up.sql")

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
		{name: "names the index idx_ledger_org_name_unique", substring: "idx_ledger_org_name_unique", description: "must name the index idx_ledger_org_name_unique"},
		{name: "targets the ledger table", substring: "on ledger", description: "must index the ledger table"},
		{name: "keys on organization_id then lower(name)", substring: "(organization_id, lower(name))", description: "must key on organization_id then LOWER(name), in that order"},
		{name: "is partial on live rows", substring: "where deleted_at is null", description: "must be partial on live rows so a soft delete releases the name"},
		{name: "documents the duplicate detection query", substring: "having count(*) > 1", description: "must carry the duplicate detection query operators need before deploying"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000023_UpSQL_DoesNotBuildConcurrently guards the decision to
// build in-line: a CONCURRENTLY build that meets a duplicate leaves an INVALID
// index behind instead of aborting clean.
func TestMigration000023_UpSQL_DoesNotBuildConcurrently(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_create_unique_idx_ledger_org_name.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	statement := strings.ToLower(string(content))
	statement = statement[strings.Index(statement, "create unique index"):]

	assert.NotContains(t, statement, "concurrently",
		"the index must be built in-line so a pre-existing duplicate aborts the migration cleanly")
}

// TestMigration000023_DownSQL_DropsIndex verifies the down migration removes
// the index added by the up migration.
func TestMigration000023_DownSQL_DropsIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000023_create_unique_idx_ledger_org_name.down.sql")

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
		{name: "targets idx_ledger_org_name_unique", substring: "idx_ledger_org_name_unique", description: "must drop the idx_ledger_org_name_unique index"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
