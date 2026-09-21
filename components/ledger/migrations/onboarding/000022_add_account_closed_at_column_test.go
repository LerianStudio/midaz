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

// TestMigration000022_FilesExist verifies that migration 000022 ships both
// up and down SQL files and that neither is empty.
func TestMigration000022_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000022_add_account_closed_at_column.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000022_add_account_closed_at_column.down.sql",
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

// TestMigration000022_UpSQL_AddsClosedAtColumn verifies the up migration adds
// the nullable closed_at timestamp column to the account table. Nullable with
// no default is what keeps pre-existing accounts open: the ALTER is
// metadata-only and every historical row reads NULL.
func TestMigration000022_UpSQL_AddsClosedAtColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000022_add_account_closed_at_column.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := statementsOnly(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "targets account table", substring: "alter table account", description: "must alter the account table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-run"},
		{name: "adds closed_at column", substring: "closed_at", description: "must add the closed_at column"},
		{name: "column is TIMESTAMPTZ type", substring: "timestamptz", description: "closing instant must use TIMESTAMPTZ type"},
		{name: "column is nullable", substring: "null", description: "closed_at must be nullable so existing accounts stay open"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}

	assert.NotContains(t, sql, "default", "closed_at must carry no default: a default would mark historical accounts closed")
	assert.NotContains(t, sql, "not null", "closed_at must stay nullable")
}

// statementsOnly lowercases the migration and drops its `--` comment lines, so a
// shape assertion reads the executed SQL rather than the prose around it.
func statementsOnly(content string) string {
	var kept []string

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}

		kept = append(kept, line)
	}

	return strings.ToLower(strings.Join(kept, "\n"))
}

// TestMigration000022_DownSQL_DropsClosedAtColumn verifies the down migration
// removes the column added by the up migration and documents the rollback
// restriction: the column is the only record of a closing, so dropping it on a
// database that holds closings reopens those accounts.
func TestMigration000022_DownSQL_DropsClosedAtColumn(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000022_add_account_closed_at_column.down.sql")

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
		{name: "drops closed_at column", substring: "closed_at", description: "must drop the closed_at column"},
		{name: "uses DROP COLUMN statement", substring: "drop column", description: "must DROP COLUMN the closing instant"},
		{name: "records the rollback restriction", substring: "rollback restriction", description: "must state that the rollback is safe only without closings"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
