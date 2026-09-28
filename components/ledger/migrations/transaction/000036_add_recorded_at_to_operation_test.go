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

func TestMigration000036_FilesExist(t *testing.T) {
	t.Parallel()

	for _, filename := range []string{
		"000036_add_recorded_at_to_operation.up.sql",
		"000036_add_recorded_at_to_operation.down.sql",
	} {
		filename := filename
		t.Run(filename, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(filepath.Join(migrationsDir(t), filename))
			require.NoError(t, err)
			assert.NotEmpty(t, content)
		})
	}
}

func TestMigration000036_UpSQLAddsNullableRecordedAtWithoutDefault(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(migrationsDir(t), "000036_add_recorded_at_to_operation.up.sql"))
	require.NoError(t, err)

	sql := strings.ToLower(string(content))
	assert.Contains(t, sql, "alter table operation")
	assert.Contains(t, sql, "add column if not exists recorded_at")
	assert.Contains(t, sql, "timestamp with time zone")
	assert.NotContains(t, sql, "default")
	assert.NotContains(t, sql, "not null")
}

func TestMigration000036_DownSQLDropsRecordedAt(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(migrationsDir(t), "000036_add_recorded_at_to_operation.down.sql"))
	require.NoError(t, err)

	sql := strings.ToLower(string(content))
	assert.Contains(t, sql, "alter table operation")
	assert.Contains(t, sql, "drop column if exists recorded_at")
}
