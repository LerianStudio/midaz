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

// TestMigration000034_FilesExist verifies that migration 000034 ships both
// up and down SQL files and that neither is empty.
func TestMigration000034_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000034_create_transaction_backup_quarantine.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000034_create_transaction_backup_quarantine.down.sql",
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

// TestMigration000034_UpSQL_CreatesQuarantineTable verifies the up migration
// creates the transaction_backup_quarantine table with the invariant-critical
// columns: a NOT NULL payload (the financial copy) and a UNIQUE redis_key.
func TestMigration000034_UpSQL_CreatesQuarantineTable(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000034_create_transaction_backup_quarantine.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "creates the table", substring: "create table if not exists transaction_backup_quarantine", description: "must create transaction_backup_quarantine"},
		{name: "id primary key", substring: "id              uuid primary key not null", description: "must define id as UUID primary key"},
		{name: "organization_id column", substring: "organization_id uuid not null", description: "must add organization_id column"},
		{name: "ledger_id column", substring: "ledger_id       uuid not null", description: "must add ledger_id column"},
		{name: "transaction_id column", substring: "transaction_id  uuid not null", description: "must add transaction_id column"},
		{name: "redis_key is unique", substring: "redis_key       text not null unique", description: "redis_key must be UNIQUE so a record lands exactly once"},
		{name: "payload is NOT NULL bytea", substring: "payload         bytea not null", description: "payload (the financial copy) must be opaque BYTEA NOT NULL so non-JSON poison bytes can be stored verbatim"},
		{name: "failure_reason column", substring: "failure_reason  text", description: "must add failure_reason column"},
		{name: "attempts column", substring: "attempts        integer not null default 0", description: "must add attempts column"},
		{name: "first_failed_at column", substring: "first_failed_at timestamp with time zone", description: "must add first_failed_at column"},
		{name: "quarantined_at defaults to now", substring: "quarantined_at  timestamp with time zone not null default now()", description: "quarantined_at must default to now()"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000034_DownSQL_DropsQuarantineTable verifies the down migration
// removes the table and its index.
func TestMigration000034_DownSQL_DropsQuarantineTable(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000034_create_transaction_backup_quarantine.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops the table", substring: "drop table if exists transaction_backup_quarantine", description: "must DROP the transaction_backup_quarantine table"},
		{name: "drops the index", substring: "drop index if exists idx_transaction_backup_quarantine_org_ledger", description: "must DROP the org/ledger index"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
