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

// TestMigration000038_FilesExist verifies that migration 000038 ships both
// up and down SQL files and that neither is empty.
func TestMigration000038_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000038_add_applied_exception_id_to_transaction.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000038_add_applied_exception_id_to_transaction.down.sql",
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

// TestMigration000038_UpSQL_AddsAppliedExceptionIDColumn verifies the up
// migration adds the applied_exception_id UUID column as NULLABLE to the
// transaction table. The column records which account exception rule applied to
// a transaction. It is metadata-only on PostgreSQL 11+ (a nullable column with
// no default triggers no rewrite), and IF NOT EXISTS keeps the ALTER idempotent
// for multi-tenant re-runs.
func TestMigration000038_UpSQL_AddsAppliedExceptionIDColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000038_add_applied_exception_id_to_transaction.up.sql")

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
		{name: "adds applied_exception_id column", substring: "applied_exception_id", description: "must add applied_exception_id column"},
		{name: "column is UUID type", substring: "uuid", description: "applied_exception_id must use UUID type"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000038_UpSQL_ColumnIsNullable verifies the up migration adds the
// column as nullable: the value is written only when an account exception
// applied, so historical rows must read NULL rather than being forced to a
// non-null default. A NOT NULL clause would also make the ADD COLUMN a table
// rewrite risk, which the additive design avoids.
func TestMigration000038_UpSQL_ColumnIsNullable(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000038_add_applied_exception_id_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "not null", "applied_exception_id must be nullable (no NOT NULL constraint)")
	assert.NotContains(t, sql, "default", "applied_exception_id must not carry a default; absent means NULL")
}

// TestMigration000038_UpSQL_CreatesNoIndex verifies the up migration does not
// create an index. The field is read from an already-loaded transaction row on
// the extract path; "which transactions passed through an exception" is an
// analytical query, not a hot path, so an index would add write amplification
// for no read benefit on the transaction hot path.
func TestMigration000038_UpSQL_CreatesNoIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000038_add_applied_exception_id_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "create index", "up migration must not create an index for applied_exception_id")
}

// TestMigration000038_UpSQL_CreatesNoForeignKey verifies the up migration does
// not create a foreign key. The exception rule lives in the onboarding database
// (cross-DB, so a FK is impossible), and the soft delete of a rule must not
// invalidate the historical trail of a transaction that already referenced it.
func TestMigration000038_UpSQL_CreatesNoForeignKey(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000038_add_applied_exception_id_to_transaction.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	assert.NotContains(t, sql, "references", "applied_exception_id must not carry a foreign key (cross-DB onboarding)")
	assert.NotContains(t, sql, "foreign key", "applied_exception_id must not declare a foreign key constraint")
}

// TestMigration000038_DownSQL_DropsAppliedExceptionIDColumn verifies the down
// migration removes the column added by the up migration with an idempotent
// IF EXISTS guard.
func TestMigration000038_DownSQL_DropsAppliedExceptionIDColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000038_add_applied_exception_id_to_transaction.down.sql")

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
		{name: "drops applied_exception_id column", substring: "applied_exception_id", description: "must drop applied_exception_id column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the applied_exception_id field"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
