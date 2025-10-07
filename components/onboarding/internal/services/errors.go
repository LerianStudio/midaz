// Package services provides shared utilities for the onboarding service business logic layer.
//
// This package contains error handling utilities that are shared between the command
// and query packages. It bridges the gap between database-level errors (PostgreSQL)
// and business-level errors (domain errors).
package services

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound indicates that a requested database item was not found.
//
// This sentinel error is used throughout the service layer to indicate "not found"
// conditions from repository queries. It's typically converted to business errors
// (like ErrAccountIDNotFound, ErrLedgerIDNotFound) at the use case layer.
//
// Usage:
//
//	account, err := repo.Find(ctx, id)
//	if errors.Is(err, services.ErrDatabaseItemNotFound) {
//	    return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "Account")
//	}
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError converts PostgreSQL constraint errors to business errors.
//
// This function maps PostgreSQL constraint violation errors to appropriate business
// error codes. It's used throughout the service layer to provide user-friendly error
// messages when database constraints are violated.
//
// The function examines the constraint name from the PostgreSQL error and returns
// the corresponding business error. This provides:
//   - User-friendly error messages
//   - Consistent error codes across the API
//   - Proper HTTP status code mapping
//   - Detailed error information for clients
//
// Mapped Constraints:
//   - Foreign Key Constraints:
//   - organization_parent_organization_id_fkey → ErrParentOrganizationIDNotFound
//   - account_parent_account_id_fkey → ErrInvalidParentAccountID
//   - account_asset_code_fkey → ErrAssetCodeNotFound
//   - account_portfolio_id_fkey → ErrPortfolioIDNotFound
//   - account_segment_id_fkey → ErrSegmentIDNotFound
//   - *_ledger_id_fkey → ErrLedgerIDNotFound
//   - *_organization_id_fkey → ErrOrganizationIDNotFound
//   - Unique Constraints:
//   - idx_account_type_unique_key_value → ErrDuplicateAccountTypeKeyValue
//
// Parameters:
//   - pgErr: PostgreSQL error from pgx driver
//   - entityType: Type of entity being operated on (for error context)
//
// Returns:
//   - error: Business error with appropriate code, or original pgErr if not mapped
//
// Example:
//
//	_, err := repo.Create(ctx, account)
//	if err != nil {
//	    var pgErr *pgconn.PgError
//	    if errors.As(err, &pgErr) {
//	        return services.ValidatePGError(pgErr, "Account")
//	    }
//	    return err
//	}
//
// Note: Unmapped constraint names return the original PostgreSQL error, which will
// be converted to an internal server error at the HTTP layer.
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
