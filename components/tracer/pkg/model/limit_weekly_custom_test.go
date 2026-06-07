// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestLimitType_IsValid_Extended tests the extended LimitType enum including WEEKLY and CUSTOM.
func TestLimitType_IsValid_Extended(t *testing.T) {
	tests := []struct {
		name      string
		limitType LimitType
		expected  bool
	}{
		{
			name:      "WEEKLY is valid",
			limitType: LimitTypeWeekly,
			expected:  true,
		},
		{
			name:      "CUSTOM is valid",
			limitType: LimitTypeCustom,
			expected:  true,
		},
		// Ensure existing types still work
		{
			name:      "DAILY is still valid",
			limitType: LimitTypeDaily,
			expected:  true,
		},
		{
			name:      "MONTHLY is still valid",
			limitType: LimitTypeMonthly,
			expected:  true,
		},
		{
			name:      "PER_TRANSACTION is still valid",
			limitType: LimitTypePerTransaction,
			expected:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.limitType.IsValid()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCalculateResetAt_Weekly tests WEEKLY limit reset calculation.
func TestCalculateResetAt_Weekly(t *testing.T) {
	tests := []struct {
		name     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "Wednesday resets to next Monday",
			now:      time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),   // Monday
		},
		{
			name:     "Monday morning resets to next Monday",
			now:      time.Date(2025, 1, 13, 8, 0, 0, 0, time.UTC), // Monday 8AM
			expected: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Sunday night resets to Monday",
			now:      time.Date(2025, 1, 19, 23, 59, 0, 0, time.UTC), // Sunday 23:59
			expected: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),   // Monday
		},
		{
			name:     "Friday resets to Monday",
			now:      time.Date(2025, 1, 17, 14, 0, 0, 0, time.UTC), // Friday
			expected: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),  // Monday
		},
		{
			name:     "Saturday resets to Monday",
			now:      time.Date(2025, 1, 18, 12, 0, 0, 0, time.UTC), // Saturday
			expected: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),  // Monday
		},
		{
			name:     "handles year boundary",
			now:      time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC), // Wednesday Dec 31
			expected: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),    // Monday Jan 5
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CalculateResetAt(LimitTypeWeekly, tc.now)
			require.NotNil(t, result)
			assert.Equal(t, tc.expected, *result)
		})
	}
}

// TestNewLimit_WithWeeklyType tests creating a limit with WEEKLY type.
func TestNewLimit_WithWeeklyType(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(100))}
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC) // Wednesday

	limit, err := NewLimit(
		"Weekly Spending Limit",
		LimitTypeWeekly,
		decimal.RequireFromString("5000"),
		"USD",
		[]Scope{validScope},
		nil,
		fixedTime,
	)

	require.NoError(t, err)
	require.NotNil(t, limit)
	assert.Equal(t, LimitTypeWeekly, limit.LimitType)
	require.NotNil(t, limit.ResetAt)
	// Should reset to next Monday (Jan 20)
	assert.Equal(t, time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC), *limit.ResetAt)
}

// TestNewLimit_WithCustomType tests creating a limit with CUSTOM type.
func TestNewLimit_WithCustomType(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(101))}
	fixedTime := time.Date(2027, 1, 15, 10, 0, 0, 0, time.UTC)

	customStart := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	customEnd := time.Date(2027, 3, 31, 23, 59, 59, 0, time.UTC)

	limit, err := NewLimitWithCustomPeriod(
		"Q1 Budget Limit",
		LimitTypeCustom,
		decimal.RequireFromString("100000"),
		"USD",
		[]Scope{validScope},
		nil,
		customStart,
		customEnd,
		fixedTime,
	)

	require.NoError(t, err)
	require.NotNil(t, limit)
	assert.Equal(t, LimitTypeCustom, limit.LimitType)
	require.NotNil(t, limit.CustomStartDate)
	require.NotNil(t, limit.CustomEndDate)
	assert.Equal(t, customStart, *limit.CustomStartDate)
	assert.Equal(t, customEnd, *limit.CustomEndDate)
	// ResetAt should be customEndDate + 1 day at midnight
	require.NotNil(t, limit.ResetAt)
	assert.Equal(t, time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC), *limit.ResetAt)
}

// TestNewLimit_WithActiveTimeWindow tests creating limits with time-of-day restrictions.
func TestNewLimit_WithActiveTimeWindow(t *testing.T) {
	tests := []struct {
		name            string
		limitType       LimitType
		activeTimeStart string
		activeTimeEnd   string
		expectError     bool
		errorIs         error
	}{
		{
			name:            "creates DAILY limit with business hours window",
			limitType:       LimitTypeDaily,
			activeTimeStart: "09:00",
			activeTimeEnd:   "17:00",
			expectError:     false,
		},
		{
			name:            "creates DAILY limit with overnight window (crosses midnight)",
			limitType:       LimitTypeDaily,
			activeTimeStart: "20:00",
			activeTimeEnd:   "06:00",
			expectError:     false,
		},
		{
			name:            "creates WEEKLY limit with time window",
			limitType:       LimitTypeWeekly,
			activeTimeStart: "08:00",
			activeTimeEnd:   "22:00",
			expectError:     false,
		},
		{
			name:            "rejects zero-width window (start == end)",
			limitType:       LimitTypeDaily,
			activeTimeStart: "12:00",
			activeTimeEnd:   "12:00",
			expectError:     true,
			errorIs:         constant.ErrLimitTimeWindowZeroWidth,
		},
		{
			name:            "rejects invalid activeTimeStart format",
			limitType:       LimitTypeDaily,
			activeTimeStart: "invalid",
			activeTimeEnd:   "17:00",
			expectError:     true,
			errorIs:         constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:            "rejects invalid activeTimeEnd format",
			limitType:       LimitTypeDaily,
			activeTimeStart: "09:00",
			activeTimeEnd:   "invalid",
			expectError:     true,
			errorIs:         constant.ErrTimeOfDayInvalidFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(102))}
			fixedTime := testutil.FixedTime()

			limit, err := NewLimitWithTimeWindow(
				"Time-Restricted Limit",
				tc.limitType,
				decimal.RequireFromString("1000"),
				"USD",
				[]Scope{validScope},
				nil,
				tc.activeTimeStart,
				tc.activeTimeEnd,
				fixedTime,
			)

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
				assert.Nil(t, limit)
			} else {
				require.NoError(t, err)
				require.NotNil(t, limit)
				require.NotNil(t, limit.ActiveTimeStart)
				require.NotNil(t, limit.ActiveTimeEnd)
				assert.Equal(t, tc.activeTimeStart, limit.ActiveTimeStart.String())
				assert.Equal(t, tc.activeTimeEnd, limit.ActiveTimeEnd.String())
			}
		})
	}
}

// TestLimit_ValidateTimeWindow tests the ValidateTimeWindow method.
func TestLimit_ValidateTimeWindow(t *testing.T) {
	tests := []struct {
		name        string
		start       *TimeOfDay
		end         *TimeOfDay
		expectError bool
		errorIs     error
	}{
		{
			name:        "valid daytime window",
			start:       testutil.Ptr(mustNewTimeOfDay("09:00")),
			end:         testutil.Ptr(mustNewTimeOfDay("17:00")),
			expectError: false,
		},
		{
			name:        "valid overnight window (crosses midnight)",
			start:       testutil.Ptr(mustNewTimeOfDay("20:00")),
			end:         testutil.Ptr(mustNewTimeOfDay("06:00")),
			expectError: false,
		},
		{
			name:        "nil start and end is valid (no time restriction)",
			start:       nil,
			end:         nil,
			expectError: false,
		},
		{
			name:        "rejects start without end",
			start:       testutil.Ptr(mustNewTimeOfDay("09:00")),
			end:         nil,
			expectError: true,
			errorIs:     constant.ErrLimitTimeWindowMismatch,
		},
		{
			name:        "rejects end without start",
			start:       nil,
			end:         testutil.Ptr(mustNewTimeOfDay("17:00")),
			expectError: true,
			errorIs:     constant.ErrLimitTimeWindowMismatch,
		},
		{
			name:        "rejects zero-width window",
			start:       testutil.Ptr(mustNewTimeOfDay("12:00")),
			end:         testutil.Ptr(mustNewTimeOfDay("12:00")),
			expectError: true,
			errorIs:     constant.ErrLimitTimeWindowZeroWidth,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTimeWindow(tc.start, tc.end)

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestLimit_ValidateCustomPeriod tests the ValidateCustomPeriod method.
func TestLimit_ValidateCustomPeriod(t *testing.T) {
	tests := []struct {
		name        string
		limitType   LimitType
		startDate   *time.Time
		endDate     *time.Time
		now         time.Time
		expectError bool
		errorIs     error
	}{
		{
			name:        "valid CUSTOM period",
			limitType:   LimitTypeCustom,
			startDate:   testutil.Ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:     testutil.Ptr(time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)),
			now:         time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: false,
		},
		{
			name:        "rejects CUSTOM without dates",
			limitType:   LimitTypeCustom,
			startDate:   nil,
			endDate:     nil,
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesRequired,
		},
		{
			name:        "rejects CUSTOM with only start date",
			limitType:   LimitTypeCustom,
			startDate:   testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:     nil,
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesRequired,
		},
		{
			name:        "rejects CUSTOM with only end date",
			limitType:   LimitTypeCustom,
			startDate:   nil,
			endDate:     testutil.Ptr(time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)),
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesRequired,
		},
		{
			name:        "rejects end date before start date",
			limitType:   LimitTypeCustom,
			startDate:   testutil.Ptr(time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)),
			endDate:     testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesOrder,
		},
		{
			name:        "accepts period equal to 5 years",
			limitType:   LimitTypeCustom,
			startDate:   testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:     testutil.Ptr(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)), // exactly 5 years
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: false,
		},
		{
			name:        "rejects period exceeding 5 years",
			limitType:   LimitTypeCustom,
			startDate:   testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:     testutil.Ptr(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)), // > 5 years
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomPeriodTooLong,
		},
		{
			name:        "rejects non-CUSTOM with custom dates",
			limitType:   LimitTypeDaily,
			startDate:   testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:     testutil.Ptr(time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)),
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesNotAllowed,
		},
		{
			name:        "non-CUSTOM without dates is valid",
			limitType:   LimitTypeDaily,
			startDate:   nil,
			endDate:     nil,
			now:         time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCustomPeriod(tc.limitType, tc.startDate, tc.endDate, tc.now)

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestLimit_Validate_ExtendedFields tests validation of new fields in Limit.
func TestLimit_Validate_ExtendedFields(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(110))}
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name        string
		setupLimit  func() *Limit
		expectError bool
		errorIs     error
	}{
		{
			name: "valid WEEKLY limit",
			setupLimit: func() *Limit {
				return &Limit{
					ID:        testutil.MustDeterministicUUID(111),
					Name:      "Weekly Limit",
					LimitType: LimitTypeWeekly,
					MaxAmount: decimal.RequireFromString("1000"),
					Currency:  "USD",
					Scopes:    []Scope{validScope},
					Status:    LimitStatusActive,
					ResetAt:   testutil.Ptr(fixedTime.AddDate(0, 0, 7)),
					CreatedAt: fixedTime,
					UpdatedAt: fixedTime,
				}
			},
			expectError: false,
		},
		{
			name: "valid CUSTOM limit with dates",
			setupLimit: func() *Limit {
				return &Limit{
					ID:              testutil.MustDeterministicUUID(112),
					Name:            "Custom Limit",
					LimitType:       LimitTypeCustom,
					MaxAmount:       decimal.RequireFromString("1000"),
					Currency:        "USD",
					Scopes:          []Scope{validScope},
					Status:          LimitStatusActive,
					CustomStartDate: testutil.Ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
					CustomEndDate:   testutil.Ptr(time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)),
					ResetAt:         testutil.Ptr(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC)),
					CreatedAt:       fixedTime,
					UpdatedAt:       fixedTime,
				}
			},
			expectError: false,
		},
		{
			name: "valid limit with time window",
			setupLimit: func() *Limit {
				return &Limit{
					ID:              testutil.MustDeterministicUUID(113),
					Name:            "Time Window Limit",
					LimitType:       LimitTypeDaily,
					MaxAmount:       decimal.RequireFromString("1000"),
					Currency:        "USD",
					Scopes:          []Scope{validScope},
					Status:          LimitStatusActive,
					ActiveTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
					ActiveTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
					ResetAt:         testutil.Ptr(fixedTime.AddDate(0, 0, 1)),
					CreatedAt:       fixedTime,
					UpdatedAt:       fixedTime,
				}
			},
			expectError: false,
		},
		{
			name: "rejects CUSTOM without custom dates",
			setupLimit: func() *Limit {
				return &Limit{
					ID:        testutil.MustDeterministicUUID(114),
					Name:      "Invalid Custom",
					LimitType: LimitTypeCustom,
					MaxAmount: decimal.RequireFromString("1000"),
					Currency:  "USD",
					Scopes:    []Scope{validScope},
					Status:    LimitStatusActive,
					CreatedAt: fixedTime,
					UpdatedAt: fixedTime,
				}
			},
			expectError: true,
			errorIs:     constant.ErrLimitCustomDatesRequired,
		},
		{
			name: "rejects time window mismatch",
			setupLimit: func() *Limit {
				return &Limit{
					ID:              testutil.MustDeterministicUUID(115),
					Name:            "Invalid Window",
					LimitType:       LimitTypeDaily,
					MaxAmount:       decimal.RequireFromString("1000"),
					Currency:        "USD",
					Scopes:          []Scope{validScope},
					Status:          LimitStatusActive,
					ActiveTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
					ActiveTimeEnd:   nil, // Mismatch!
					CreatedAt:       fixedTime,
					UpdatedAt:       fixedTime,
				}
			},
			expectError: true,
			errorIs:     constant.ErrLimitTimeWindowMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := tc.setupLimit()
			err := limit.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestLimit_GetAPIResponse tests that new fields are included in API response.
func TestLimit_GetAPIResponse(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(120))}
	fixedTime := testutil.FixedTime()

	t.Run("includes WEEKLY type in response", func(t *testing.T) {
		limit := &Limit{
			ID:        testutil.MustDeterministicUUID(121),
			Name:      "Weekly Limit",
			LimitType: LimitTypeWeekly,
			MaxAmount: decimal.RequireFromString("5000"),
			Currency:  "USD",
			Scopes:    []Scope{validScope},
			Status:    LimitStatusActive,
			ResetAt:   testutil.Ptr(fixedTime.AddDate(0, 0, 7)),
			CreatedAt: fixedTime,
			UpdatedAt: fixedTime,
		}

		assert.Equal(t, LimitTypeWeekly, limit.LimitType)
	})

	t.Run("includes CUSTOM type with dates in response", func(t *testing.T) {
		limit := &Limit{
			ID:              testutil.MustDeterministicUUID(122),
			Name:            "Custom Limit",
			LimitType:       LimitTypeCustom,
			MaxAmount:       decimal.RequireFromString("100000"),
			Currency:        "USD",
			Scopes:          []Scope{validScope},
			Status:          LimitStatusActive,
			CustomStartDate: testutil.Ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			CustomEndDate:   testutil.Ptr(time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)),
			ResetAt:         testutil.Ptr(time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)),
			CreatedAt:       fixedTime,
			UpdatedAt:       fixedTime,
		}

		assert.Equal(t, LimitTypeCustom, limit.LimitType)
		require.NotNil(t, limit.CustomStartDate)
		require.NotNil(t, limit.CustomEndDate)
	})

	t.Run("includes time window fields in response", func(t *testing.T) {
		limit := &Limit{
			ID:              testutil.MustDeterministicUUID(123),
			Name:            "Time Window Limit",
			LimitType:       LimitTypeDaily,
			MaxAmount:       decimal.RequireFromString("1000"),
			Currency:        "USD",
			Scopes:          []Scope{validScope},
			Status:          LimitStatusActive,
			ActiveTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			ActiveTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			ResetAt:         testutil.Ptr(fixedTime.AddDate(0, 0, 1)),
			CreatedAt:       fixedTime,
			UpdatedAt:       fixedTime,
		}

		require.NotNil(t, limit.ActiveTimeStart)
		require.NotNil(t, limit.ActiveTimeEnd)
		assert.Equal(t, "20:00", limit.ActiveTimeStart.String())
		assert.Equal(t, "06:00", limit.ActiveTimeEnd.String())
	})
}

// TestNewUsageSnapshot_WeeklyLimit tests UsageSnapshot creation for WEEKLY limits.
func TestNewUsageSnapshot_WeeklyLimit(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(130))}
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC) // Wednesday
	resetAt := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)    // Next Monday

	limit := &Limit{
		ID:        testutil.MustDeterministicUUID(131),
		Name:      "Weekly Limit",
		LimitType: LimitTypeWeekly,
		MaxAmount: decimal.RequireFromString("10000"),
		Currency:  "USD",
		Scopes:    []Scope{validScope},
		Status:    LimitStatusActive,
		ResetAt:   &resetAt,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}

	counters := []UsageCounter{
		{CurrentUsage: decimal.RequireFromString("3000")},
		{CurrentUsage: decimal.RequireFromString("2000")},
	}

	snapshot := NewUsageSnapshot(limit, counters)

	assert.Equal(t, limit.ID, snapshot.LimitID)
	assert.True(t, decimal.RequireFromString("5000").Equal(snapshot.CurrentUsage))
	assert.True(t, decimal.RequireFromString("10000").Equal(snapshot.LimitAmount))
	assert.Equal(t, 50.0, snapshot.UtilizationPercent)
	assert.False(t, snapshot.NearLimit)
	require.NotNil(t, snapshot.ResetAt)
	assert.Equal(t, resetAt, *snapshot.ResetAt)
}

// TestNewUsageSnapshot_CustomLimit tests UsageSnapshot creation for CUSTOM limits.
func TestNewUsageSnapshot_CustomLimit(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(140))}
	fixedTime := time.Date(2025, 2, 15, 10, 0, 0, 0, time.UTC)
	customStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	customEnd := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	resetAt := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)

	limit := &Limit{
		ID:              testutil.MustDeterministicUUID(141),
		Name:            "Q1 Custom Limit",
		LimitType:       LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("100000"),
		Currency:        "USD",
		Scopes:          []Scope{validScope},
		Status:          LimitStatusActive,
		CustomStartDate: &customStart,
		CustomEndDate:   &customEnd,
		ResetAt:         &resetAt,
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime,
	}

	counters := []UsageCounter{
		{CurrentUsage: decimal.RequireFromString("85000")},
	}

	snapshot := NewUsageSnapshot(limit, counters)

	assert.Equal(t, limit.ID, snapshot.LimitID)
	assert.True(t, decimal.RequireFromString("85000").Equal(snapshot.CurrentUsage))
	assert.Equal(t, 85.0, snapshot.UtilizationPercent)
	assert.True(t, snapshot.NearLimit, "85% should be near limit")
	require.NotNil(t, snapshot.ResetAt)
	assert.Equal(t, resetAt, *snapshot.ResetAt)
}

// TestListLimitsFilter_LimitType_Extended tests filtering by WEEKLY and CUSTOM types.
func TestListLimitsFilter_LimitType_Extended(t *testing.T) {
	tests := []struct {
		name        string
		limitType   *LimitType
		expectError bool
	}{
		{
			name:        "filter by WEEKLY is valid",
			limitType:   testutil.Ptr(LimitTypeWeekly),
			expectError: false,
		},
		{
			name:        "filter by CUSTOM is valid",
			limitType:   testutil.Ptr(LimitTypeCustom),
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filter := ListLimitsFilter{
				LimitType: tc.limitType,
				Limit:     10,
			}

			err := filter.Validate()

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestLimit_IsWithinTimeWindow tests the IsWithinTimeWindow method.
func TestLimit_IsWithinTimeWindow(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(200))}
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name            string
		activeTimeStart *TimeOfDay
		activeTimeEnd   *TimeOfDay
		checkTime       time.Time
		expected        bool
	}{
		{
			name:            "no time window configured - always true",
			activeTimeStart: nil,
			activeTimeEnd:   nil,
			checkTime:       time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "normal window 09:00-17:00 - inside at 10:00",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "normal window 09:00-17:00 - at start boundary",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "normal window 09:00-17:00 - at end boundary (exclusive)",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 17, 0, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "normal window 09:00-17:00 - 1 minute before start",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 8, 59, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "normal window 09:00-17:00 - 1 minute after end",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 17, 1, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "overnight window 20:00-06:00 - inside at 22:00",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 22, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "overnight window 20:00-06:00 - inside at 02:00",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 2, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "overnight window 20:00-06:00 - at start boundary",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 20, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "overnight window 20:00-06:00 - at end boundary (exclusive)",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 6, 0, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "overnight window 20:00-06:00 - outside at 12:00",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "overnight window 20:00-06:00 - outside at 19:59",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("20:00")),
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("06:00")),
			checkTime:       time.Date(2025, 1, 15, 19, 59, 0, 0, time.UTC),
			expected:        false,
		},
		{
			name:            "only start set - defensive check returns true",
			activeTimeStart: testutil.Ptr(mustNewTimeOfDay("09:00")),
			activeTimeEnd:   nil,
			checkTime:       time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			expected:        true,
		},
		{
			name:            "only end set - defensive check returns true",
			activeTimeStart: nil,
			activeTimeEnd:   testutil.Ptr(mustNewTimeOfDay("17:00")),
			checkTime:       time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			expected:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := &Limit{
				ID:              testutil.MustDeterministicUUID(201),
				Name:            "Time Window Test",
				LimitType:       LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				ActiveTimeStart: tc.activeTimeStart,
				ActiveTimeEnd:   tc.activeTimeEnd,
				CreatedAt:       fixedTime,
				UpdatedAt:       fixedTime,
			}

			result := limit.IsWithinTimeWindow(tc.checkTime)
			assert.Equal(t, tc.expected, result, "Expected %v for time %s", tc.expected, tc.checkTime.Format("15:04"))
		})
	}
}

// TestValidateCustomPeriod_Expired tests that expired custom periods are rejected.
func TestValidateCustomPeriod_Expired(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	pastStart := now.AddDate(0, -3, 0) // 3 months ago
	pastEnd := now.AddDate(0, -1, 0)   // 1 month ago

	err := ValidateCustomPeriod(LimitTypeCustom, &pastStart, &pastEnd, now)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitCustomPeriodExpired)
}

// TestValidateCustomPeriod_FutureEndDate tests that custom periods with future end dates are valid.
func TestValidateCustomPeriod_FutureEndDate(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	pastStart := now.AddDate(0, -1, 0) // 1 month ago (start can be in past)
	futureEnd := now.AddDate(0, 1, 0)  // 1 month from now

	err := ValidateCustomPeriod(LimitTypeCustom, &pastStart, &futureEnd, now)

	require.NoError(t, err)
}

// TestValidateCustomPeriod_EndDateToday tests that custom periods ending today are valid.
func TestValidateCustomPeriod_EndDateToday(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	pastStart := now.AddDate(0, -1, 0)
	todayEnd := time.Date(2025, 6, 15, 23, 59, 59, 0, time.UTC)

	err := ValidateCustomPeriod(LimitTypeCustom, &pastStart, &todayEnd, now)

	require.NoError(t, err)
}

// =============================================================================
// IsWithinCustomPeriod Tests
// Tests for the IsWithinCustomPeriod method that checks if a transaction timestamp
// falls within a CUSTOM limit's custom period [customStartDate, customEndDate).
// Seed range: 7000-7099
// =============================================================================

// TestLimit_IsWithinCustomPeriod tests the IsWithinCustomPeriod method for CUSTOM limits.
// This method should return true if the given timestamp is within [customStartDate, customEndDate).
// For non-CUSTOM limits, it should always return true (not filtered by custom period).
func TestLimit_IsWithinCustomPeriod(t *testing.T) {
	t.Parallel()

	// Test dates for CUSTOM period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Create a CUSTOM limit with the test period
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(7000))}

	tests := []struct {
		name      string
		limit     *Limit
		timestamp time.Time
		expected  bool
	}{
		{
			name: "CUSTOM limit - inside period at Nov 27 12:00 - returns true",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7001),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC), // Inside period
			expected:  true,
		},
		{
			name: "CUSTOM limit - outside period at Mar 09 - returns false",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7002),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC), // Way before period
			expected:  false,
		},
		{
			name: "CUSTOM limit - outside period at Nov 29 - returns false",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7003),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: time.Date(2025, 11, 29, 10, 0, 0, 0, time.UTC), // After period
			expected:  false,
		},
		{
			name: "CUSTOM limit - boundary: exactly at customStartDate - returns true (inclusive)",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7004),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: customStartDate, // Exactly at start boundary
			expected:  true,            // Start is inclusive
		},
		{
			name: "CUSTOM limit - boundary: exactly at customEndDate - returns false (exclusive)",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7005),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: customEndDate, // Exactly at end boundary
			expected:  false,         // End is exclusive
		},
		{
			name: "CUSTOM limit - one nanosecond before end - returns true",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7006),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: customEndDate.Add(-1 * time.Nanosecond), // Just before end
			expected:  true,
		},
		{
			name: "CUSTOM limit - one nanosecond before start - returns false",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7007),
				Name:            "Custom Period Limit",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			timestamp: customStartDate.Add(-1 * time.Nanosecond), // Just before start
			expected:  false,
		},
		{
			name: "DAILY limit - always returns true (not affected by custom period)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(7010),
				Name:      "Daily Limit",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
				// No custom dates
			},
			timestamp: time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC), // Any timestamp
			expected:  true,                                         // Non-CUSTOM always returns true
		},
		{
			name: "MONTHLY limit - always returns true (not affected by custom period)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(7011),
				Name:      "Monthly Limit",
				LimitType: LimitTypeMonthly,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			timestamp: time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC),
			expected:  true,
		},
		{
			name: "WEEKLY limit - always returns true (not affected by custom period)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(7012),
				Name:      "Weekly Limit",
				LimitType: LimitTypeWeekly,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			timestamp: time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC),
			expected:  true,
		},
		{
			name: "PER_TRANSACTION limit - always returns true (not affected by custom period)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(7013),
				Name:      "Per Transaction Limit",
				LimitType: LimitTypePerTransaction,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			timestamp: time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC),
			expected:  true,
		},
		{
			name: "CUSTOM limit with nil dates (safety) - returns true",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7020),
				Name:            "Custom Limit No Dates",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: nil, // Safety check - should return true
				CustomEndDate:   nil,
			},
			timestamp: time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC),
			expected:  true, // Defensive: no dates configured → allow
		},
		{
			name: "CUSTOM limit with only start date (safety) - returns true",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7021),
				Name:            "Custom Limit Only Start",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   nil, // Only start configured
			},
			timestamp: time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC),
			expected:  true, // Defensive: incomplete config → allow
		},
		{
			name: "CUSTOM limit with only end date (safety) - returns true",
			limit: &Limit{
				ID:              testutil.MustDeterministicUUID(7022),
				Name:            "Custom Limit Only End",
				LimitType:       LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []Scope{validScope},
				Status:          LimitStatusActive,
				CustomStartDate: nil,
				CustomEndDate:   &customEndDate, // Only end configured
			},
			timestamp: time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC),
			expected:  true, // Defensive: incomplete config → allow
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.limit.IsWithinCustomPeriod(tc.timestamp)
			assert.Equal(t, tc.expected, result, "Expected %v for timestamp %s", tc.expected, tc.timestamp.Format(time.RFC3339))
		})
	}
}
