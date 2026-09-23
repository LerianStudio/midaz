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

func TestMigration000041_CreatesTransactionGroupLifecycleStore(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000041_create_transaction_group.up.sql"))
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(up))), " ")
	for _, fragment := range []string{
		"create table if not exists transaction_group",
		"id uuid primary key",
		"organization_id uuid not null",
		"ledger_id uuid not null",
		"status varchar(100) not null",
		"asset_code varchar(100) not null",
		"intent jsonb not null",
		"created_at timestamp with time zone not null default now()",
		"updated_at timestamp with time zone not null default now()",
		"idx_transaction_group_org_ledger_status",
		"create index if not exists idx_transaction_group_pending_id on transaction_group (id) where status = 'pending'",
	} {
		assert.Contains(t, sql, fragment)
	}
}

func TestMigration000041_DropsTransactionGroupLifecycleStore(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	down, err := os.ReadFile(filepath.Join(dir, "000041_create_transaction_group.down.sql"))
	require.NoError(t, err)

	sql := strings.ToLower(string(down))
	assert.Contains(t, sql, "drop index if exists idx_transaction_group_pending_id")
	assert.Contains(t, sql, "drop index if exists idx_transaction_group_org_ledger_status")
	assert.Contains(t, sql, "drop table if exists transaction_group")
}
