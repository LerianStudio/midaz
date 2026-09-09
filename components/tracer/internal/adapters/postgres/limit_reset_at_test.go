// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// storedLimitRow builds the row a limit created long ago leaves behind: the
// reset_at column holds the first boundary after creation and was never
// advanced.
func storedLimitRow(limitType string, storedResetAt sql.NullTime, createdAt time.Time) *LimitPostgreSQLModel {
	return &LimitPostgreSQLModel{
		ID:        uuid.New().String(),
		Name:      "daily cap",
		LimitType: limitType,
		MaxAmount: decimal.NewFromInt(1000),
		Asset:     "USD",
		Scopes:    "[]",
		Status:    "ACTIVE",
		ResetAt:   storedResetAt,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

// TestLimitToEntity_RecurringResetAtIsResolvedOnRead pins that a recurring limit
// reports the boundary its cap next refreshes at, not the one recorded when it
// was created. A limit created in January reported a January boundary for the
// rest of its life, so an operator answering "when does this customer's cap
// refresh?" quoted a moment months in the past.
func TestLimitToEntity_RecurringResetAtIsResolvedOnRead(t *testing.T) {
	createdAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		limitType     string
		storedResetAt time.Time
	}{
		{"daily", "DAILY", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)},
		{"weekly", "WEEKLY", time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)},
		{"monthly", "MONTHLY", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UTC()

			entity, err := storedLimitRow(tt.limitType, sql.NullTime{Time: tt.storedResetAt, Valid: true}, createdAt).ToEntity()
			require.NoError(t, err)
			require.NotNil(t, entity.ResetAt)

			assert.True(t, entity.ResetAt.After(before),
				"reported a boundary that has already passed: %s", entity.ResetAt.Format(time.RFC3339))
			assert.NotEqual(t, tt.storedResetAt, *entity.ResetAt,
				"reported the boundary recorded when the limit was created")
			assert.Equal(t, *model.CalculateResetAt(model.LimitType(tt.limitType), before), *entity.ResetAt)
		})
	}
}

// TestLimitToEntity_CustomResetAtIsPreserved pins that a CUSTOM limit keeps the
// boundary the operator set through customEndDate. It does not recur, so
// recomputing it would erase the operator's own end date.
func TestLimitToEntity_CustomResetAtIsPreserved(t *testing.T) {
	createdAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	stored := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	dbModel := storedLimitRow("CUSTOM", sql.NullTime{Time: stored, Valid: true}, createdAt)
	dbModel.CustomStartDate = sql.NullTime{Time: createdAt, Valid: true}
	dbModel.CustomEndDate = sql.NullTime{Time: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), Valid: true}

	entity, err := dbModel.ToEntity()
	require.NoError(t, err)
	require.NotNil(t, entity.ResetAt)
	assert.Equal(t, stored, *entity.ResetAt)
}

// TestLimitToEntity_PerTransactionHasNoResetAt pins that a per-transaction limit
// reports no boundary: it has no counter to reset.
func TestLimitToEntity_PerTransactionHasNoResetAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

	entity, err := storedLimitRow("PER_TRANSACTION", sql.NullTime{}, createdAt).ToEntity()
	require.NoError(t, err)
	assert.Nil(t, entity.ResetAt)
}

// TestLimitUsageSnapshotReportsTheResolvedBoundary pins the customer-facing end
// of the same path: the usage endpoint reports the boundary carried on the limit
// it was handed, so a limit read through ToEntity produces a live boundary.
func TestLimitUsageSnapshotReportsTheResolvedBoundary(t *testing.T) {
	createdAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	before := time.Now().UTC()

	limit, err := storedLimitRow("DAILY", sql.NullTime{Time: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), Valid: true}, createdAt).ToEntity()
	require.NoError(t, err)

	snapshot := model.NewUsageSnapshot(limit, nil)
	require.NotNil(t, snapshot.ResetAt)
	assert.True(t, snapshot.ResetAt.After(before),
		"the usage endpoint reported a boundary that has already passed: %s", snapshot.ResetAt.Format(time.RFC3339))
}
