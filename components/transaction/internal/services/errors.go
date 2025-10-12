package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is returned when a database query finds no matching records.
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError translates PostgreSQL errors to business-friendly error messages.
//
// This function maps PostgreSQL constraint violations to user-friendly business errors.
// Currently handles operation route type validation constraints.
//
// Parameters:
//   - pgErr: The PostgreSQL error from database operation
//   - entityType: The entity type being operated on (for error context)
//   - args: Additional arguments for error message formatting
//
// Returns:
//   - error: Business error with appropriate code and message, or original pgErr if not mapped
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	switch {
	case strings.Contains(pgErr.ConstraintName, "operation_route_type_check") ||
		strings.Contains(pgErr.Message, "type") && strings.Contains(pgErr.Message, "debit") && strings.Contains(pgErr.Message, "credit"):
		return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
	default:
		return pgErr
	}
}
