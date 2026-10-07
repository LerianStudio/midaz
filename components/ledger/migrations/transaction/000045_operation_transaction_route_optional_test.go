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

func TestMigration000045_AddsRequiredByDefaultOptionalFlagToLinks(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000045_operation_transaction_route_optional.up.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(up))), " ")
	assert.Contains(t, sql, "alter table operation_transaction_route add column if not exists optional boolean not null default false")
}

// Rolling back would turn optional links required again and refuse the
// transactions that leave them unused, so the down refuses while any active
// link is optional, and only then drops the column.
func TestMigration000045_DownRefusesWhileLinksAreOptional(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000045_operation_transaction_route_optional.down.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(down))), " ")

	guard := strings.Index(sql, "raise exception")
	drop := strings.Index(sql, "alter table operation_transaction_route drop column if exists optional")

	assert.Contains(t, sql, "column_name = 'optional'", "the guard must tolerate a database without the column")
	assert.Contains(t, sql, "where optional and deleted_at is null", "only active optional links block the rollback")
	require.NotEqual(t, -1, guard, "the down must refuse while links are optional")
	require.NotEqual(t, -1, drop, "the down must drop the column")
	assert.Less(t, guard, drop, "the guard must run before the column is dropped")
}
