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

// TestMigration000019_FilesExist verifies that migration 000019 ships both
// up and down SQL files and that neither is empty.
func TestMigration000019_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000019_add_holder_skip_audit_to_account.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000019_add_holder_skip_audit_to_account.down.sql",
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

// TestMigration000019_UpSQL_AddsHolderSkipAuditColumn verifies the up
// migration adds the holder_check_skipped boolean column with NOT NULL and
// a DEFAULT FALSE to the account table. The column is metadata-only on
// PostgreSQL 11+ (non-volatile constant default does not trigger a rewrite),
// and IF NOT EXISTS keeps the ALTER idempotent.
func TestMigration000019_UpSQL_AddsHolderSkipAuditColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000019_add_holder_skip_audit_to_account.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets account table", substring: "alter table account", description: "must alter the account table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-run"},
		{name: "adds holder_check_skipped column", substring: "holder_check_skipped", description: "must add holder_check_skipped column"},
		{name: "column is BOOLEAN type", substring: "boolean", description: "skip-audit column must use BOOLEAN type"},
		{name: "column is NOT NULL", substring: "not null", description: "skip-audit column must be NOT NULL"},
		{name: "column defaults to FALSE", substring: "default false", description: "skip-audit column must default to FALSE for historical rows"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000019_DownSQL_DropsHolderSkipAuditColumn verifies the down
// migration removes the column added by the up migration with an idempotent
// IF EXISTS guard.
func TestMigration000019_DownSQL_DropsHolderSkipAuditColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000019_add_holder_skip_audit_to_account.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets account table", substring: "alter table account", description: "must alter the account table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops holder_check_skipped column", substring: "holder_check_skipped", description: "must drop holder_check_skipped column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the skip-audit field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
