// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestValidatePGError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		constraint      string
		expectSubstring string
	}{
		{"parent_organization", "organization_parent_organization_id_fkey", "constraint organization_parent_organization_id_fkey"},
		{"parent_account", "account_parent_account_id_fkey", "constraint account_parent_account_id_fkey"},
		{"asset_code", "account_asset_code_fkey", "constraint account_asset_code_fkey"},
		{"portfolio_id", "account_portfolio_id_fkey", "constraint account_portfolio_id_fkey"},
		{"segment_id", "account_segment_id_fkey", "constraint account_segment_id_fkey"},
		{"ledger_id_variants_account", "account_ledger_id_fkey", "constraint account_ledger_id_fkey"},
		{"ledger_id_variants_portfolio", "portfolio_ledger_id_fkey", "constraint portfolio_ledger_id_fkey"},
		{"ledger_id_variants_asset", "asset_ledger_id_fkey", "constraint asset_ledger_id_fkey"},
		{"ledger_id_variants_segment", "segment_ledger_id_fkey", "constraint segment_ledger_id_fkey"},
		{"ledger_id_variants_accounttype", "account_type_ledger_id_fkey", "constraint account_type_ledger_id_fkey"},
		{"org_id_variants_account", "account_organization_id_fkey", "constraint account_organization_id_fkey"},
		{"org_id_variants_ledger", "ledger_organization_id_fkey", "constraint ledger_organization_id_fkey"},
		{"org_id_variants_asset", "asset_organization_id_fkey", "constraint asset_organization_id_fkey"},
		{"org_id_variants_portfolio", "portfolio_organization_id_fkey", "constraint portfolio_organization_id_fkey"},
		{"org_id_variants_segment", "segment_organization_id_fkey", "constraint segment_organization_id_fkey"},
		{"org_id_variants_accounttype", "account_type_organization_id_fkey", "constraint account_type_organization_id_fkey"},
		{"duplicate_account_type_key", "idx_account_type_unique_key_value", "constraint idx_account_type_unique_key_value"},
		// default branch wraps the pg error directly; since pgconn.PgError.Error()
		// renders as ":  (SQLSTATE )" for a bare struct, we only verify non-nil
		// wrapping happens in a dedicated assertion below.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePGError(&pgconn.PgError{ConstraintName: tc.constraint}, "Account")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectSubstring)
		})
	}
}

func TestValidatePGError_DefaultPassthroughWrapsPgError(t *testing.T) {
	t.Parallel()

	// Unknown constraint falls through to the default branch which wraps the
	// underlying pg error verbatim.
	pgErr := &pgconn.PgError{ConstraintName: "unknown_constraint", Code: "23505"}
	err := ValidatePGError(pgErr, "Account")

	assert.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
}

func TestErrDatabaseItemNotFound(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, ErrDatabaseItemNotFound, "errDatabaseItemNotFound")
}
