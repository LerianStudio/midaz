// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	schemeUp      = "000028_transaction_validations_scheme.up.sql"
	schemeDown    = "000028_transaction_validations_scheme.down.sql"
	schemeIdxUp   = "000029_transaction_validations_scheme_idx.up.sql"
	schemeIdxDown = "000029_transaction_validations_scheme_idx.down.sql"
)

// TestMigration000028_OpensTransactionTypeWithoutRewrite verifies the up
// migration relaxes the enum column and adds the free-form scheme column with
// metadata-only ALTERs, and that the down drops only what the up added while
// restoring NOT NULL solely when no row would violate it.
func TestMigration000028_OpensTransactionTypeWithoutRewrite(t *testing.T) {
	t.Parallel()

	up := normalizeSQL(readMigration(t, schemeUp))
	down := normalizeSQL(readMigration(t, schemeDown))

	assert.Contains(t, up, "alter table transaction_validations alter column transaction_type drop not null")
	assert.Contains(t, up, "alter table transaction_validations add column if not exists scheme varchar(50)")
	assert.NotContains(t, up, "alter column transaction_type type", "the enum column must not be rewritten")
	assert.Contains(t, up, "create or replace function transaction_validation_scheme(")
	assert.Contains(t, up, "returns text language sql immutable parallel safe")
	assert.Contains(t, up, "select coalesce($1, $2::text)")
	assert.NotContains(t, up, "drop column", "the expand step drops nothing")

	assert.Contains(t, down, "drop function if exists transaction_validation_scheme(varchar, transaction_type_enum)")
	assert.Contains(t, down, "alter table transaction_validations drop column if exists scheme")
	assert.Contains(t, down, "where transaction_type is null")
	assert.Contains(t, down, "alter table transaction_validations alter column transaction_type set not null")
	assert.NotContains(t, down, "pg_catalog", "the down probes the table itself, never the catalog")
	assert.NotContains(t, down, "information_schema", "the down probes the table itself, never the catalog")
}

// TestMigration000029_IndexesCoalescedSchemeConcurrently verifies the
// expression index over transaction_validation_scheme(scheme, transaction_type)
// is built and dropped CONCURRENTLY, and that each file holds exactly one statement: the
// runner sends a file as one simple query, which PostgreSQL wraps in an
// implicit transaction whenever it carries more than one statement, and
// CONCURRENTLY refuses to run inside a transaction block.
func TestMigration000029_IndexesCoalescedSchemeConcurrently(t *testing.T) {
	t.Parallel()

	upRaw := readMigration(t, schemeIdxUp)
	downRaw := readMigration(t, schemeIdxDown)

	up := normalizeSQL(upRaw)
	down := normalizeSQL(downRaw)

	assert.Contains(t, up, "create index concurrently if not exists idx_transaction_validations_scheme")
	assert.Contains(t, up, "on transaction_validations ((transaction_validation_scheme(scheme, transaction_type)))")
	assert.Contains(t, down, "drop index concurrently if exists idx_transaction_validations_scheme")

	assert.Equal(t, 1, statementCount(upRaw), "%s must hold exactly one statement", schemeIdxUp)
	assert.Equal(t, 1, statementCount(downRaw), "%s must hold exactly one statement", schemeIdxDown)
}
