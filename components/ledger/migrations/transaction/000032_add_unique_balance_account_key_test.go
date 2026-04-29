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

// TestMigration000032_FilesExist verifies that the migration ships both
// up and down SQL files and that neither is empty.
func TestMigration000032_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000032_add_unique_balance_account_key.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000032_add_unique_balance_account_key.down.sql",
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

// TestMigration000032_UpSQL_CreatesUniqueIndex verifies the up migration
// creates the partial unique index that guards against duplicate balance
// rows for the same (organization, ledger, account, asset, key) tuple.
// The index MUST be UNIQUE (to enforce the invariant) and MUST use
// CONCURRENTLY (to avoid table locks on live data).
func TestMigration000032_UpSQL_CreatesUniqueIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000032_add_unique_balance_account_key.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "creates a UNIQUE index", substring: "create unique index", description: "must declare the index as UNIQUE so duplicates raise 23505"},
		{name: "uses CONCURRENTLY", substring: "concurrently", description: "must build the index CONCURRENTLY to avoid table locks"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-runs"},
		{name: "names the index idx_unique_balance_account_key", substring: "idx_unique_balance_account_key", description: "must use the canonical index name"},
		{name: "targets balance table", substring: "on balance", description: "must create the index on the balance table"},
		{name: "includes organization_id column", substring: "organization_id", description: "index key must include organization_id"},
		{name: "includes ledger_id column", substring: "ledger_id", description: "index key must include ledger_id"},
		{name: "includes account_id column", substring: "account_id", description: "index key must include account_id"},
		{name: "includes asset_code column", substring: "asset_code", description: "index key must include asset_code"},
		{name: "includes key column", substring: "key", description: "index key must include the key column"},
		{name: "is a partial index filtering soft-deleted rows", substring: "where deleted_at is null", description: "must be a partial index so soft-deleted rows do not block new balances"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000032_DownSQL_DropsUniqueIndex verifies the down migration
// removes the index added by the up migration. DROP must also use
// CONCURRENTLY to stay symmetric with the up migration and avoid locks.
func TestMigration000032_DownSQL_DropsUniqueIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000032_add_unique_balance_account_key.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "drops the index", substring: "drop index", description: "must drop the index added by the up migration"},
		{name: "uses CONCURRENTLY", substring: "concurrently", description: "must drop the index CONCURRENTLY to avoid table locks"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "targets idx_unique_balance_account_key", substring: "idx_unique_balance_account_key", description: "must drop the canonical index name"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
