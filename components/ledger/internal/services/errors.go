// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown when an item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// pgCodeUndefinedColumn is the PostgreSQL SQLSTATE for a reference to a column
// that does not exist on the relation.
const pgCodeUndefinedColumn = "42703"

// IsSchemaDrift reports whether err is a PostgreSQL undefined-column failure,
// i.e. the statement named a column the applied migrations have not created.
func IsSchemaDrift(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == pgCodeUndefinedColumn
}

// ValidatePGError validates pgError and returns the appropriate business error.
// It handles constraint violations from both onboarding and transaction entities.
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	// A named column the database does not have means the binary is ahead of the
	// applied migrations. It carries no constraint name, so it has to be matched
	// on SQLSTATE before the constraint switch.
	if pgErr.Code == pgCodeUndefinedColumn {
		return pkg.ValidateBusinessError(constant.ErrSchemaMigrationPending, entityType)
	}

	// Onboarding constraint violations
	switch pgErr.ConstraintName {
	case "organization_parent_organization_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrParentOrganizationIDNotFound, entityType)
	case "account_parent_account_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, entityType)
	case "account_asset_code_fkey":
		return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, entityType)
	case "account_portfolio_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, entityType)
	case "account_segment_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, entityType)
	case "account_ledger_id_fkey", "portfolio_ledger_id_fkey", "asset_ledger_id_fkey", "segment_ledger_id_fkey", "account_type_ledger_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, entityType)
	case "account_organization_id_fkey", "ledger_organization_id_fkey", "asset_organization_id_fkey", "portfolio_organization_id_fkey", "segment_organization_id_fkey", "account_type_organization_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, entityType)
	case "idx_account_type_unique_key_value":
		return pkg.ValidateBusinessError(constant.ErrDuplicateAccountTypeKeyValue, entityType)
	}

	// Transaction constraint violations
	switch {
	case strings.Contains(pgErr.ConstraintName, "operation_route_operation_type_check"):
		return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
	}

	return pgErr
}
