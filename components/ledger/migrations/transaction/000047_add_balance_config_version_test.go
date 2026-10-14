// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readBalanceConfigVersionMigration(t *testing.T, direction string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(migrationsDir(t), "000047_add_balance_config_version."+direction+".sql"))
	require.NoError(t, err)

	sql := normalizeSQLStatements(string(content))
	require.NotEmpty(t, sql, "the %s migration must carry statements", direction)

	return sql
}

// The column must be additive and metadata-only so the previous release keeps
// reading and inserting balances while the migration runs ahead of its pods.
func TestMigration000047_AddsConfigVersionWithConstantDefault(t *testing.T) {
	t.Parallel()

	sql := readBalanceConfigVersionMigration(t, "up")

	assert.Contains(t, sql, "alter table balance add column if not exists config_version bigint not null default 0;")
}

// Only a real change of a configuration column may move the version: the balance
// sync writes money columns only and must never fire the trigger, and a rewrite of
// the same value must not count as a change.
func TestMigration000047_TriggerFiresOnlyOnConfigurationColumns(t *testing.T) {
	t.Parallel()

	sql := readBalanceConfigVersionMigration(t, "up")

	assert.Contains(t, sql, "before update of settings, allow_sending, allow_receiving, config_version on balance for each row execute function balance_bump_config_version();")
	assert.Contains(t, sql, "if new.settings is distinct from old.settings or new.allow_sending is distinct from old.allow_sending or new.allow_receiving is distinct from old.allow_receiving then new.config_version := old.config_version + 1;")
	assert.Contains(t, sql, "else new.config_version := old.config_version;")
	assert.NotContains(t, sql, "available", "the trigger must not depend on money columns")
}

// A long reader must make the migration fail rather than queue every balance write
// behind its ACCESS EXCLUSIVE request.
func TestMigration000047_BoundsLockWait(t *testing.T) {
	t.Parallel()

	assert.Contains(t, readBalanceConfigVersionMigration(t, "up"), "set local lock_timeout = '5s';")
	assert.Contains(t, readBalanceConfigVersionMigration(t, "down"), "set local lock_timeout = '5s';")
}

func TestMigration000047_DownRemovesTriggerFunctionAndColumn(t *testing.T) {
	t.Parallel()

	sql := readBalanceConfigVersionMigration(t, "down")

	assert.Contains(t, sql, "drop trigger if exists balance_config_version on balance;")
	assert.Contains(t, sql, "drop function if exists balance_bump_config_version();")
	assert.Contains(t, sql, "alter table balance drop column if exists config_version;")
}
