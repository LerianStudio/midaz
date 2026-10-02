// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// validLimitDBModel returns a LimitPostgreSQLModel that converts cleanly, so
// each error-path test can corrupt exactly one field in isolation.
func validLimitDBModel(t *testing.T) LimitPostgreSQLModel {
	t.Helper()

	ft := testutil.FixedTime()

	return LimitPostgreSQLModel{
		ID:        testutil.MustDeterministicUUID(6001).String(),
		Name:      "Valid Limit",
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("100"),
		Asset:     "USD",
		Scopes:    "[]",
		Status:    "ACTIVE",
		CreatedAt: ft,
		UpdatedAt: ft,
	}
}

// TestLimitPostgreSQLModel_ToEntity_InvalidEnumsAndTimes covers the data-integrity
// guards in ToEntity that reject rows holding enum or time-of-day values the
// domain rejects. These are the branches that distinguish a corrupt DB row from
// a clean one; each must surface a descriptive error rather than a silently
// degraded entity.
func TestLimitPostgreSQLModel_ToEntity_InvalidEnumsAndTimes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(m *LimitPostgreSQLModel)
		errPart string
	}{
		{
			name:    "invalid limit type",
			mutate:  func(m *LimitPostgreSQLModel) { m.LimitType = "HOURLY" },
			errPart: "invalid limit type in database",
		},
		{
			name:    "invalid status",
			mutate:  func(m *LimitPostgreSQLModel) { m.Status = "SUSPENDED" },
			errPart: "invalid limit status in database",
		},
		{
			name: "invalid active_time_start",
			mutate: func(m *LimitPostgreSQLModel) {
				m.ActiveTimeStart = sql.NullString{String: "25:99", Valid: true}
			},
			errPart: "invalid active_time_start in database",
		},
		{
			name: "invalid active_time_end",
			mutate: func(m *LimitPostgreSQLModel) {
				m.ActiveTimeEnd = sql.NullString{String: "not-a-time", Valid: true}
			},
			errPart: "invalid active_time_end in database",
		},
		{
			name: "invalid reset_time",
			mutate: func(m *LimitPostgreSQLModel) {
				m.ResetTime = sql.NullString{String: "24:00", Valid: true}
			},
			errPart: "invalid reset_time in database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := validLimitDBModel(t)
			tt.mutate(&m)

			entity, err := m.ToEntity()
			require.Error(t, err)
			assert.Nil(t, entity)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

// TestLimitPostgreSQLModel_ToEntity_ValidTimeWindows verifies the happy path for
// the active-time-window conversion: well-formed HH:MM strings produce non-nil
// TimeOfDay pointers carrying the parsed values, proving the guard does not
// reject valid windows.
func TestLimitPostgreSQLModel_ToEntity_ValidTimeWindows(t *testing.T) {
	t.Parallel()

	m := validLimitDBModel(t)
	m.ActiveTimeStart = sql.NullString{String: "09:00", Valid: true}
	m.ActiveTimeEnd = sql.NullString{String: "17:30", Valid: true}

	entity, err := m.ToEntity()
	require.NoError(t, err)
	require.NotNil(t, entity)
	require.NotNil(t, entity.ActiveTimeStart)
	require.NotNil(t, entity.ActiveTimeEnd)
	assert.Equal(t, "09:00", entity.ActiveTimeStart.String())
	assert.Equal(t, "17:30", entity.ActiveTimeEnd.String())
}

// TestLimitPostgreSQLModel_ResetTimeMapping verifies that reset_time maps to
// and from the domain's optional ResetTime: a stored HH:MM value becomes a
// non-nil TimeOfDay, a NULL column stays nil, and FromEntity writes the
// normalized HH:MM string (or NULL when the limit has no reset time).
func TestLimitPostgreSQLModel_ResetTimeMapping(t *testing.T) {
	t.Parallel()

	t.Run("stored value reads back as the reset time", func(t *testing.T) {
		t.Parallel()

		m := validLimitDBModel(t)
		m.ResetTime = sql.NullString{String: "09:00", Valid: true}

		entity, err := m.ToEntity()
		require.NoError(t, err)
		require.NotNil(t, entity.ResetTime)
		assert.Equal(t, "09:00", entity.ResetTime.String())
	})

	t.Run("NULL reads back as no reset time", func(t *testing.T) {
		t.Parallel()

		m := validLimitDBModel(t)

		entity, err := m.ToEntity()
		require.NoError(t, err)
		assert.Nil(t, entity.ResetTime)
	})

	t.Run("entity reset time is written as HH:MM", func(t *testing.T) {
		t.Parallel()

		source := validLimitDBModel(t)

		entity, err := source.ToEntity()
		require.NoError(t, err)

		resetTime, err := model.NewTimeOfDay("9:00")
		require.NoError(t, err)

		entity.ResetTime = &resetTime

		var m LimitPostgreSQLModel
		require.NoError(t, m.FromEntity(entity))
		assert.Equal(t, sql.NullString{String: "09:00", Valid: true}, m.ResetTime)
	})

	t.Run("entity without reset time is written as NULL", func(t *testing.T) {
		t.Parallel()

		source := validLimitDBModel(t)

		entity, err := source.ToEntity()
		require.NoError(t, err)

		var m LimitPostgreSQLModel
		require.NoError(t, m.FromEntity(entity))
		assert.False(t, m.ResetTime.Valid)
	})
}
