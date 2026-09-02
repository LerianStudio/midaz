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

// TestMigration000037_FilesExist verifies that migration 000037 ships both
// up and down SQL files and that neither is empty.
func TestMigration000037_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000037_add_operational_type_code_to_transaction.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000037_add_operational_type_code_to_transaction.down.sql",
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

// TestMigration000037_UpSQL_AddsOperationalTypeCodeColumn verifies the up
// migration adds the operational_type_code TEXT column as NULLABLE to the
// transaction table. The column records the operational type code applied to a
// transaction when an account exception routed it. It is metadata-only on
// PostgreSQL 11+ (a nullable column with no default triggers no rewrite), and
// IF NOT EXISTS keeps the ALTER idempotent for multi-tenant re-runs.
func TestMigration000037_UpSQL_AddsOperationalTypeCodeColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000037_add_operational_type_code_to_transaction.up.sql")

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
		{name: "adds operational_type_code column", substring: "operational_type_code", description: "must add operational_type_code column"},
		{name: "column is TEXT type", substring: "text", description: "operational_type_code must use TEXT type"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000037_UpSQL_ColumnIsNullable verifies the up migration adds the
// column as nullable: the frozen response contract makes operational_type_code
// present only when applicable, so historical rows must read NULL (omitted from
// JSON) rather than being forced to a non-null default. A NOT NULL clause would
// also make the ADD COLUMN a table rewrite risk, which the additive design
// avoids.
func TestMigration000037_UpSQL_ColumnIsNullable(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000037_add_operational_type_code_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "not null", "operational_type_code must be nullable (no NOT NULL constraint)")
	assert.NotContains(t, sql, "default", "operational_type_code must not carry a default; absent means NULL")
}

// TestMigration000037_UpSQL_CreatesNoIndex verifies the up migration does not
// create an index. The field is read from an already-loaded transaction row on
// the extract path and is never filtered on in isolation, so an index would add
// write amplification for no read benefit.
func TestMigration000037_UpSQL_CreatesNoIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000037_add_operational_type_code_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "create index", "up migration must not create an index for operational_type_code")
}

// TestMigration000037_DownSQL_DropsOperationalTypeCodeColumn verifies the down
// migration removes the column added by the up migration with an idempotent
// IF EXISTS guard.
func TestMigration000037_DownSQL_DropsOperationalTypeCodeColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000037_add_operational_type_code_to_transaction.down.sql")

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
		{name: "drops operational_type_code column", substring: "operational_type_code", description: "must drop operational_type_code column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the operational_type_code field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
