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

// TestMigration000020_FilesExist verifies that migration 000020 ships both
// up and down SQL files and that neither is empty.
func TestMigration000020_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000020_add_default_direction_to_account_type.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000020_add_default_direction_to_account_type.down.sql",
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

// TestMigration000020_UpSQL_AddsDefaultDirectionColumn verifies the up migration
// adds the default_direction column to the account_type table with the correct
// default and CHECK constraint.
func TestMigration000020_UpSQL_AddsDefaultDirectionColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000020_add_default_direction_to_account_type.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets account_type table", substring: "alter table account_type", description: "must alter the account_type table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-runs"},
		{name: "adds default_direction column", substring: "default_direction", description: "must add default_direction column"},
		{name: "default_direction is VARCHAR(16)", substring: "varchar(16)", description: "default_direction column must be VARCHAR(16)"},
		{name: "default_direction is NOT NULL", substring: "not null", description: "default_direction column must be NOT NULL"},
		{name: "default_direction defaults to 'credit'", substring: "default 'credit'", description: "default_direction column must default to 'credit'"},
		{name: "default_direction has CHECK constraint", substring: "check (default_direction in ('credit', 'debit'))", description: "default_direction column must have a CHECK constraint limiting values to credit/debit"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000020_DownSQL_DropsDefaultDirectionColumn verifies the down
// migration removes the default_direction column added by the up migration.
func TestMigration000020_DownSQL_DropsDefaultDirectionColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000020_add_default_direction_to_account_type.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets account_type table", substring: "alter table account_type", description: "must alter the account_type table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops default_direction column", substring: "default_direction", description: "must drop default_direction column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the default_direction field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
