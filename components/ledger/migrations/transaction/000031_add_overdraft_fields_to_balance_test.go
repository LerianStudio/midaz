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

// TestMigration000031_FilesExist verifies that migration 000031 ships both
// up and down SQL files and that neither is empty.
func TestMigration000031_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000031_add_overdraft_fields_to_balance.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000031_add_overdraft_fields_to_balance.down.sql",
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

// TestMigration000031_UpSQL_AddsOverdraftColumns verifies the up migration
// adds the three columns required by the overdraft feature to the balance table.
func TestMigration000031_UpSQL_AddsOverdraftColumns(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000031_add_overdraft_fields_to_balance.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets balance table", substring: "alter table balance", description: "must alter the balance table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-runs"},
		{name: "adds direction column", substring: "direction", description: "must add direction column"},
		{name: "adds overdraft_used column", substring: "overdraft_used", description: "must add overdraft_used column"},
		{name: "adds settings column", substring: "settings", description: "must add settings column"},
		{name: "direction defaults to 'credit'", substring: "'credit'", description: "direction column must default to 'credit'"},
		{name: "settings column is JSONB", substring: "jsonb", description: "settings column must use JSONB type"},
		{name: "direction has CHECK constraint", substring: "check (direction in ('credit', 'debit'))", description: "direction column must have a CHECK constraint limiting values to credit/debit"},
		{name: "overdraft_used has CHECK constraint", substring: "check (overdraft_used >= 0)", description: "overdraft_used column must have a non-negative CHECK constraint"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000031_DownSQL_DropsOverdraftColumns verifies the down migration
// removes the three columns added by the up migration.
func TestMigration000031_DownSQL_DropsOverdraftColumns(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000031_add_overdraft_fields_to_balance.down.sql")

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
		{name: "drops direction column", substring: "direction", description: "must drop direction column"},
		{name: "drops overdraft_used column", substring: "overdraft_used", description: "must drop overdraft_used column"},
		{name: "drops settings column", substring: "settings", description: "must drop settings column"},
		{name: "uses DROP COLUMN statements", substring: "drop column", description: "must DROP COLUMN the overdraft fields"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
