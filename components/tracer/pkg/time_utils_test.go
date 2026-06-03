// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"tracer/internal/testutil"
)

func TestIsValidDate(t *testing.T) {
	tests := []struct {
		name     string
		date     string
		expected bool
	}{
		{
			name:     "valid date format",
			date:     "2024-01-15",
			expected: true,
		},
		{
			name:     "valid date - end of month",
			date:     "2024-12-31",
			expected: true,
		},
		{
			name:     "valid date - leap year",
			date:     "2024-02-29",
			expected: true,
		},
		{
			name:     "invalid format - wrong separator",
			date:     "2024/01/15",
			expected: false,
		},
		{
			name:     "invalid format - month day year",
			date:     "01-15-2024",
			expected: false,
		},
		{
			name:     "invalid date - month 13",
			date:     "2024-13-01",
			expected: false,
		},
		{
			name:     "invalid date - day 32",
			date:     "2024-01-32",
			expected: false,
		},
		{
			name:     "invalid date - Feb 30",
			date:     "2024-02-30",
			expected: false,
		},
		{
			name:     "invalid - empty string",
			date:     "",
			expected: false,
		},
		{
			name:     "invalid - random string",
			date:     "not-a-date",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDate(tt.date)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInitialDateBeforeFinalDate(t *testing.T) {
	tests := []struct {
		name     string
		initial  time.Time
		final    time.Time
		expected bool
	}{
		{
			name:     "initial before final",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "initial equals final",
			initial:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "initial after final",
			initial:  time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "different years - initial before",
			initial:  time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "different years - initial after",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInitialDateBeforeFinalDate(tt.initial, tt.final)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDateRangeWithinMonthLimit(t *testing.T) {
	tests := []struct {
		name     string
		initial  time.Time
		final    time.Time
		limit    int
		expected bool
	}{
		{
			name:     "within 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
		{
			name:     "exactly at 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
		{
			name:     "exceeds 1 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: false,
		},
		{
			name:     "within 3 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			limit:    3,
			expected: true,
		},
		{
			name:     "exceeds 3 month limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
			limit:    3,
			expected: false,
		},
		{
			name:     "same day is within any limit",
			initial:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			final:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			limit:    1,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDateRangeWithinMonthLimit(tt.initial, tt.final, tt.limit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeDate(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		days     *int
		expected string
	}{
		{
			name:     "no days adjustment",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     nil,
			expected: "2024-01-15",
		},
		{
			name:     "add 1 day",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(1),
			expected: "2024-01-16",
		},
		{
			name:     "subtract 1 day",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(-1),
			expected: "2024-01-14",
		},
		{
			name:     "add 0 days",
			date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(0),
			expected: "2024-01-15",
		},
		{
			name:     "cross month boundary forward",
			date:     time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(1),
			expected: "2024-02-01",
		},
		{
			name:     "cross month boundary backward",
			date:     time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(-1),
			expected: "2024-01-31",
		},
		{
			name:     "cross year boundary",
			date:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			days:     testutil.Ptr(1),
			expected: "2025-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeDate(tt.date, tt.days)
			assert.Equal(t, tt.expected, result)
		})
	}
}
