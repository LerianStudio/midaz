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

// TestMigration000021_FilesExist verifies that migration 000021 ships both
// up and down SQL files and that neither is empty.
func TestMigration000021_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000021_create_idx_account_org_holder.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000021_create_idx_account_org_holder.down.sql",
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

// TestMigration000021_UpSQL_CreatesOrgHolderIndex verifies the up migration
// creates the partial (organization_id, holder_id) index that backs the
// org-wide holder account listing.
func TestMigration000021_UpSQL_CreatesOrgHolderIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000021_create_idx_account_org_holder.up.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "up migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "creates an index", substring: "create index", description: "must create an index"},
		{name: "creates it concurrently", substring: "concurrently", description: "must build the index CONCURRENTLY to avoid locking writes"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "if not exists", description: "must use IF NOT EXISTS for idempotent re-runs"},
		{name: "names the index idx_account_org_holder", substring: "idx_account_org_holder", description: "must name the index idx_account_org_holder"},
		{name: "targets the account table", substring: "on account", description: "must index the account table"},
		{name: "keys on organization_id then holder_id", substring: "(organization_id, holder_id)", description: "must key on organization_id then holder_id, in that order"},
		{name: "is partial on live rows", substring: "where deleted_at is null", description: "must be partial on live rows only"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000021_DownSQL_DropsIndex verifies the down migration removes
// the index added by the up migration.
func TestMigration000021_DownSQL_DropsIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	path := filepath.Join(dir, "000021_create_idx_account_org_holder.down.sql")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "down migration file must be readable")

	sql := strings.ToLower(string(content))

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "drops an index", substring: "drop index", description: "must drop an index"},
		{name: "drops it concurrently", substring: "concurrently", description: "must drop the index CONCURRENTLY to avoid locking writes"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "targets idx_account_org_holder", substring: "idx_account_org_holder", description: "must drop the idx_account_org_holder index"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
