package services

import (
	"errors"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
	switch pgErr.ConstraintName {
	case "idx_operation_route_title_type_unique":
		return pkg.ValidateBusinessError(constant.ErrOperationRouteTitleAlreadyExists, entityType)
	default:
		return pgErr
	}
}
