package services

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is returned when a database query finds no matching records.
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError translates PostgreSQL errors to business-friendly error messages.
//
// This function maps PostgreSQL constraint violations (foreign keys, unique constraints)
// to user-friendly business errors defined in the constant package. It examines the
// constraint name from PostgreSQL errors and returns appropriate error codes.
//
// Handled Constraints:
// - Foreign key violations → Entity not found errors (e.g., parent organization doesn't exist)
// - Unique constraint violations → Duplicate/conflict errors (e.g., duplicate account type key)
//
// Parameters:
//   - pgErr: The PostgreSQL error from database operation
//   - entityType: The entity type being operated on (for error context)
//
// Returns:
//   - error: Business error with appropriate code and message, or original pgErr if not mapped
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
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
	default:
		return pgErr
	}
}
