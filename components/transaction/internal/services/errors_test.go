package services

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestValidatePGError(t *testing.T) {
	t.Parallel()

	t.Run("maps operation route type check constraint to business error", func(t *testing.T) {
		t.Parallel()

		err := ValidatePGError(&pgconn.PgError{ConstraintName: "operation_route_type_check"}, "OperationRoute")
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "debit")
		assert.Contains(t, err.Error(), "credit")
	})

	t.Run("returns original pg error for unrelated errors", func(t *testing.T) {
		t.Parallel()

		pgErr := &pgconn.PgError{Message: "unrelated postgres error"}
		err := ValidatePGError(pgErr, "OperationRoute")
		assert.Same(t, pgErr, err)
	})
}
