// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// A missing column carries no constraint name, so it has to be recognized on
// SQLSTATE — otherwise it falls through as a raw driver error and reaches the
// client as an opaque 500 that names no cause.
func TestValidatePGError_UndefinedColumnMapsToSchemaMigrationPending(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "42703",
		Message: `column "holder_id" of relation "account" does not exist`,
	}

	err := ValidatePGError(pgErr, constant.EntityAccount)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable,
		"schema drift is retryable infrastructure state, not a caller mistake")

	assert.Equal(t, constant.ErrSchemaMigrationPending.Error(), unavailable.Code)
	assert.NotContains(t, unavailable.Message, "holder_id",
		"the client message must not carry the driver's column text")
}

func TestValidatePGError_ConstraintViolationStillWins(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "account_asset_code_fkey",
	}

	err := ValidatePGError(pgErr, constant.EntityAccount)

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrAssetCodeNotFound.Error(), notFound.Code)
}

func TestValidatePGError_UnknownErrorPassesThrough(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "22001"}

	assert.Equal(t, pgErr, ValidatePGError(pgErr, constant.EntityAccount))
}

func TestIsSchemaDrift(t *testing.T) {
	drift := &pgconn.PgError{Code: "42703"}

	assert.True(t, IsSchemaDrift(drift))
	assert.True(t, IsSchemaDrift(fmt.Errorf("exec failed: %w", drift)),
		"the check must survive wrapping")
	assert.False(t, IsSchemaDrift(&pgconn.PgError{Code: "23505"}))
	assert.False(t, IsSchemaDrift(errors.New("boom")))
	assert.False(t, IsSchemaDrift(nil))
}
