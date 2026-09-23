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

func TestMigration000040_AddsGroupLookupIndexWithoutLedgerScope(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000040_add_group_id_index_to_transaction.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000040_add_group_id_index_to_transaction.down.sql"))
	require.NoError(t, err)

	upSQL := strings.ToLower(string(up))
	downSQL := strings.ToLower(string(down))

	assert.Contains(t, upSQL, "create index if not exists idx_transaction_group")
	assert.Contains(t, upSQL, "on transaction (group_id)")
	assert.Contains(t, upSQL, "where deleted_at is null and group_id is not null")
	assert.NotContains(t, upSQL, "organization_id, ledger_id, group_id")
	assert.Contains(t, downSQL, "drop index if exists idx_transaction_group")
}
