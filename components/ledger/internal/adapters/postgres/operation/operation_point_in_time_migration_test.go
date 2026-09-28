// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTransactionMigrationFile(t *testing.T, fileName string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime caller must resolve current test file")

	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "migrations", "transaction", fileName)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(contents)
}

func TestRecordedAtMigration_IsRollingUpdateSafe(t *testing.T) {
	t.Parallel()

	recordedAtUp := strings.ToUpper(readTransactionMigrationFile(t, "000036_add_recorded_at_to_operation.up.sql"))

	assert.Contains(t, recordedAtUp, "ADD COLUMN IF NOT EXISTS RECORDED_AT TIMESTAMP WITH TIME ZONE")
	assert.NotContains(t, recordedAtUp, "NOT NULL", "old pods must be able to omit recorded_at")
	assert.NotContains(t, recordedAtUp, "DEFAULT", "the additive migration must not rewrite existing rows")
}

func TestOperationPointInTimeMigration_SemanticShape(t *testing.T) {
	t.Parallel()

	pointInTimeUp := readTransactionMigrationFile(t, "000037_add_idx_operation_point_in_time_recorded.up.sql")
	legacyIndexRemoval := readTransactionMigrationFile(t, "000038_drop_idx_operation_point_in_time.up.sql")

	// Verify the unified lean index with balance_version_after in key, no INCLUDE columns
	assert.Contains(t, pointInTimeUp, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_balance_pit_recorded")
	assert.Contains(t, pointInTimeUp, "ON operation (organization_id, ledger_id, account_id, balance_id, (COALESCE(recorded_at, created_at)) DESC, balance_version_after DESC)")
	assert.NotContains(t, pointInTimeUp, "INCLUDE (", "lean index must not have INCLUDE columns")
	assert.Contains(t, pointInTimeUp, "WHERE deleted_at IS NULL")

	assert.Contains(t, legacyIndexRemoval, "DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account_balance_pit;")
}
