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

// The index is built online on a hot table, keyed by balance then version so the
// top-1-by-version lookup of one balance reads a single entry, and partial on the rows
// that trail lookup filters on.
func TestMigration000046_CreatesPartialVersionIndexConcurrently(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000046_add_idx_operation_balance_version.up.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(up))), " ")

	assert.Contains(t, sql, "create index concurrently if not exists idx_operation_balance_version on operation")
	assert.Contains(t, sql, "on operation (balance_id, balance_version_after desc)")
	assert.Contains(t, sql, "where deleted_at is null and balance_affected = true;")
}

func TestMigration000046_DownDropsIndexConcurrently(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000046_add_idx_operation_balance_version.down.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(down))), " ")
	assert.Contains(t, sql, "drop index concurrently if exists idx_operation_balance_version")
}
