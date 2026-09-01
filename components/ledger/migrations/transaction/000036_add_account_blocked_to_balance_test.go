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

// TestMigration000036_FilesExist verifies that migration 000036 ships both
// up and down SQL files and that neither is empty.
func TestMigration000036_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000036_add_account_blocked_to_balance.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000036_add_account_blocked_to_balance.down.sql",
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

// TestMigration000036_UpSQL_AddsAccountBlockedColumn verifies the up migration
// adds the account_blocked boolean column with NOT NULL and a DEFAULT FALSE to
// the balance table. The column denormalizes the account-level block flag into
// the balance read model, which is the only structure loaded by the hot
// validation path. It is metadata-only on PostgreSQL 11+ (non-volatile constant
// default does not trigger a rewrite), and IF NOT EXISTS keeps the ALTER
// idempotent for multi-tenant re-runs.
func TestMigration000036_UpSQL_AddsAccountBlockedColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000036_add_account_blocked_to_balance.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets balance table", substring: "alter table balance", description: "must alter the balance table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-run"},
		{name: "adds account_blocked column", substring: "account_blocked", description: "must add account_blocked column"},
		{name: "column is BOOLEAN type", substring: "boolean", description: "account_blocked must use BOOLEAN type"},
		{name: "column is NOT NULL", substring: "not null", description: "account_blocked must be NOT NULL"},
		{name: "column defaults to FALSE", substring: "default false", description: "account_blocked must default to FALSE for historical rows"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000036_UpSQL_CreatesNoIndex verifies the up migration does not
// create an index. The flag is read via an already-loaded balance row and is
// never filtered on in isolation, so an index would add write amplification on
// the hot path for no read benefit.
func TestMigration000036_UpSQL_CreatesNoIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000036_add_account_blocked_to_balance.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "create index", "up migration must not create an index for account_blocked")
}

// TestMigration000036_DownSQL_DropsAccountBlockedColumn verifies the down
// migration removes the column added by the up migration with an idempotent
// IF EXISTS guard.
func TestMigration000036_DownSQL_DropsAccountBlockedColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000036_add_account_blocked_to_balance.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets balance table", substring: "alter table balance", description: "must alter the balance table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops account_blocked column", substring: "account_blocked", description: "must drop account_blocked column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the account_blocked field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
