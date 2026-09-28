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

func TestMigration000043_AllowsRoutesWithoutLedger(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000043_route_ledger_id_nullable.up.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(up))), " ")
	assert.Contains(t, sql, "alter table transaction_route alter column ledger_id drop not null")
	assert.Contains(t, sql, "alter table operation_route alter column ledger_id drop not null")
}

func TestMigration000043_DownRestoresRequiredLedger(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000043_route_ledger_id_nullable.down.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(down))), " ")
	assert.Contains(t, sql, "alter table transaction_route alter column ledger_id set not null")
	assert.Contains(t, sql, "alter table operation_route alter column ledger_id set not null")
}
