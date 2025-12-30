// Package services provides business logic and use case implementations for the transaction domain.
// It contains command and query handlers for managing transactions, operations, and balances.
package services

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ErrInvalidOperationRouteType is thrown when operation route type is invalid
var ErrInvalidOperationRouteType = errors.New("invalid operation route type")

// ErrOperationRouteLookup is thrown when operation route lookup fails
var ErrOperationRouteLookup = errors.New("operation route lookup failed")

// ErrContextCanceled is thrown when context is canceled during operation
var ErrContextCanceled = errors.New("context canceled")

// ErrOperationRouteUpdate is thrown when operation route update fails
var ErrOperationRouteUpdate = errors.New("operation route update failed")

// ErrOperationRouteNotFound is thrown when an operation route is not found.
// This sentinel is intended for callers that want to detect "not found" via errors.Is,
// even when the underlying error is a typed business error (e.g. *pkg.EntityNotFoundError).
var ErrOperationRouteNotFound = errors.New("operation route not found")

// ErrOutboxLookup is thrown when outbox lookup fails
var ErrOutboxLookup = errors.New("outbox lookup failed")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	switch {
	case strings.Contains(pgErr.ConstraintName, "operation_route_type_check") ||
		strings.Contains(pgErr.Message, "type") && strings.Contains(pgErr.Message, "debit") && strings.Contains(pgErr.Message, "credit"):
		return pkg.ValidateBusinessError(constant.ErrInvalidOperationRouteType, entityType)
	default:
		return pkg.ValidateInternalError(pgErr, entityType)
	}
}
