// Package services provides shared utilities for the transaction service business logic layer.
//
// This package contains error handling utilities that are shared between the command
// and query packages. It bridges the gap between database-level errors (PostgreSQL)
// and business-level errors (domain errors).
package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is a sentinel error indicating an entity was not found in the database.
//
// This error is used internally by repository implementations to signal "not found" conditions.
// It is then converted to appropriate business errors (e.g., ErrTransactionIDNotFound) by
// the service layer using ValidateBusinessError.
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError converts PostgreSQL errors to business errors.
//
// This function maps PostgreSQL constraint violations and database errors to user-friendly
// business errors. It examines constraint names and error messages to determine the
// appropriate business error code.
//
// Currently handles:
//   - operation_route_type_check constraint: Invalid operation route type
//   - Type validation errors: Must be "debit" or "credit"
//
// Parameters:
//   - pgErr: PostgreSQL error from database operation
//   - entityType: Type of entity (for error context)
//   - args: Additional arguments for error formatting (unused currently)
//
// Returns:
//   - error: Business error if recognized, original pgErr otherwise
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	switch {
	case strings.Contains(pgErr.ConstraintName, "operation_route_type_check") ||
		strings.Contains(pgErr.Message, "type") && strings.Contains(pgErr.Message, "debit") && strings.Contains(pgErr.Message, "credit"):
		return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
	default:
		return pgErr
	}
}
