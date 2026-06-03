// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestValidateParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		params    map[string]string
		wantErr   bool
		checkFunc func(*testing.T, *QueryHeader)
	}{
		{
			name:    "Valid parameters with defaults",
			params:  map[string]string{},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.Equal(t, 10, q.Limit)
				assert.Equal(t, 1, q.Page)
				assert.Equal(t, "desc", q.SortOrder)
			},
		},
		{
			name: "Valid parameters with custom values",
			params: map[string]string{
				"limit": "20",
				"page":  "2",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.Equal(t, 20, q.Limit)
				assert.Equal(t, 2, q.Page)
			},
		},
		{
			name: "Valid UUID parameters",
			params: map[string]string{
				"segmentId": uuid.New().String(),
				"ledgerId":  uuid.New().String(),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.NotEqual(t, uuid.Nil, q.SegmentID)
				assert.NotEqual(t, uuid.Nil, q.LedgerID)
			},
		},
		{
			name: "Invalid UUID in segmentId",
			params: map[string]string{
				"segmentId": "invalid-uuid",
			},
			wantErr: true,
		},
		{
			name: "Invalid UUID in ledgerId",
			params: map[string]string{
				"ledgerId": "invalid-uuid",
			},
			wantErr: true,
		},
		{
			name: "Valid date range",
			params: map[string]string{
				"start_date": "2024-01-01",
				"end_date":   "2024-01-31",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.False(t, q.StartDate.IsZero())
				assert.False(t, q.EndDate.IsZero())
			},
		},
		{
			name: "Valid enable flag",
			params: map[string]string{
				"enable": "true",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.NotNil(t, q.Enable)
				assert.True(t, *q.Enable)
			},
		},
		{
			name: "Valid enable flag false",
			params: map[string]string{
				"enable": "false",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.NotNil(t, q.Enable)
				assert.False(t, *q.Enable)
			},
		},
		{
			name: "Invalid enable flag format",
			params: map[string]string{
				"enable": "invalid",
			},
			wantErr: true,
		},
		{
			name: "Valid transaction route",
			params: map[string]string{
				"transactionRoute": "debitoted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.NotNil(t, q.TransactionRoute)
				assert.Equal(t, "debitoted", *q.TransactionRoute)
			},
		},
		{
			name: "Valid to asset codes",
			params: map[string]string{
				"to": "USD,EUR,BTC",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, q *QueryHeader) {
				assert.Equal(t, []string{"USD", "EUR", "BTC"}, q.ToAssetCodes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateParameters(tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestValidateParameters_DateValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		errCode string
	}{
		{
			name: "Invalid date format",
			params: map[string]string{
				"start_date": "2024/01/01",
				"end_date":   "2024/01/31",
			},
			wantErr: true,
			errCode: constant.ErrInvalidDateFormat.Error(),
		},
		{
			name: "End date before start date",
			params: map[string]string{
				"start_date": "2024-01-31",
				"end_date":   "2024-01-01",
			},
			wantErr: true,
			errCode: constant.ErrInvalidFinalDate.Error(),
		},
		{
			name: "Date range exceeds limit",
			params: map[string]string{
				"start_date": "2023-01-01",
				"end_date":   "2024-12-31",
			},
			wantErr: true,
			errCode: constant.ErrDateRangeExceedsLimit.Error(),
		},
		{
			name: "Only start date provided",
			params: map[string]string{
				"start_date": "2024-01-01",
			},
			wantErr: true,
			errCode: constant.ErrInvalidDateRange.Error(),
		},
		{
			name: "Only end date provided",
			params: map[string]string{
				"end_date": "2024-01-31",
			},
			wantErr: true,
			errCode: constant.ErrInvalidDateRange.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateParameters(tt.params)

			assert.Error(t, err)
			assert.Nil(t, result)

			if httpErr, ok := err.(*pkg.HTTPError); ok {
				assert.Contains(t, httpErr.Code, tt.errCode)
			}
		})
	}
}

func TestValidateParameters_PaginationValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		errCode string
	}{
		{
			name: "Limit exceeds maximum",
			params: map[string]string{
				"limit": "101",
			},
			wantErr: true,
			errCode: constant.ErrPaginationLimitExceeded.Error(),
		},
		{
			name: "Invalid sort order",
			params: map[string]string{
				"sort_order": "invalid",
			},
			wantErr: true,
			errCode: constant.ErrInvalidSortOrder.Error(),
		},
		{
			name: "Page less than 1",
			params: map[string]string{
				"page": "0",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameterPage.Error(),
		},
		{
			name: "Invalid page format (non-numeric)",
			params: map[string]string{
				"page": "abc",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameterPage.Error(),
		},
		{
			name: "Invalid limit format (non-numeric)",
			params: map[string]string{
				"limit": "xyz",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name: "Limit zero",
			params: map[string]string{
				"limit": "0",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name: "Limit negative",
			params: map[string]string{
				"limit": "-5",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name: "Invalid cursor",
			params: map[string]string{
				"cursor": "invalid-cursor-format",
			},
			wantErr: true,
			errCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name: "Valid sort order asc",
			params: map[string]string{
				"sort_order": "asc",
			},
			wantErr: false,
		},
		{
			name: "Valid sort order desc",
			params: map[string]string{
				"sort_order": "desc",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateParameters(tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)

				if httpErr, ok := err.(*pkg.HTTPError); ok {
					assert.Contains(t, httpErr.Code, tt.errCode)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

func TestParseParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
	}{
		{
			name:    "Empty params",
			params:  map[string]string{},
			wantErr: false,
		},
		{
			name: "All param types",
			params: map[string]string{
				"limit":            "10",
				"page":             "1",
				"cursor":           "test",
				"sort_order":       "desc",
				"start_date":       "2024-01-01",
				"end_date":         "2024-01-31",
				"segmentId":        uuid.New().String(),
				"ledgerId":         uuid.New().String(),
				"transactionRoute": "test",
				"enable":           "true",
				"to":               "USD,EUR",
			},
			wantErr: false,
		},
		{
			name: "Case insensitive keys",
			params: map[string]string{
				"LIMIT": "20",
				"PAGE":  "2",
			},
			wantErr: false,
		},
		{
			name: "Invalid limit format",
			params: map[string]string{
				"limit": "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "Invalid page format",
			params: map[string]string{
				"page": "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "Invalid enable format",
			params: map[string]string{
				"enable": "not-a-boolean",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &QueryHeader{
				Limit:     10,
				Page:      1,
				SortOrder: "desc",
			}

			err := parseParams(tt.params, query)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify case-insensitive keys were applied correctly
			if tt.name == "Case insensitive keys" {
				assert.Equal(t, 20, query.Limit, "LIMIT should be parsed as limit")
				assert.Equal(t, 2, query.Page, "PAGE should be parsed as page")
			}
		})
	}
}

func TestValidateDates(t *testing.T) {
	tests := []struct {
		name      string
		startDate time.Time
		endDate   time.Time
		wantErr   bool
	}{
		{
			name:      "Both zero - should set defaults",
			startDate: time.Time{},
			endDate:   time.Time{},
			wantErr:   false,
		},
		{
			name:      "Valid date range",
			startDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			wantErr:   false,
		},
		{
			name:      "Start date after end date",
			startDate: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			endDate:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr:   true,
		},
		{
			name:      "Only start date",
			startDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:   time.Time{},
			wantErr:   true,
		},
		{
			name:      "Only end date",
			startDate: time.Time{},
			endDate:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startDate := tt.startDate
			endDate := tt.endDate

			err := validateDates(&startDate, &endDate)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// If both were zero, they should be set to defaults
			if tt.startDate.IsZero() && tt.endDate.IsZero() {
				assert.False(t, startDate.IsZero())
				assert.False(t, endDate.IsZero())
			}
		})
	}
}

func TestValidatePagination(t *testing.T) {
	tests := []struct {
		name      string
		cursor    string
		sortOrder string
		page      int
		limit     int
		wantErr   bool
	}{
		{
			name:      "Valid pagination",
			cursor:    "",
			sortOrder: "desc",
			page:      1,
			limit:     10,
			wantErr:   false,
		},
		{
			name:      "Limit exceeds maximum",
			cursor:    "",
			sortOrder: "desc",
			page:      1,
			limit:     101,
			wantErr:   true,
		},
		{
			name:      "Invalid sort order",
			cursor:    "",
			sortOrder: "invalid",
			page:      1,
			limit:     10,
			wantErr:   true,
		},
		{
			name:      "Page less than 1",
			cursor:    "",
			sortOrder: "desc",
			page:      0,
			limit:     10,
			wantErr:   true,
		},
		{
			name:      "Valid cursor",
			cursor:    "eyJpZCI6InRlc3QtaWQiLCJkaXJlY3Rpb24iOiJuZXh0In0=", // base64 encoded JSON: {"id":"test-id","direction":"next"}
			sortOrder: "desc",
			page:      1,
			limit:     10,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePagination(tt.cursor, tt.sortOrder, tt.page, tt.limit)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestQueryHeader_DefaultValues(t *testing.T) {
	result, err := ValidateParameters(map[string]string{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 10, result.Limit)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, "desc", result.SortOrder)
}
