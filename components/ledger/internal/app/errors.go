package app

import (
	"errors"

	"github.com/LerianStudio/midaz/common"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is throws an item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
	switch pgErr.ConstraintName {
	case "organization_parent_organization_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Parent Organization ID Not Found",
			Code:       "0039",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		}
	case "account_parent_account_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Invalid Parent Account ID",
			Code:       "0029",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		}
	case "account_asset_code_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Asset Code Not Found",
			Code:       "0034",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		}
	case "account_portfolio_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Portfolio ID Not Found",
			Code:       "0035",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case "account_product_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Product ID Not Found",
			Code:       "0036",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case "account_ledger_id_fkey", "portfolio_ledger_id_fkey", "asset_ledger_id_fkey", "product_ledger_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Ledger ID Not Found",
			Code:       "0037",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		}
	case "account_organization_id_fkey", "ledger_organization_id_fkey", "asset_organization_id_fkey", "portfolio_organization_id_fkey", "product_organization_id_fkey":
		return common.ValidationError{
			EntityType: entityType,
			Title:      "Organization ID Not Found",
			Code:       "0038",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		}
	default:
		return pgErr
	}
}
