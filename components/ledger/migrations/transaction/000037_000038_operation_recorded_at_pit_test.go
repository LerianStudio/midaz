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

func readPITMigration(t *testing.T, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(migrationsDir(t), filename))
	require.NoError(t, err)
	return strings.ToLower(string(content))
}

func TestMigration000037_CreatesRecordedAtPITIndex(t *testing.T) {
	t.Parallel()

	up := readPITMigration(t, "000037_add_idx_operation_point_in_time_recorded.up.sql")
	down := readPITMigration(t, "000037_add_idx_operation_point_in_time_recorded.down.sql")

	assert.Contains(t, up, "create index concurrently if not exists idx_operation_account_balance_pit_recorded")
	assert.Contains(t, up, "coalesce(recorded_at, created_at)) desc")
	assert.Contains(t, up, "balance_version_after desc")
	assert.Contains(t, up, "where deleted_at is null")
	assert.NotContains(t, up, "include (")
	assert.Contains(t, down, "drop index concurrently if exists idx_operation_account_balance_pit_recorded")
}

func TestMigration000038_ReplacesLegacyPITIndex(t *testing.T) {
	t.Parallel()

	up := readPITMigration(t, "000038_drop_idx_operation_point_in_time.up.sql")
	down := readPITMigration(t, "000038_drop_idx_operation_point_in_time.down.sql")

	assert.Contains(t, up, "drop index concurrently if exists idx_operation_account_balance_pit")
	assert.Contains(t, down, "create index concurrently if not exists idx_operation_account_balance_pit")
	assert.Contains(t, down, "created_at desc")
}
