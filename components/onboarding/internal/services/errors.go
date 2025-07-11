package services

import (
	"errors"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is throws an item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
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
	case "idx_settings_unique_key":
		return pkg.ValidateBusinessError(constant.ErrDuplicateSettingsKey, entityType)
	case "idx_account_type_unique_key_value":
		return pkg.ValidateBusinessError(constant.ErrDuplicateAccountTypeKeyValue, entityType)
	default:
		return pgErr
	}
}
