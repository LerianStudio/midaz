package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	switch pgErr.ConstraintName {
	// Organization constraints
	case "organization_parent_organization_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrParentOrganizationIDNotFound, entityType)

	// Account constraints
	case "account_parent_account_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, entityType)
	case "account_asset_code_fkey":
		return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, entityType)
	case "account_portfolio_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, entityType)
	case "account_segment_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, entityType)

	// Ledger ID constraints
	case "account_ledger_id_fkey", "portfolio_ledger_id_fkey", "asset_ledger_id_fkey",
		"segment_ledger_id_fkey", "account_type_ledger_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, entityType)

	// Organization ID constraints
	case "account_organization_id_fkey", "ledger_organization_id_fkey", "asset_organization_id_fkey",
		"portfolio_organization_id_fkey", "segment_organization_id_fkey", "account_type_organization_id_fkey":
		return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, entityType)

	// Account Type constraints
	case "idx_account_type_unique_key_value":
		return pkg.ValidateBusinessError(constant.ErrDuplicateAccountTypeKeyValue, entityType)

	default:
		// Handle operation route type check separately since it requires pattern matching
		if strings.Contains(pgErr.ConstraintName, "operation_route_type_check") ||
			strings.Contains(pgErr.Message, "type") && strings.Contains(pgErr.Message, "debit") && strings.Contains(pgErr.Message, "credit") {
			return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
		}

		return pgErr
	}
}
