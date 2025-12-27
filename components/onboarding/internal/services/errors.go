package services

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is throws an item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// pgConstraintErrorMap maps PostgreSQL constraint names to their corresponding business errors.
// Using a map reduces cyclomatic complexity compared to a switch statement.
var pgConstraintErrorMap = map[string]error{
	"organization_parent_organization_id_fkey": constant.ErrParentOrganizationIDNotFound,
	"account_parent_account_id_fkey":           constant.ErrInvalidParentAccountID,
	"account_asset_code_fkey":                  constant.ErrAssetCodeNotFound,
	"account_portfolio_id_fkey":                constant.ErrPortfolioIDNotFound,
	"account_segment_id_fkey":                  constant.ErrSegmentIDNotFound,
	"account_ledger_id_fkey":                   constant.ErrLedgerIDNotFound,
	"portfolio_ledger_id_fkey":                 constant.ErrLedgerIDNotFound,
	"asset_ledger_id_fkey":                     constant.ErrLedgerIDNotFound,
	"segment_ledger_id_fkey":                   constant.ErrLedgerIDNotFound,
	"account_type_ledger_id_fkey":              constant.ErrLedgerIDNotFound,
	"account_organization_id_fkey":             constant.ErrOrganizationIDNotFound,
	"ledger_organization_id_fkey":              constant.ErrOrganizationIDNotFound,
	"asset_organization_id_fkey":               constant.ErrOrganizationIDNotFound,
	"portfolio_organization_id_fkey":           constant.ErrOrganizationIDNotFound,
	"segment_organization_id_fkey":             constant.ErrOrganizationIDNotFound,
	"account_type_organization_id_fkey":        constant.ErrOrganizationIDNotFound,
	"idx_account_type_unique_key_value":        constant.ErrDuplicateAccountTypeKeyValue,
	"idx_account_alias_unique":                 constant.ErrAliasUnavailability,
}

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
	if businessErr, ok := pgConstraintErrorMap[pgErr.ConstraintName]; ok {
		return pkg.ValidateBusinessError(businessErr, entityType)
	}

	return pgErr
}
