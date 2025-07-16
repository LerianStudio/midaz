package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	switch {
	case strings.Contains(pgErr.ConstraintName, "operation_route_type_check") ||
		strings.Contains(pgErr.Message, "type") && strings.Contains(pgErr.Message, "debit") && strings.Contains(pgErr.Message, "credit"):
		return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
	default:
		return pgErr
	}
}
