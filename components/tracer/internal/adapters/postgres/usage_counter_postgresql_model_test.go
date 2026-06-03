// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestUsageCounterPostgreSQLModel_FromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	dbModel := &UsageCounterPostgreSQLModel{}

	err := dbModel.FromEntity(nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}

func TestUsageCounterPostgreSQLModel_FromEntity_Valid(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(1)
	testLimitID := testutil.MustDeterministicUUID(2)
	fixedTime := testutil.FixedTime()

	entity := &model.UsageCounter{
		ID:            testID,
		LimitID:       testLimitID,
		ScopeKey:      "acct:abc-123",
		PeriodKey:     "2025-12-28",
		CurrentUsage:  decimal.RequireFromString("5"),
		LastUpdatedAt: fixedTime,
	}

	dbModel := &UsageCounterPostgreSQLModel{}
	err := dbModel.FromEntity(entity)

	require.NoError(t, err)
	assert.Equal(t, testID.String(), dbModel.ID)
	assert.Equal(t, testLimitID.String(), dbModel.LimitID)
	assert.Equal(t, "acct:abc-123", dbModel.ScopeKey)
	assert.Equal(t, "2025-12-28", dbModel.PeriodKey)
	assert.True(t, decimal.RequireFromString("5").Equal(dbModel.CurrentUsage), "CurrentUsage should be 5")
	assert.Equal(t, fixedTime, dbModel.LastUpdatedAt)
}

func TestUsageCounterPostgreSQLModel_ToEntity_Valid(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(3)
	testLimitID := testutil.MustDeterministicUUID(4)
	fixedTime := testutil.FixedTime()

	dbModel := &UsageCounterPostgreSQLModel{
		ID:            testID.String(),
		LimitID:       testLimitID.String(),
		ScopeKey:      "segment:gold",
		PeriodKey:     "2025-12",
		CurrentUsage:  decimal.RequireFromString("10"),
		LastUpdatedAt: fixedTime,
	}

	entity, err := dbModel.ToEntity()

	require.NoError(t, err)
	assert.Equal(t, testID, entity.ID)
	assert.Equal(t, testLimitID, entity.LimitID)
	assert.Equal(t, "segment:gold", entity.ScopeKey)
	assert.Equal(t, "2025-12", entity.PeriodKey)
	assert.True(t, decimal.RequireFromString("10").Equal(entity.CurrentUsage), "CurrentUsage should be 10")
	assert.Equal(t, fixedTime, entity.LastUpdatedAt)
}

func TestUsageCounterPostgreSQLModel_ToEntity_InvalidID(t *testing.T) {
	t.Parallel()

	dbModel := &UsageCounterPostgreSQLModel{
		ID:      "not-a-uuid",
		LimitID: testutil.MustDeterministicUUID(5).String(),
	}

	_, err := dbModel.ToEntity()

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid UsageCounter ID")
}

func TestUsageCounterPostgreSQLModel_ToEntity_InvalidLimitID(t *testing.T) {
	t.Parallel()

	dbModel := &UsageCounterPostgreSQLModel{
		ID:      testutil.MustDeterministicUUID(6).String(),
		LimitID: "not-a-uuid",
	}

	_, err := dbModel.ToEntity()

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid LimitID")
}

func TestUsageCounterPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		usage string
	}{
		{name: "fractional", usage: "7.50"},
		{name: "zero", usage: "0"},
		{name: "small precision", usage: "0.01"},
		{name: "large value", usage: "999999999.99"},
		{name: "negative", usage: "-100.50"},
		{name: "high precision", usage: "123.456789"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testID := testutil.MustDeterministicUUID(int64(7 + i*2))
			testLimitID := testutil.MustDeterministicUUID(int64(8 + i*2))
			fixedTime := testutil.FixedTime()

			original := &model.UsageCounter{
				ID:            testID,
				LimitID:       testLimitID,
				ScopeKey:      "portfolio:xyz",
				PeriodKey:     "2025-12-28",
				CurrentUsage:  decimal.RequireFromString(tt.usage),
				LastUpdatedAt: fixedTime,
			}

			dbModel := &UsageCounterPostgreSQLModel{}
			err := dbModel.FromEntity(original)
			require.NoError(t, err)

			restored, err := dbModel.ToEntity()
			require.NoError(t, err)

			assert.Equal(t, original.ID, restored.ID)
			assert.Equal(t, original.LimitID, restored.LimitID)
			assert.Equal(t, original.ScopeKey, restored.ScopeKey)
			assert.Equal(t, original.PeriodKey, restored.PeriodKey)
			assert.True(t, original.CurrentUsage.Equal(restored.CurrentUsage),
				"CurrentUsage mismatch: want %s, got %s", original.CurrentUsage, restored.CurrentUsage)
			assert.Equal(t, original.LastUpdatedAt, restored.LastUpdatedAt)
		})
	}
}
