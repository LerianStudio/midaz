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

func TestMigration000045_DownDropsTheOptionalFlag(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000045_operation_transaction_route_optional.down.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(down))), " ")
	assert.Contains(t, sql, "alter table operation_transaction_route drop column if exists optional")
}
