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

func TestMigration000036_AddsNullableGroupIDAndScopedIndex(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000036_add_group_id_to_transaction.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000036_add_group_id_to_transaction.down.sql"))
	require.NoError(t, err)

	upSQL := strings.ToLower(string(up))
	assert.Contains(t, upSQL, "add column if not exists group_id uuid")
	assert.NotContains(t, upSQL, "group_id uuid not null")
	assert.Contains(t, upSQL, "create index if not exists")
	assert.Contains(t, upSQL, "organization_id, ledger_id, group_id")

	downSQL := strings.ToLower(string(down))
	assert.Contains(t, downSQL, "drop index if exists")
	assert.Contains(t, downSQL, "drop column if exists group_id")
}
