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

func TestMigration000044_AddsNullableScheme(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000044_add_scheme_to_transaction.up.sql"))
	require.NoError(t, err)

	sql := normalizeSQLStatements(string(up))
	assert.Contains(t, sql, "alter table transaction add column if not exists scheme varchar(16) null")
	assert.NotContains(t, sql, "not null")
	assert.NotContains(t, sql, "default")
	assert.NotContains(t, sql, "create index")
}

func TestMigration000044_DownDropsScheme(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000044_add_scheme_to_transaction.down.sql"))
	require.NoError(t, err)

	sql := normalizeSQLStatements(string(down))
	assert.Contains(t, sql, "alter table transaction drop column if exists scheme")
}

// normalizeSQLStatements lowercases and collapses whitespace, dropping `--`
// comment lines so assertions see only the DDL.
func normalizeSQLStatements(raw string) string {
	var statements []string

	for _, line := range strings.Split(raw, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			statements = append(statements, trimmed)
		}
	}

	return strings.Join(strings.Fields(strings.ToLower(strings.Join(statements, " "))), " ")
}
