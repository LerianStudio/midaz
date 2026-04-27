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

// TestMigration000033_FilesExist verifies that migration 000033 ships both
// up and down SQL files and that neither is empty.
func TestMigration000033_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000033_add_snapshot_to_operation.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000033_add_snapshot_to_operation.down.sql",
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

// TestMigration000033_UpSQL_AddsSnapshotColumn verifies the up migration
// adds a snapshot JSONB column with NOT NULL and a DEFAULT '{}' to the
// operation table. The column is metadata-only on PostgreSQL 11+
// (non-volatile default does not trigger a table rewrite).
func TestMigration000033_UpSQL_AddsSnapshotColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000033_add_snapshot_to_operation.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets operation table", substring: "alter table operation", description: "must alter the operation table"},
		{name: "adds snapshot column", substring: "snapshot", description: "must add snapshot column"},
		{name: "snapshot is JSONB type", substring: "jsonb", description: "snapshot column must use JSONB type"},
		{name: "snapshot is NOT NULL", substring: "not null", description: "snapshot column must be NOT NULL"},
		{name: "snapshot has empty object default", substring: "'{}'::jsonb", description: "snapshot column must default to empty JSONB object"},
		{name: "documents metadata-only nature", substring: "metadata-only", description: "must document that this is a metadata-only ALTER on PostgreSQL 11+"},
		{name: "documents no GIN index decision", substring: "gin", description: "must document the decision not to add a GIN index"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000033_DownSQL_DropsSnapshotColumn verifies the down migration
// removes the snapshot column added by the up migration.
func TestMigration000033_DownSQL_DropsSnapshotColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000033_add_snapshot_to_operation.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets operation table", substring: "alter table operation", description: "must alter the operation table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "drops snapshot column", substring: "snapshot", description: "must drop snapshot column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the snapshot field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
