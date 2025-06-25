package services

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// ValidatePGError validate pgError and return business error
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error {
	switch pgErr.ConstraintName {
	default:
		return pgErr
	}
}
