// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// businessErrCode unwraps the typed business error returned by pkg.ValidateBusinessError
// (one of EntityNotFoundError, ValidationError, EntityConflictError, …) and returns the
// Code field. Sentinel-as-Code is the project convention: the sentinel stores the numeric
// error code (e.g. "0037") and the typed error carries it on .Code, not via Unwrap.
func businessErrCode(t *testing.T, err error) string {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		return notFound.Code
	}

	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		return validation.Code
	}

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	t.Fatalf("error %v is not a typed business error", err)

	return ""
}

// TestValidatePGError_ConstraintMatrix asserts every named constraint maps to the
// correct business error. Each branch is locked by sentinel.Error() == returned.Code so
// a rename of either side surfaces here, not in production traces.
func TestValidatePGError_ConstraintMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		constraintName string
		entity         string
		wantSentinel   error
	}{
		{
			name:           "organization_parent_organization_id_fkey maps to parent org not found",
			constraintName: "organization_parent_organization_id_fkey",
			entity:         "Organization",
			wantSentinel:   constant.ErrParentOrganizationIDNotFound,
		},
		{
			name:           "account_parent_account_id_fkey maps to invalid parent account",
			constraintName: "account_parent_account_id_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrInvalidParentAccountID,
		},
		{
			name:           "account_asset_code_fkey maps to asset code not found",
			constraintName: "account_asset_code_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrAssetCodeNotFound,
		},
		{
			name:           "account_portfolio_id_fkey maps to portfolio not found",
			constraintName: "account_portfolio_id_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrPortfolioIDNotFound,
		},
		{
			name:           "account_segment_id_fkey maps to segment not found",
			constraintName: "account_segment_id_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrSegmentIDNotFound,
		},
		{
			name:           "account_ledger_id_fkey maps to ledger not found",
			constraintName: "account_ledger_id_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrLedgerIDNotFound,
		},
		{
			name:           "portfolio_ledger_id_fkey maps to ledger not found",
			constraintName: "portfolio_ledger_id_fkey",
			entity:         "Portfolio",
			wantSentinel:   constant.ErrLedgerIDNotFound,
		},
		{
			name:           "asset_ledger_id_fkey maps to ledger not found",
			constraintName: "asset_ledger_id_fkey",
			entity:         "Asset",
			wantSentinel:   constant.ErrLedgerIDNotFound,
		},
		{
			name:           "segment_ledger_id_fkey maps to ledger not found",
			constraintName: "segment_ledger_id_fkey",
			entity:         "Segment",
			wantSentinel:   constant.ErrLedgerIDNotFound,
		},
		{
			name:           "account_type_ledger_id_fkey maps to ledger not found",
			constraintName: "account_type_ledger_id_fkey",
			entity:         "AccountType",
			wantSentinel:   constant.ErrLedgerIDNotFound,
		},
		{
			name:           "account_organization_id_fkey maps to organization not found",
			constraintName: "account_organization_id_fkey",
			entity:         "Account",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "ledger_organization_id_fkey maps to organization not found",
			constraintName: "ledger_organization_id_fkey",
			entity:         "Ledger",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "asset_organization_id_fkey maps to organization not found",
			constraintName: "asset_organization_id_fkey",
			entity:         "Asset",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "portfolio_organization_id_fkey maps to organization not found",
			constraintName: "portfolio_organization_id_fkey",
			entity:         "Portfolio",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "segment_organization_id_fkey maps to organization not found",
			constraintName: "segment_organization_id_fkey",
			entity:         "Segment",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "account_type_organization_id_fkey maps to organization not found",
			constraintName: "account_type_organization_id_fkey",
			entity:         "AccountType",
			wantSentinel:   constant.ErrOrganizationIDNotFound,
		},
		{
			name:           "idx_account_type_unique_key_value maps to duplicate account type",
			constraintName: "idx_account_type_unique_key_value",
			entity:         "AccountType",
			wantSentinel:   constant.ErrDuplicateAccountTypeKeyValue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pgErr := &pgconn.PgError{ConstraintName: tc.constraintName}
			got := ValidatePGError(pgErr, tc.entity)

			require.Error(t, got)
			assert.Equal(t, tc.wantSentinel.Error(), businessErrCode(t, got),
				"expected code %s, got %v", tc.wantSentinel.Error(), got)
		})
	}
}

// TestValidatePGError_OperationRouteCheckSubstring covers the substring fallback for
// transaction-domain check constraints — the constraint name carries dynamic suffixes
// (e.g. operation_route_operation_type_check1) so the matcher uses strings.Contains.
func TestValidatePGError_OperationRouteCheckSubstring(t *testing.T) {
	t.Parallel()

	cases := []string{
		"operation_route_operation_type_check",
		"operation_route_operation_type_check1",
		"some_prefix_operation_route_operation_type_check_suffix",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pgErr := &pgconn.PgError{ConstraintName: name}
			got := ValidatePGError(pgErr, "OperationRoute")

			require.Error(t, got)
			assert.Equal(t, constant.ErrInvalidOperationRouteType.Error(), businessErrCode(t, got),
				"constraint %q should map to ErrInvalidOperationRouteType, got %v", name, got)
		})
	}
}

// TestValidatePGError_UnknownConstraint_ReturnsOriginal asserts the function returns the
// raw pg error untouched when no named branch matches. The caller can then wrap or log
// it as a generic infra failure rather than as a typed business violation.
func TestValidatePGError_UnknownConstraint_ReturnsOriginal(t *testing.T) {
	t.Parallel()

	pgErr := &pgconn.PgError{
		ConstraintName: "totally_unknown_constraint",
		Message:        "duplicate key value violates unique constraint",
	}

	got := ValidatePGError(pgErr, "Account")

	require.Error(t, got)
	// The raw pgErr is returned; it must not be a typed pkg.ValidateBusinessError.
	assert.Same(t, any(pgErr), any(got), "unknown constraint must return the original *pgconn.PgError")

	// Sanity: it is not one of the mapped business errors.
	_, isBusinessErr := got.(pkg.ValidationError)
	assert.False(t, isBusinessErr, "unknown constraint must not be a typed business error")
}
