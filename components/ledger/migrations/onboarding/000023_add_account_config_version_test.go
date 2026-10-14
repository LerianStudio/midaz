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

func readAccountConfigVersionMigration(t *testing.T, direction string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(migrationsDir(t), "000023_add_account_config_version."+direction+".sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(statementsOnly(string(content))), " ")
	require.NotEmpty(t, sql, "the %s migration must carry statements", direction)

	return sql
}

// The column must be additive and metadata-only so the previous release keeps
// reading and inserting accounts while the migration runs ahead of its pods.
func TestMigration000023_AddsConfigVersionWithConstantDefault(t *testing.T) {
	t.Parallel()

	sql := readAccountConfigVersionMigration(t, "up")

	assert.Contains(t, sql, "alter table account add column if not exists config_version bigint not null default 0;")
}

// Only a real change of blocked may move the version: updates of other account
// fields, and a rewrite of the same value, must leave it alone.
func TestMigration000023_TriggerFiresOnlyOnBlocked(t *testing.T) {
	t.Parallel()

	sql := readAccountConfigVersionMigration(t, "up")

	assert.Contains(t, sql, "before update of blocked, config_version on account for each row execute function account_bump_config_version();")
	assert.Contains(t, sql, "if new.blocked is distinct from old.blocked then new.config_version := old.config_version + 1;")
	assert.Contains(t, sql, "else new.config_version := old.config_version;")
}

func TestMigration000023_BoundsLockWait(t *testing.T) {
	t.Parallel()

	assert.Contains(t, readAccountConfigVersionMigration(t, "up"), "set local lock_timeout = '5s';")
	assert.Contains(t, readAccountConfigVersionMigration(t, "down"), "set local lock_timeout = '5s';")
}

func TestMigration000023_DownRemovesTriggerFunctionAndColumn(t *testing.T) {
	t.Parallel()

	sql := readAccountConfigVersionMigration(t, "down")

	assert.Contains(t, sql, "drop trigger if exists account_config_version on account;")
	assert.Contains(t, sql, "drop function if exists account_bump_config_version();")
	assert.Contains(t, sql, "alter table account drop column if exists config_version;")
}
