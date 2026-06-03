// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
)

// =============================================================================
// Error Code Tests for Audit Event Handler
// =============================================================================
// These tests verify the error codes returned by audit event handler operations.
//
// Error code mapping:
//   - TRC-0007: Invalid path parameter (invalid UUID for eventId)
//   - TRC-0140: Audit event not found
//   - TRC-0141: Invalid audit event filters
//   - TRC-0043: Invalid sort column
//   - TRC-0044: Invalid pagination cursor
// =============================================================================

// =============================================================================
// GET /v1/audit-events/{id} - Get Audit Event Error Code Tests
// =============================================================================

// TestGetAuditEvent_InvalidUUID_ReturnsTRC0007 verifies that invalid UUID format returns TRC-0007.
func TestGetAuditEvent_InvalidUUID_ReturnsTRC0007(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		eventID string
		desc    string
	}{
		{
			name:    "plain_text",
			eventID: "invalid-uuid",
			desc:    "Plain text instead of UUID",
		},
		{
			name:    "too_short",
			eventID: "12345",
			desc:    "String too short to be UUID",
		},
		{
			name:    "partial_uuid",
			eventID: "550e8400-e29b-41d4",
			desc:    "Partial UUID (incomplete)",
		},
		{
			name:    "uuid_with_extra_chars",
			eventID: "550e8400-e29b-41d4-a716-446655440000-extra",
			desc:    "UUID with extra characters at end",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+tc.eventID, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0007", errResp.Code, "Test case: %s - Expected TRC-0007 for invalid UUID", tc.desc)
			assert.Equal(t, "Invalid Path Parameter", errResp.Title, "Test case: %s", tc.desc)
			assert.Equal(t, "Invalid event ID format", errResp.Message, "Test case: %s", tc.desc)
		})
	}
}

// TestGetAuditEvent_NonExistentUUID_ReturnsTRC0140 verifies that non-existent UUID returns TRC-0140.
func TestGetAuditEvent_NonExistentUUID_ReturnsTRC0140(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		eventID string
		desc    string
	}{
		{
			name:    "all_zeros",
			eventID: "00000000-0000-0000-0000-000000000000",
			desc:    "All zeros UUID",
		},
		{
			name:    "random_uuid_v4",
			eventID: testutil.MustDeterministicUUID(7103).String(),
			desc:    "Random valid UUID that doesn't exist",
		},
		{
			name:    "specific_nonexistent",
			eventID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			desc:    "Specific non-existent UUID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+tc.eventID, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0140", errResp.Code, "Test case: %s - Expected TRC-0140 for audit event not found", tc.desc)
			assert.Equal(t, "Not Found", errResp.Title, "Test case: %s", tc.desc)
			assert.Equal(t, "Audit event not found", errResp.Message, "Test case: %s", tc.desc)
		})
	}
}

// =============================================================================
// GET /v1/audit-events - List Audit Events Error Code Tests
// =============================================================================

// TestListAuditEvents_InvalidSortColumn_ReturnsTRC0043 verifies that invalid sortBy returns TRC-0043.
func TestListAuditEvents_InvalidSortColumn_ReturnsTRC0043(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		queryParams string
		desc        string
	}{
		{
			name:        "invalid_column_name",
			queryParams: "sort_by=invalidColumn",
			desc:        "Non-existent column name",
		},
		{
			name:        "random_string",
			queryParams: "sort_by=xyz123",
			desc:        "Random string as sort column",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0043", errResp.Code, "Test case: %s - Expected TRC-0043 for invalid sort column", tc.desc)
			assert.Equal(t, "Validation Error", errResp.Title, "Test case: %s", tc.desc)
		})
	}
}

// TestListAuditEvents_InvalidCursor_ReturnsTRC0044 verifies that invalid cursor returns TRC-0044.
func TestListAuditEvents_InvalidCursor_ReturnsTRC0044(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		queryParams string
		desc        string
	}{
		{
			name:        "random_string",
			queryParams: "cursor=not-a-valid-cursor",
			desc:        "Random string as cursor",
		},
		{
			name:        "numeric_string",
			queryParams: "cursor=12345678",
			desc:        "Numeric string as cursor",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, "TRC-0044", errResp.Code, "Test case: %s - Expected TRC-0044 for invalid cursor", tc.desc)
			assert.Equal(t, "Bad Request", errResp.Title, "Test case: %s", tc.desc)
		})
	}
}

// TestListAuditEvents_InvalidFilters_ReturnsValidationError verifies filter validation.
// Note: TRC-0141 is returned by the service layer for invalid audit event filters,
// while TRC-0001 is returned for input validation errors at the handler layer.
func TestListAuditEvents_InvalidFilters_ReturnsValidationError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name          string
		queryParams   string
		expectedCode  string
		expectedTitle string
		desc          string
	}{
		{
			name:          "invalid_event_type",
			queryParams:   "event_type=INVALID_EVENT_TYPE",
			expectedCode:  "TRC-0001",
			expectedTitle: "Validation Error",
			desc:          "Invalid event type enum value",
		},
		{
			name:          "invalid_action",
			queryParams:   "action=INVALID_ACTION",
			expectedCode:  "TRC-0001",
			expectedTitle: "Validation Error",
			desc:          "Invalid action enum value",
		},
		{
			name:          "invalid_result",
			queryParams:   "result=INVALID_RESULT",
			expectedCode:  "TRC-0001",
			expectedTitle: "Validation Error",
			desc:          "Invalid result enum value",
		},
		{
			name:          "invalid_resource_type",
			queryParams:   "resource_type=invalid_type",
			expectedCode:  "TRC-0001",
			expectedTitle: "Validation Error",
			desc:          "Invalid resource type",
		},
		{
			name:          "invalid_start_date_format",
			queryParams:   "start_date=not-a-date",
			expectedCode:  "TRC-0020",
			expectedTitle: "Validation Error",
			desc:          "Invalid start date format",
		},
		{
			name:          "invalid_end_date_format",
			queryParams:   "end_date=2024-13-45",
			expectedCode:  "TRC-0020",
			expectedTitle: "Validation Error",
			desc:          "Invalid end date format",
		},
		{
			name:          "limit_negative",
			queryParams:   "limit=-5",
			expectedCode:  "TRC-0041",
			expectedTitle: "Validation Error",
			desc:          "Negative limit is invalid",
		},
		{
			name:          "limit_zero",
			queryParams:   "limit=0",
			expectedCode:  "TRC-0041",
			expectedTitle: "Validation Error",
			desc:          "Zero limit is invalid (must be at least 1)",
		},
		{
			name:          "limit_exceeds_max",
			queryParams:   "limit=2000",
			expectedCode:  "TRC-0040",
			expectedTitle: "Validation Error",
			desc:          "Limit exceeds maximum allowed (1000)",
		},
		{
			name:          "invalid_transaction_type",
			queryParams:   "transaction_type=INVALID_TYPE",
			expectedCode:  "TRC-0001",
			expectedTitle: "Validation Error",
			desc:          "Invalid transaction type enum value",
		},
		{
			name:          "invalid_sort_order",
			queryParams:   "sort_order=INVALID",
			expectedCode:  "TRC-0042",
			expectedTitle: "Validation Error",
			desc:          "Invalid sort order value (must be ASC or DESC)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, tc.expectedCode, errResp.Code, "Test case: %s - Expected %s", tc.desc, tc.expectedCode)
			assert.Equal(t, tc.expectedTitle, errResp.Title, "Test case: %s", tc.desc)
		})
	}
}

// TestListAuditEvents_CursorWithSortParams_ReturnsTRC0045 verifies that providing
// sortBy or sortOrder parameters when a cursor is present returns TRC-0045.
// Sort parameters are encoded in the cursor and cannot be changed mid-pagination.
func TestListAuditEvents_CursorWithSortParams_ReturnsTRC0045(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Setup: Create 2 rules to ensure we have enough events for pagination
	rule1Name := "Cursor Params Test 1 " + testutil.MustDeterministicUUID(7109).String()[:8]
	rule1ID := testutil.CreateTestRuleWithExpression(t, rule1Name, "amount > 20000", "DENY")
	t.Cleanup(func() {
		testutil.CleanupRule(t, rule1ID)
	})

	rule2Name := "Cursor Params Test 2 " + testutil.MustDeterministicUUID(7110).String()[:8]
	rule2ID := testutil.CreateTestRuleWithExpression(t, rule2Name, "amount < 50", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, rule2ID)
	})

	// Step 1: Get a valid cursor by listing audit events with a small limit
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=1", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Initial list should succeed: %s", string(respBody))

	// Parse response to get cursor
	var listResp struct {
		NextCursor string `json:"nextCursor"`
		HasMore    bool   `json:"hasMore"`
	}
	err = json.Unmarshal(respBody, &listResp)
	require.NoError(t, err)

	require.NotEmpty(t, listResp.NextCursor, "Should have next cursor after creating 2+ events with limit=1")

	// Step 2: Test cursor with sortBy parameter
	t.Run("cursor_with_sortBy", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?cursor="+listResp.NextCursor+"&sort_by=created_at", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Cursor with sortBy should return 400: %s", string(respBody))

		errResp := testutil.ParseErrorResponse(t, respBody)

		assert.Equal(t, "TRC-0045", errResp.Code, "Expected TRC-0045 for cursor with sortBy")
		assert.Equal(t, "Validation Error", errResp.Title, "Expected 'Validation Error' title")
	})

	// Step 3: Test cursor with sortOrder parameter
	t.Run("cursor_with_sortOrder", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?cursor="+listResp.NextCursor+"&sort_order=DESC", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Cursor with sortOrder should return 400: %s", string(respBody))

		errResp := testutil.ParseErrorResponse(t, respBody)

		assert.Equal(t, "TRC-0045", errResp.Code, "Expected TRC-0045 for cursor with sortOrder")
		assert.Equal(t, "Validation Error", errResp.Title, "Expected 'Validation Error' title")
	})

	// Step 4: Test cursor with both sortBy and sortOrder parameters
	t.Run("cursor_with_both_sort_params", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?cursor="+listResp.NextCursor+"&sort_by=created_at&sort_order=ASC", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Cursor with both sort params should return 400: %s", string(respBody))

		errResp := testutil.ParseErrorResponse(t, respBody)

		assert.Equal(t, "TRC-0045", errResp.Code, "Expected TRC-0045 for cursor with both sort params")
		assert.Equal(t, "Validation Error", errResp.Title, "Expected 'Validation Error' title")
	})
}

// =============================================================================
// GET /v1/audit-events/{id}/verify - Verify Hash Chain Error Code Tests
// =============================================================================

// TestVerifyAuditEvent_InvalidUUID_ReturnsTRC0007 verifies that invalid UUID format returns TRC-0007.
func TestVerifyAuditEvent_InvalidUUID_ReturnsTRC0007(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		eventID string
		desc    string
	}{
		{
			name:    "plain_text",
			eventID: "invalid-uuid",
			desc:    "Plain text instead of UUID",
		},
		{
			name:    "too_short",
			eventID: "12345",
			desc:    "String too short to be UUID",
		},
		{
			name:    "partial_uuid",
			eventID: "550e8400-e29b-41d4",
			desc:    "Partial UUID (incomplete)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+tc.eventID+"/verify", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0007", errResp.Code, "Test case: %s - Expected TRC-0007 for invalid UUID", tc.desc)
			assert.Equal(t, "Invalid Path Parameter", errResp.Title, "Test case: %s", tc.desc)
			assert.Equal(t, "Invalid event ID format", errResp.Message, "Test case: %s", tc.desc)
		})
	}
}

// TestVerifyAuditEvent_NonExistentUUID_ReturnsTRC0140 verifies that non-existent UUID returns TRC-0140.
func TestVerifyAuditEvent_NonExistentUUID_ReturnsTRC0140(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		eventID string
		desc    string
	}{
		{
			name:    "all_zeros",
			eventID: "00000000-0000-0000-0000-000000000000",
			desc:    "All zeros UUID",
		},
		{
			name:    "random_uuid_v4",
			eventID: testutil.MustDeterministicUUID(7102).String(),
			desc:    "Random valid UUID that doesn't exist",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+tc.eventID+"/verify", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0140", errResp.Code, "Test case: %s - Expected TRC-0140 for audit event not found", tc.desc)
			assert.Equal(t, "Not Found", errResp.Title, "Test case: %s", tc.desc)
			assert.Equal(t, "Audit event not found", errResp.Message, "Test case: %s", tc.desc)
		})
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// TestAuditEvents_ValidSortColumns_Succeeds verifies that valid sort columns work correctly.
func TestAuditEvents_ValidSortColumns_Succeeds(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	validSortColumns := []string{"created_at", "event_type"}

	for _, column := range validSortColumns {
		t.Run("sortBy_"+column, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?sort_by="+column+"&limit=1", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode, "sort_by=%s should succeed - Response: %s", column, string(respBody))
		})
	}
}

// TestAuditEvents_ValidSortOrders_Succeeds verifies that valid sort orders work correctly.
func TestAuditEvents_ValidSortOrders_Succeeds(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	validSortOrders := []string{"ASC", "DESC", "asc", "desc"}

	for _, order := range validSortOrders {
		t.Run("sortOrder_"+order, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?sort_order="+order+"&limit=1", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode, "sort_order=%s should succeed - Response: %s", order, string(respBody))
		})
	}
}

// TestAuditEvents_ValidLimitBoundaries_Succeeds verifies valid limit boundaries.
func TestAuditEvents_ValidLimitBoundaries_Succeeds(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name  string
		limit string
		desc  string
	}{
		{
			name:  "minimum_limit",
			limit: "1",
			desc:  "Minimum valid limit",
		},
		{
			name:  "default_limit",
			limit: "100",
			desc:  "Default limit value",
		},
		{
			name:  "maximum_limit",
			limit: "1000",
			desc:  "Maximum valid limit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit="+tc.limit, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Test case: %s - Response: %s", tc.desc, string(respBody))
		})
	}
}

// =============================================================================
// Cross-Endpoint Date Range Consistency Tests
// =============================================================================

// TestDateRangeValidation_ConsistentAcrossEndpoints verifies that both audit-events and
// transaction-validations endpoints reject invalid date ranges (start_date > end_date)
// with HTTP 400 status, ensuring consistent behavior across the API.
func TestDateRangeValidation_ConsistentAcrossEndpoints(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Invalid date range using past dates: start_date is after end_date
	startDate := "2025-06-30T23:59:59Z"
	endDate := "2025-01-01T00:00:00Z"

	endpoints := []struct {
		name            string
		url             string
		expectedCode    string
		expectedMessage string
	}{
		{
			name:            "audit-events",
			url:             fmt.Sprintf("%s/v1/audit-events?start_date=%s&end_date=%s", baseURL, startDate, endDate),
			expectedCode:    "TRC-0023",
			expectedMessage: "end_date must be on or after start_date",
		},
		{
			name:            "transaction-validations",
			url:             fmt.Sprintf("%s/v1/validations?start_date=%s&end_date=%s", baseURL, startDate, endDate),
			expectedCode:    "TRC-0023",
			expectedMessage: "end_date must be on or after start_date",
		},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ep.url, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Endpoint %s should reject invalid date range (start > end): %s", ep.name, string(respBody))

			var errResp map[string]any
			err = json.Unmarshal(respBody, &errResp)
			require.NoError(t, err, "Endpoint %s should return valid JSON error response", ep.name)

			require.Equal(t, ep.expectedCode, errResp["code"],
				"Endpoint %s should return error code %s", ep.name, ep.expectedCode)
			require.Equal(t, ep.expectedMessage, errResp["message"],
				"Endpoint %s should return expected error message", ep.name)
		})
	}
}
