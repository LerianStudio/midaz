// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTransactionMigrationFile(t *testing.T, fileName string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime caller must resolve current test file")

	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "migrations", fileName)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(contents)
}

func TestOperationPointInTimeMigration_SemanticShape(t *testing.T) {
	t.Parallel()

	up := readTransactionMigrationFile(t, "000017_add_idx_operation_point_in_time.up.sql")
	down := readTransactionMigrationFile(t, "000017_add_idx_operation_point_in_time.down.sql")

	assert.Contains(t, up, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_point_in_time")
	assert.Contains(t, up, "ON operation (organization_id, ledger_id, balance_id, created_at DESC)")
	assert.Contains(t, up, "INCLUDE (id, balance_key, available_balance_after, on_hold_balance_after, balance_version_after, account_id, asset_code)")
	assert.Contains(t, up, "WHERE deleted_at IS NULL")

	assert.Contains(t, up, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_point_in_time")
	assert.Contains(t, up, "ON operation (organization_id, ledger_id, account_id, balance_id, created_at DESC)")
	assert.Contains(t, up, "INCLUDE (id, balance_key, available_balance_after, on_hold_balance_after, balance_version_after, asset_code)")

	assert.Contains(t, down, "DROP INDEX CONCURRENTLY IF EXISTS idx_operation_point_in_time;")
	assert.Contains(t, down, "DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account_point_in_time;")
}
