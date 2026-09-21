// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The unique index is the only serialization point between concurrent writers,
// so its violation is a caller conflict (409/0002), never a technical 500.
func TestValidatePGError_LedgerNameUniqueIndexMapsToNameConflict(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_ledger_org_name_unique",
		Detail:         `Key (organization_id, lower(name))=(0192..., alpha) already exists.`,
	}

	err := ValidatePGError(pgErr, constant.EntityLedger, "Alpha")

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)

	assert.Equal(t, constant.ErrLedgerNameConflict.Error(), conflict.Code)
	assert.Equal(t, constant.EntityLedger, conflict.EntityType)
	assert.Contains(t, conflict.Message, "Alpha",
		"the conflict message must name the ledger the caller asked for")
	assert.NotContains(t, conflict.Message, "idx_ledger_org_name_unique",
		"the client message must not carry the index name")
}

// Without a name argument the mapping must still classify as a conflict; only
// the interpolated name is missing from the message.
func TestValidatePGError_LedgerNameUniqueIndexWithoutArgs(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_ledger_org_name_unique",
	}

	err := ValidatePGError(pgErr, constant.EntityLedger)

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrLedgerNameConflict.Error(), conflict.Code)
}
