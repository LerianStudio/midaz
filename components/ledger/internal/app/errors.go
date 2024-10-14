package app

import (
	"errors"
	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is throws an item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
	switch pgErr.ConstraintName {
	case "organization_parent_organization_id_fkey":
		return common.ValidateBusinessError(cn.ParentOrganizationIDNotFoundBusinessError, entityType)
	case "account_parent_account_id_fkey":
		return common.ValidateBusinessError(cn.InvalidParentAccountIDBusinessError, entityType)
	case "account_asset_code_fkey":
		return common.ValidateBusinessError(cn.AssetCodeNotFoundBusinessError, entityType)
	case "account_portfolio_id_fkey":
		return common.ValidateBusinessError(cn.PortfolioIDNotFoundBusinessError, entityType)
	case "account_product_id_fkey":
		return common.ValidateBusinessError(cn.ProductIDNotFoundBusinessError, entityType)
	case "account_ledger_id_fkey", "portfolio_ledger_id_fkey", "asset_ledger_id_fkey", "product_ledger_id_fkey":
		return common.ValidateBusinessError(cn.LedgerIDNotFoundBusinessError, entityType)
	case "account_organization_id_fkey", "ledger_organization_id_fkey", "asset_organization_id_fkey", "portfolio_organization_id_fkey", "product_organization_id_fkey":
		return common.ValidateBusinessError(cn.OrganizationIDNotFoundBusinessError, entityType)
	default:
		return pgErr
	}
}
