// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"tracer/internal/testutil"
)

func TestAuditEventFilters_Validate(t *testing.T) {
	tests := []struct {
		name      string
		filters   *AuditEventFilters
		expectErr bool
		errorMsg  string
	}{
		{
			name: "success - valid filters with date range",
			filters: &AuditEventFilters{
				StartDate: testutil.DefaultTestTime.Add(-7 * 24 * time.Hour),
				EndDate:   testutil.DefaultTestTime,
				Limit:     100,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with event type",
			filters: &AuditEventFilters{
				EventType: func() *AuditEventType { et := AuditEventTransactionValidated; return &et }(),
				Limit:     50,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with result",
			filters: &AuditEventFilters{
				Result: func() *AuditResult { r := AuditResultAllow; return &r }(),
				Limit:  100,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with JSONB accountID",
			filters: &AuditEventFilters{
				AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
				Limit:     100,
			},
			expectErr: false,
		},
		{
			name: "success - valid filters with matchedRuleID",
			filters: &AuditEventFilters{
				MatchedRuleID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2)),
				Limit:         100,
			},
			expectErr: false,
		},
		{
			name:      "success - empty filters (uses defaults)",
			filters:   &AuditEventFilters{},
			expectErr: false,
		},
		{
			name: "success - limit equals maximum",
			filters: &AuditEventFilters{
				Limit: MaxAuditEventFilterLimit,
			},
			expectErr: false,
		},
		{
			name: "success - valid sortBy created_at",
			filters: &AuditEventFilters{
				Limit:  100,
				SortBy: "created_at",
			},
			expectErr: false,
		},
		{
			name: "success - valid sortBy event_type",
			filters: &AuditEventFilters{
				Limit:  100,
				SortBy: "event_type",
			},
			expectErr: false,
		},
		{
			name: "success - valid sortOrder ASC",
			filters: &AuditEventFilters{
				Limit:     100,
				SortOrder: "ASC",
			},
			expectErr: false,
		},
		{
			name: "success - valid sortOrder DESC",
			filters: &AuditEventFilters{
				Limit:     100,
				SortOrder: "DESC",
			},
			expectErr: false,
		},
		{
			name: "error - endDate before startDate",
			filters: &AuditEventFilters{
				StartDate: testutil.DefaultTestTime,
				EndDate:   testutil.DefaultTestTime.Add(-7 * 24 * time.Hour),
				Limit:     100,
			},
			expectErr: true,
			errorMsg:  "end_date must be on or after start_date",
		},
		{
			name: "error - limit exceeds maximum",
			filters: &AuditEventFilters{
				Limit: MaxAuditEventFilterLimit + 1,
			},
			expectErr: true,
			errorMsg:  "limit cannot exceed 1000",
		},
		{
			name: "error - negative limit",
			filters: &AuditEventFilters{
				Limit: -1,
			},
			expectErr: true,
			errorMsg:  "limit cannot be negative",
		},
		{
			name: "error - invalid sortBy field",
			filters: &AuditEventFilters{
				Limit:  100,
				SortBy: "invalidField",
			},
			expectErr: true,
			errorMsg:  "invalid sort_by field",
		},
		{
			name: "error - invalid sortOrder",
			filters: &AuditEventFilters{
				Limit:     100,
				SortOrder: "INVALID",
			},
			expectErr: true,
			errorMsg:  "sort_order must be ASC or DESC",
		},
		{
			name: "success - lowercase sortOrder normalized to uppercase",
			filters: &AuditEventFilters{
				Limit:     100,
				SortOrder: "asc",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filters.Validate()

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuditEventFilters_SetDefaults(t *testing.T) {
	t.Run("sets default limit when zero", func(t *testing.T) {
		filters := &AuditEventFilters{
			Limit: 0,
		}

		filters.SetDefaults()

		assert.Equal(t, DefaultAuditEventFilterLimit, filters.Limit)
	})

	t.Run("preserves non-zero limit", func(t *testing.T) {
		filters := &AuditEventFilters{
			Limit: 50,
		}

		filters.SetDefaults()

		assert.Equal(t, 50, filters.Limit)
	})

	t.Run("sets default date range when both dates are zero", func(t *testing.T) {
		filters := &AuditEventFilters{}

		filters.SetDefaults()

		assert.False(t, filters.StartDate.IsZero())
		assert.False(t, filters.EndDate.IsZero())
		assert.True(t, filters.EndDate.After(filters.StartDate))

		expectedDuration := DefaultAuditEventDateRangeDays * 24 * time.Hour
		actualDuration := filters.EndDate.Sub(filters.StartDate)
		assert.Equal(t, expectedDuration+24*time.Hour, actualDuration)
	})

	t.Run("preserves startDate when only startDate is set", func(t *testing.T) {
		customStart := testutil.DefaultTestTime.Add(-30 * 24 * time.Hour)
		filters := &AuditEventFilters{
			StartDate: customStart,
		}

		filters.SetDefaults()

		assert.Equal(t, customStart, filters.StartDate)
		assert.True(t, filters.EndDate.IsZero())
	})

	t.Run("preserves endDate when only endDate is set", func(t *testing.T) {
		customEnd := testutil.DefaultTestTime
		filters := &AuditEventFilters{
			EndDate: customEnd,
		}

		filters.SetDefaults()

		assert.True(t, filters.StartDate.IsZero())
		assert.Equal(t, customEnd, filters.EndDate)
	})

	t.Run("preserves both dates when both are set", func(t *testing.T) {
		customStart := testutil.DefaultTestTime.Add(-7 * 24 * time.Hour)
		customEnd := testutil.DefaultTestTime
		filters := &AuditEventFilters{
			StartDate: customStart,
			EndDate:   customEnd,
		}

		filters.SetDefaults()

		assert.Equal(t, customStart, filters.StartDate)
		assert.Equal(t, customEnd, filters.EndDate)
	})

	t.Run("sets default sortBy to created_at", func(t *testing.T) {
		filters := &AuditEventFilters{}

		filters.SetDefaults()

		assert.Equal(t, "created_at", filters.SortBy)
	})

	t.Run("preserves non-empty sortBy", func(t *testing.T) {
		filters := &AuditEventFilters{
			SortBy: "event_type",
		}

		filters.SetDefaults()

		assert.Equal(t, "event_type", filters.SortBy)
	})

	t.Run("sets default sortOrder to DESC", func(t *testing.T) {
		filters := &AuditEventFilters{}

		filters.SetDefaults()

		assert.Equal(t, "DESC", filters.SortOrder)
	})

	t.Run("preserves and normalizes sortOrder to uppercase", func(t *testing.T) {
		filters := &AuditEventFilters{
			SortOrder: "asc",
		}

		filters.SetDefaults()

		assert.Equal(t, "ASC", filters.SortOrder)
	})
}

func TestIsValidAuditEventSortField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "createdAt is rejected (camelCase)",
			field:    "createdAt",
			expected: false,
		},
		{
			name:     "eventType is rejected (camelCase)",
			field:    "eventType",
			expected: false,
		},
		{
			name:     "invalid field",
			field:    "invalidField",
			expected: false,
		},
		{
			name:     "empty string is invalid",
			field:    "",
			expected: false,
		},
		{
			name:     "action is not a valid sort field",
			field:    "action",
			expected: false,
		},
		{
			name:     "created_at is valid (snake_case)",
			field:    "created_at",
			expected: true,
		},
		{
			name:     "event_type is valid (snake_case)",
			field:    "event_type",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidAuditEventSortField(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuditEventFilters_Constants(t *testing.T) {
	t.Run("MaxAuditEventFilterLimit is 1000", func(t *testing.T) {
		assert.Equal(t, 1000, MaxAuditEventFilterLimit)
	})

	t.Run("DefaultAuditEventFilterLimit is 100", func(t *testing.T) {
		assert.Equal(t, 100, DefaultAuditEventFilterLimit)
	})

	t.Run("DefaultAuditEventDateRangeDays is 90", func(t *testing.T) {
		assert.Equal(t, 90, DefaultAuditEventDateRangeDays)
	})
}
