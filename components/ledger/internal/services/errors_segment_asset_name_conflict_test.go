// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The segment unique index is the only serialization point between concurrent
// writers, so its violation is the same caller conflict (409/0015) the lookup
// returns, never a technical 500.
func TestValidatePGError_SegmentNameUniqueIndexMapsToDuplicateSegmentName(t *testing.T) {
	ledgerID := uuid.MustParse("01920000-0000-7000-8000-000000000001")
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_segment_ledger_name_unique",
		Detail:         `Key (organization_id, ledger_id, lower(name))=(0192..., 0192..., retail) already exists.`,
	}

	err := ValidatePGError(pgErr, constant.EntitySegment, "Retail", ledgerID)

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)

	assert.Equal(t, constant.ErrDuplicateSegmentName.Error(), conflict.Code)
	assert.Equal(t, constant.EntitySegment, conflict.EntityType)
	assert.Contains(t, conflict.Message, "Retail",
		"the conflict message must name the segment the caller asked for")
	assert.Contains(t, conflict.Message, ledgerID.String(),
		"the conflict message must name the ledger")
	assert.NotContains(t, conflict.Message, "%!v(MISSING)",
		"every placeholder of the message template must be filled")
	assert.NotContains(t, conflict.Message, "idx_segment_ledger_name_unique",
		"the client message must not carry the index name")
}

// Both asset unique indexes surface as the single name-or-code conflict
// (409/0003) the lookup returns.
func TestValidatePGError_AssetUniqueIndexesMapToNameOrCodeDuplicate(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
	}{
		{name: "name index", constraint: "idx_asset_ledger_name_unique"},
		{name: "code index", constraint: "idx_asset_ledger_code_unique"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{
				Code:           "23505",
				ConstraintName: tc.constraint,
			}

			err := ValidatePGError(pgErr, constant.EntityAsset)

			var conflict pkg.EntityConflictError
			require.ErrorAs(t, err, &conflict)

			assert.Equal(t, constant.ErrAssetNameOrCodeDuplicate.Error(), conflict.Code)
			assert.Equal(t, constant.EntityAsset, conflict.EntityType)
			assert.NotContains(t, conflict.Message, tc.constraint,
				"the client message must not carry the index name")
		})
	}
}
