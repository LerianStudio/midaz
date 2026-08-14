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

// TestMigration000035_FilesExist verifies that migration 000035 ships both
// up and down SQL files and that neither is empty.
func TestMigration000035_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000035_add_skip_audit_to_transaction.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000035_add_skip_audit_to_transaction.down.sql",
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

// TestMigration000035_UpSQL_AddsSkipAuditColumns verifies the up migration
// adds the fees_skipped and tracer_skipped boolean columns with NOT NULL and
// a DEFAULT FALSE to the transaction table. The columns are metadata-only on
// PostgreSQL 11+ (non-volatile constant default does not trigger a rewrite),
// and IF NOT EXISTS keeps the ALTER idempotent.
func TestMigration000035_UpSQL_AddsSkipAuditColumns(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000035_add_skip_audit_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets transaction table", substring: "alter table transaction", description: "must alter the transaction table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-run"},
		{name: "adds fees_skipped column", substring: "fees_skipped", description: "must add fees_skipped column"},
		{name: "adds tracer_skipped column", substring: "tracer_skipped", description: "must add tracer_skipped column"},
		{name: "columns are BOOLEAN type", substring: "boolean", description: "skip-audit columns must use BOOLEAN type"},
		{name: "columns are NOT NULL", substring: "not null", description: "skip-audit columns must be NOT NULL"},
		{name: "columns default to FALSE", substring: "default false", description: "skip-audit columns must default to FALSE for historical rows"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000035_DownSQL_DropsSkipAuditColumns verifies the down
// migration removes both columns added by the up migration with idempotent
// IF EXISTS guards.
func TestMigration000035_DownSQL_DropsSkipAuditColumns(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000035_add_skip_audit_to_transaction.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets transaction table", substring: "alter table transaction", description: "must alter the transaction table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops fees_skipped column", substring: "fees_skipped", description: "must drop fees_skipped column"},
		{name: "drops tracer_skipped column", substring: "tracer_skipped", description: "must drop tracer_skipped column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the skip-audit fields"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
