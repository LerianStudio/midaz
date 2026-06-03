// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsValidDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		date     string
		expected bool
	}{
		{
			name:     "Valid date - standard format",
			date:     "2024-01-15",
			expected: true,
		},
		{
			name:     "Valid date - first day of month",
			date:     "2024-01-01",
			expected: true,
		},
		{
			name:     "Valid date - last day of month",
			date:     "2024-01-31",
			expected: true,
		},
		{
			name:     "Valid date - leap year February",
			date:     "2024-02-29",
			expected: true,
		},
		{
			name:     "Invalid date - non-leap year February 29",
			date:     "2023-02-29",
			expected: false,
		},
		{
			name:     "Invalid date - wrong format with slashes",
			date:     "2024/01/15",
			expected: false,
		},
		{
			name:     "Invalid date - wrong format DD-MM-YYYY",
			date:     "15-01-2024",
			expected: false,
		},
		{
			name:     "Invalid date - month out of range",
			date:     "2024-13-01",
			expected: false,
		},
		{
			name:     "Invalid date - day out of range",
			date:     "2024-01-32",
			expected: false,
		},
		{
			name:     "Invalid date - empty string",
			date:     "",
			expected: false,
		},
		{
			name:     "Invalid date - random text",
			date:     "not-a-date",
			expected: false,
		},
		{
			name:     "Invalid date - partial date",
			date:     "2024-01",
			expected: false,
		},
		{
			name:     "Invalid date - extra characters",
			date:     "2024-01-15T00:00:00",
			expected: false,
		},
		{
			name:     "Valid date - edge case year",
			date:     "9999-12-31",
			expected: true,
		},
		{
			name:     "Valid date - year 0001",
			date:     "0001-01-01",
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsValidDate(tt.date)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInitialDateBeforeFinalDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  time.Time
		final    time.Time
		expected bool
	}{
		{
			name:     "Initial before final",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "Initial equals final",
			initial:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "Initial after final",
			initial:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "Same day, different times - initial before",
			initial:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "Same day, different times - initial after",
			initial:  time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "Different years - initial before",
			initial:  time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "Different years - initial after",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsInitialDateBeforeFinalDate(tt.initial, tt.final)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDateRangeWithinMonthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  time.Time
		final    time.Time
		limit    int
		expected bool
	}{
		{
			name:     "Within 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
		{
			name:     "Exactly at 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
		{
			name:     "Exceeds 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: false,
		},
		{
			name:     "Within 3 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			limit:    3,
			expected: true,
		},
		{
			name:     "Exceeds 3 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
			limit:    3,
			expected: false,
		},
		{
			name:     "Within 12 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			limit:    12,
			expected: true,
		},
		{
			name:     "Same day - within any limit",
			initial:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
		{
			name:     "Zero month limit - same day",
			initial:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			limit:    0,
			expected: true,
		},
		{
			name:     "Cross year boundary within limit",
			initial:  time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			limit:    2,
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsDateRangeWithinMonthLimit(tt.initial, tt.final, tt.limit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		date     time.Time
		days     *int
		expected string
	}{
		{
			name:     "No days adjustment",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     nil,
			expected: "2024-01-15",
		},
		{
			name:     "Add 1 day",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     intPtr(1),
			expected: "2024-01-16",
		},
		{
			name:     "Subtract 1 day",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     intPtr(-1),
			expected: "2024-01-14",
		},
		{
			name:     "Add 0 days",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     intPtr(0),
			expected: "2024-01-15",
		},
		{
			name:     "Add days crossing month boundary",
			date:     time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			days:     intPtr(1),
			expected: "2024-02-01",
		},
		{
			name:     "Subtract days crossing month boundary",
			date:     time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			days:     intPtr(-1),
			expected: "2024-01-31",
		},
		{
			name:     "Add days crossing year boundary",
			date:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			days:     intPtr(1),
			expected: "2025-01-01",
		},
		{
			name:     "Subtract days crossing year boundary",
			date:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			days:     intPtr(-1),
			expected: "2023-12-31",
		},
		{
			name:     "Add large number of days",
			date:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			days:     intPtr(365),
			expected: "2024-12-31", // 2024 is a leap year
		},
		{
			name:     "Leap year February 29",
			date:     time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC),
			days:     intPtr(1),
			expected: "2024-02-29",
		},
		{
			name:     "Non-leap year February to March",
			date:     time.Date(2023, 2, 28, 0, 0, 0, 0, time.UTC),
			days:     intPtr(1),
			expected: "2023-03-01",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeDate(tt.date, tt.days)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to create an int pointer
func intPtr(i int) *int {
	return &i
}
