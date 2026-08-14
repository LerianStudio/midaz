// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

func TestTransactionValidationFilters_Validate(t *testing.T) {
	tests := []struct {
		name      string
		filters   *TransactionValidationFilters
		expectErr bool
		errorMsg  string
	}{
		{
			name: "success - valid filters with date range",
			filters: &TransactionValidationFilters{
				StartDate: testutil.DefaultTestTime.Add(-7 * 24 * time.Hour),
				EndDate:   testutil.DefaultTestTime,
				Limit:     100,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with decision filter",
			filters: &TransactionValidationFilters{
				Decision: func() *Decision { d := DecisionDeny; return &d }(),
				Limit:    50,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with accountID",
			filters: &TransactionValidationFilters{
				AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
				Limit:     100,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with matchedRuleID",
			filters: &TransactionValidationFilters{
				MatchedRuleID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2)),
				Limit:         100,
			},
			expectErr: false,
		},
		{
			name:      "success - empty filters (uses defaults)",
			filters:   &TransactionValidationFilters{},
			expectErr: false,
		},
		{
			name: "success - limit equals maximum",
			filters: &TransactionValidationFilters{
				Limit: MaxTransactionValidationFilterLimit,
			},
			expectErr: false,
		},
		{
			name: "error - limit exceeds maximum by one (boundary)",
			filters: &TransactionValidationFilters{
				Limit: MaxTransactionValidationFilterLimit + 1,
			},
			expectErr: true,
			errorMsg:  "limit cannot exceed 1000",
		},
		{
			name: "error - endDate before startDate",
			filters: &TransactionValidationFilters{
				StartDate: testutil.DefaultTestTime,
				EndDate:   testutil.DefaultTestTime.Add(-7 * 24 * time.Hour),
				Limit:     100,
			},
			expectErr: true,
			errorMsg:  "end_date must be on or after start_date",
		},
		{
			name: "error - limit exceeds maximum",
			filters: &TransactionValidationFilters{
				Limit: 10000,
			},
			expectErr: true,
			errorMsg:  "limit cannot exceed 1000",
		},
		{
			name: "error - negative limit",
			filters: &TransactionValidationFilters{
				Limit: -1,
			},
			expectErr: true,
			errorMsg:  "limit cannot be negative",
		},
		{
			name: "error - invalid sortBy",
			filters: &TransactionValidationFilters{
				Limit:  100,
				SortBy: "invalid_field",
			},
			expectErr: true,
			errorMsg:  "invalid sort_by field",
		},
		{
			name: "error - invalid sortOrder",
			filters: &TransactionValidationFilters{
				Limit:     100,
				SortOrder: "INVALID",
			},
			expectErr: true,
			errorMsg:  "sort_order must be ASC or DESC",
		},
		{
			name: "accepts snake_case sort field created_at",
			filters: &TransactionValidationFilters{
				Limit:  100,
				SortBy: "created_at",
			},
			expectErr: false,
		},
		{
			name: "accepts snake_case sort field processing_time_ms",
			filters: &TransactionValidationFilters{
				Limit:  100,
				SortBy: "processing_time_ms",
			},
			expectErr: false,
		},
		{
			name: "rejects camelCase sort field createdAt",
			filters: &TransactionValidationFilters{
				Limit:  100,
				SortBy: "createdAt",
			},
			expectErr: true,
			errorMsg:  "invalid sort_by field",
		},
		{
			name: "success - valid sortBy and sortOrder",
			filters: &TransactionValidationFilters{
				Limit:     100,
				SortBy:    "created_at",
				SortOrder: "ASC",
			},
			expectErr: false,
		},
		{
			name: "success - valid transactionType filter",
			filters: &TransactionValidationFilters{
				TransactionType: func() *TransactionType { t := TransactionTypeCard; return &t }(),
				Limit:           100,
			},
			expectErr: false,
		},
		{
			name: "error - invalid transactionType filter",
			filters: &TransactionValidationFilters{
				TransactionType: func() *TransactionType { t := TransactionType("INVALID"); return &t }(),
				Limit:           100,
			},
			expectErr: true,
			errorMsg:  "invalid transaction_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.filters.Validate()

			// Assert
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTransactionValidationFilters_SetDefaults(t *testing.T) {
	tests := []struct {
		name           string
		filters        *TransactionValidationFilters
		expectedLimit  int
		checkDateRange bool
	}{
		{
			name:           "sets default limit when zero",
			filters:        &TransactionValidationFilters{},
			expectedLimit:  DefaultTransactionValidationFilterLimit,
			checkDateRange: true,
		},
		{
			name: "preserves existing limit",
			filters: &TransactionValidationFilters{
				Limit: 50,
			},
			expectedLimit:  50,
			checkDateRange: true,
		},
		{
			name: "preserves existing date range",
			filters: &TransactionValidationFilters{
				StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			},
			expectedLimit:  DefaultTransactionValidationFilterLimit,
			checkDateRange: false, // Don't override existing dates
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep original dates for comparison
			origStartDate := tt.filters.StartDate
			origEndDate := tt.filters.EndDate

			// Act
			tt.filters.SetDefaults()

			// Assert
			assert.Equal(t, tt.expectedLimit, tt.filters.Limit)

			if tt.checkDateRange {
				if origStartDate.IsZero() && origEndDate.IsZero() {
					// If original had no dates, defaults should have been set
					assert.False(t, tt.filters.StartDate.IsZero(), "StartDate should be set")
					assert.False(t, tt.filters.EndDate.IsZero(), "EndDate should be set")

					// Verify the range is 91 days (90 days before today midnight to tomorrow midnight)
					// This is because we use truncated day boundaries for consistent caching
					expectedRange := (DefaultTransactionValidationDateRangeDays + 1) * 24 * time.Hour
					actualRange := tt.filters.EndDate.Sub(tt.filters.StartDate)
					assert.Equal(t, expectedRange, actualRange, "Date range should be 91 days (90 + today)")
				}
			} else {
				// Dates should be preserved
				assert.Equal(t, origStartDate, tt.filters.StartDate)
				assert.Equal(t, origEndDate, tt.filters.EndDate)
			}
		})
	}
}

func TestTransactionValidationFiltersConstants(t *testing.T) {
	// Verify constant values
	assert.Equal(t, 1000, MaxTransactionValidationFilterLimit)
	assert.Equal(t, 100, DefaultTransactionValidationFilterLimit)
	assert.Equal(t, 90, DefaultTransactionValidationDateRangeDays)
}

func TestTransactionValidationFilters_SetDefaults_DateRange(t *testing.T) {
	// Test that SetDefaults sets a proper 90-day range using truncated day boundaries
	// Override nowFunc to use a fixed time for deterministic testing
	fixedNow := time.Date(2024, 6, 15, 14, 30, 45, 123456789, time.UTC)
	originalNowFunc := nowFunc
	nowFunc = func() time.Time { return fixedNow }
	defer func() { nowFunc = originalNowFunc }()

	// Calculate expected values using the same fixed time
	now := fixedNow.UTC().Truncate(24 * time.Hour)
	expectedEndDate := now.Add(24 * time.Hour)
	expectedStartDate := now.Add(-DefaultTransactionValidationDateRangeDays * 24 * time.Hour)

	filters := &TransactionValidationFilters{}
	filters.SetDefaults()

	// Verify EndDate is start of tomorrow (midnight tomorrow UTC)
	assert.Equal(t, expectedEndDate, filters.EndDate, "EndDate should be start of tomorrow (midnight)")

	// Verify StartDate is 90 days before the truncated now (start of day 90 days ago)
	assert.Equal(t, expectedStartDate, filters.StartDate, "StartDate should be 90 days before today midnight")

	// Verify both dates are at midnight (no sub-second precision)
	assert.Equal(t, 0, filters.StartDate.Hour(), "StartDate should be at midnight")
	assert.Equal(t, 0, filters.StartDate.Minute())
	assert.Equal(t, 0, filters.StartDate.Second())
	assert.Equal(t, 0, filters.StartDate.Nanosecond())

	assert.Equal(t, 0, filters.EndDate.Hour(), "EndDate should be at midnight")
	assert.Equal(t, 0, filters.EndDate.Minute())
	assert.Equal(t, 0, filters.EndDate.Second())
	assert.Equal(t, 0, filters.EndDate.Nanosecond())

	// Verify the range is exactly 91 days (90 days + today)
	actualRange := filters.EndDate.Sub(filters.StartDate)
	expectedRange := (DefaultTransactionValidationDateRangeDays + 1) * 24 * time.Hour
	assert.Equal(t, expectedRange, actualRange, "Date range should be 91 days (90 + today)")
}

func TestTransactionValidationFilters_SetDefaults_OnlyStartDateSet(t *testing.T) {
	// When only StartDate is set, EndDate should NOT be set automatically
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	filters := &TransactionValidationFilters{
		StartDate: startDate,
	}

	filters.SetDefaults()

	// StartDate should be preserved
	assert.Equal(t, startDate, filters.StartDate)
	// EndDate should remain zero (not set)
	assert.True(t, filters.EndDate.IsZero())
}

func TestTransactionValidationFilters_SetDefaults_OnlyEndDateSet(t *testing.T) {
	// When only EndDate is set, StartDate should NOT be set automatically
	endDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	filters := &TransactionValidationFilters{
		EndDate: endDate,
	}

	filters.SetDefaults()

	// EndDate should be preserved
	assert.Equal(t, endDate, filters.EndDate)
	// StartDate should remain zero (not set)
	assert.True(t, filters.StartDate.IsZero())
}
