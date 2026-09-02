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

// readNormalizedSQL reads a migration file and returns its contents lowercased
// with every run of whitespace collapsed to a single space, so that assertions
// can pin column definitions without depending on the file's alignment padding.
func readNormalizedSQL(t *testing.T, filename string) string {
	t.Helper()

	path := filepath.Join(migrationsDir(t), filename)

	content, err := os.ReadFile(path)
	require.NoError(t, err, "migration file %s must be readable", filename)

	return strings.Join(strings.Fields(strings.ToLower(string(content))), " ")
}

// TestMigration000022_FilesExist verifies that migration 000022 ships both
// up and down SQL files and that neither is empty.
func TestMigration000022_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "up migration file exists",
			filename: "000022_create_account_exception_table.up.sql",
		},
		{
			name:     "down migration file exists",
			filename: "000022_create_account_exception_table.down.sql",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(dir, tc.filename)
			_, err := os.Stat(path)
			require.NoError(t, err, "migration file %s must exist", tc.filename)

			content, err := os.ReadFile(path)
			require.NoError(t, err, "migration file %s must be readable", tc.filename)
			assert.NotEmpty(t, string(content), "migration file %s must not be empty", tc.filename)
		})
	}
}

// TestMigration000022_UpSQL_CreatesAccountExceptionTable verifies the up
// migration creates the account_exception table with tenant scope
// (organization + ledger), the owning account, the optional validity window
// and soft-delete support.
func TestMigration000022_UpSQL_CreatesAccountExceptionTable(t *testing.T) {
	t.Parallel()

	sql := readNormalizedSQL(t, "000022_create_account_exception_table.up.sql")

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "creates a table", substring: "create table", description: "must create a table"},
		{name: "uses IF NOT EXISTS for idempotency", substring: "create table if not exists account_exception", description: "must create account_exception with IF NOT EXISTS for idempotent re-runs"},
		{name: "declares the id primary key", substring: "id uuid primary key not null", description: "must declare id as a non-null UUID primary key"},
		{name: "declares organization_id", substring: "organization_id uuid not null", description: "must scope rows to an organization"},
		{name: "declares ledger_id", substring: "ledger_id uuid not null", description: "must scope rows to a ledger"},
		{name: "declares account_id", substring: "account_id uuid not null", description: "the rule lives under an account, so account_id is mandatory"},
		{name: "declares operational_type_codes as JSONB", substring: "operational_type_codes jsonb not null", description: "must store the operational type code list as a mandatory JSONB collection"},
		{name: "declares balance_key as nullable varchar(100)", substring: "balance_key varchar(100),", description: "balance_key must be nullable (NULL means every balance) and mirror the balance Key limit of 100"},
		{name: "declares context as mandatory varchar(256)", substring: "context varchar(256) not null", description: "context is mandatory and capped at 256 characters"},
		{name: "declares effective_at as nullable", substring: "effective_at timestamp with time zone,", description: "effective_at must be nullable — the validity window is optional"},
		{name: "declares expires_at as nullable", substring: "expires_at timestamp with time zone,", description: "expires_at must be nullable — NULL means an open-ended rule"},
		{name: "declares created_at", substring: "created_at timestamp with time zone not null", description: "must record creation time"},
		{name: "declares updated_at with a default", substring: "updated_at timestamp with time zone not null default now()", description: "must record update time defaulting to now()"},
		{name: "declares deleted_at as nullable", substring: "deleted_at timestamp with time zone,", description: "deleted_at must be nullable to support soft delete"},
		{name: "references organization", substring: "foreign key (organization_id) references organization (id)", description: "must have a foreign key to organization"},
		{name: "references ledger", substring: "foreign key (ledger_id) references ledger (id)", description: "must have a foreign key to ledger"},
		{name: "references account", substring: "foreign key (account_id) references account (id)", description: "must have a foreign key to account"},
		{name: "creates the scope lookup index", substring: "create index if not exists idx_account_exception_org_ledger_account on account_exception (organization_id, ledger_id, account_id) where deleted_at is null", description: "must create the partial scope index backing enrichment lookup and listing"},
		{name: "creates the deleted_at index", substring: "create index if not exists idx_account_exception_deleted_at on account_exception (organization_id, ledger_id, deleted_at)", description: "must create the deleted_at index following the account_type convention"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}

// TestMigration000022_UpSQL_HasNoNaturalKeyUniqueIndex verifies the up
// migration does NOT constrain a natural key: several exception rules may
// coexist for the same account.
func TestMigration000022_UpSQL_HasNoNaturalKeyUniqueIndex(t *testing.T) {
	t.Parallel()

	sql := readNormalizedSQL(t, "000022_create_account_exception_table.up.sql")

	assert.NotContains(t, sql, "unique index", "must not add a natural-key unique index — multiple rules per account are allowed")
}

// TestMigration000022_DownSQL_DropsTable verifies the down migration removes
// the table added by the up migration.
func TestMigration000022_DownSQL_DropsTable(t *testing.T) {
	t.Parallel()

	sql := readNormalizedSQL(t, "000022_create_account_exception_table.down.sql")

	tests := []struct {
		name        string
		substring   string
		description string
	}{
		{name: "drops a table", substring: "drop table", description: "must drop a table"},
		{name: "uses IF EXISTS for idempotency", substring: "if exists", description: "must use IF EXISTS for idempotent rollback"},
		{name: "targets account_exception", substring: "account_exception", description: "must drop the account_exception table"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, sql, tc.substring, tc.description)
		})
	}
}
