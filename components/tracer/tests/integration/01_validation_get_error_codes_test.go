// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Error Code Tests for Transaction Validation Handler
// =============================================================================
//
// These tests document the error handling behavior for the transaction
// validation endpoints (GET /v1/validations and GET /v1/validations/{id}).
//
// Errors follow the RFC 9457 (application/problem+json) envelope with a numeric
// `code`, a specific `title`, and a `detail`. Each error code carries its own
// title:
// - Invalid path parameter (UUID): code 0065, title "Invalid Path Parameter".
// - Invalid query parameter (non-numeric, malformed): code 0082, title
//   "Invalid Query Parameter".
// - Pagination/sort/filter validation: per-error codes and titles, e.g. 0331
//   "Pagination Limit Invalid", 0080 "Pagination Limit Exceeded", 0332 "Invalid
//   Sort Column", 0081 "Invalid Sort Order", 0334 "Cursor With Sort Params",
//   0077 "Invalid Date Format Error", 0083 "Invalid Date Range Error", 0431
//   "Invalid Transaction Validation Filters".
// =============================================================================

// =============================================================================
// 3.1 GET /v1/validations/{id} - Invalid ID Tests
// =============================================================================

// TestGetTransactionValidation_InvalidID_ReturnsError verifies invalid UUID handling in path parameter.
// Returns code 0065 (title "Invalid Path Parameter") for invalid path parameters.
func TestGetTransactionValidation_InvalidID_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name     string
		id       string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "not-a-uuid",
			id:       "not-a-uuid",
			wantCode: "0065",
			wantMsg:  "",
		},
		{
			name:     "numeric-id",
			id:       "12345",
			wantCode: "0065",
			wantMsg:  "",
		},
		{
			name:     "partial-uuid",
			id:       "550e8400-e29b-41d4",
			wantCode: "0065",
			wantMsg:  "",
		},
		{
			name:     "uuid-with-extra-chars",
			id:       "550e8400-e29b-41d4-a716-446655440000-extra",
			wantCode: "0065",
			wantMsg:  "",
		},
		{
			name:     "uppercase-invalid-uuid",
			id:       "NOT-A-UUID-FORMAT",
			wantCode: "0065",
			wantMsg:  "",
		},
		{
			name:     "malformed-uuid-bad-chars",
			id:       "550e8400-e29b-41d4-a716-44665544000g",
			wantCode: "0065",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations/"+tc.id, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for id=%q", tc.wantCode, tc.id)
			assert.Equal(t, "Invalid Path Parameter", errResp.Title)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// =============================================================================
// 3.2 GET /v1/validations - Invalid Query Parameter Tests
// =============================================================================

// TestListTransactionValidations_InvalidLimit_ReturnsError verifies invalid limit parameter handling.
// Returns 0331 "Pagination Limit Invalid" / 0080 "Pagination Limit Exceeded" for out-of-range limits
// and 0082 "Invalid Query Parameter" for non-numeric/decimal limits.
func TestListTransactionValidations_InvalidLimit_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name      string
		query     string
		wantCode  string
		wantTitle string
		wantMsg   string
	}{
		{
			name:      "limit-zero",
			query:     "limit=0",
			wantCode:  "0331",
			wantTitle: "Pagination Limit Invalid",
			wantMsg:   "",
		},
		{
			name:      "limit-negative",
			query:     "limit=-1",
			wantCode:  "0331",
			wantTitle: "Pagination Limit Invalid",
			wantMsg:   "",
		},
		{
			name:      "limit-exceeds-max",
			query:     "limit=1001",
			wantCode:  "0080",
			wantTitle: "Pagination Limit Exceeded",
			wantMsg:   "",
		},
		{
			name:      "limit-non-numeric",
			query:     "limit=abc",
			wantCode:  "0082",
			wantTitle: "Invalid Query Parameter",
			wantMsg:   "",
		},
		{
			name:      "limit-decimal",
			query:     "limit=10.5",
			wantCode:  "0082",
			wantTitle: "Invalid Query Parameter",
			wantMsg:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, tc.wantTitle, errResp.Title, "Expected title %q for query=%q", tc.wantTitle, tc.query)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// TestListTransactionValidations_InvalidSortParams_ReturnsError verifies invalid sort parameter handling.
// Returns 0332 (title "Invalid Sort Column") for invalid sort_by and 0081 (title "Invalid Sort Order") for invalid sort_order.
func TestListTransactionValidations_InvalidSortParams_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name      string
		query     string
		wantCode  string
		wantTitle string
		wantMsg   string
	}{
		{
			name:      "invalid-sortBy-field",
			query:     "sort_by=invalidField",
			wantCode:  "0332",
			wantTitle: "Invalid Sort Column",
			wantMsg:   "",
		},
		{
			name:      "invalid-sortOrder-value",
			query:     "sort_order=INVALID",
			wantCode:  "0081",
			wantTitle: "Invalid Sort Order",
			wantMsg:   "",
		},
		{
			name:      "sortOrder-lowercase-invalid",
			query:     "sort_order=ascending",
			wantCode:  "0081",
			wantTitle: "Invalid Sort Order",
			wantMsg:   "",
		},
		{
			name:      "sortBy-numeric",
			query:     "sort_by=123",
			wantCode:  "0332",
			wantTitle: "Invalid Sort Column",
			wantMsg:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, tc.wantTitle, errResp.Title, "Expected title %q for query=%q", tc.wantTitle, tc.query)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// TestListTransactionValidations_SortParamsWithCursor_ReturnsError verifies that sort_by/sort_order
// cannot be used together with cursor pagination.
// Returns code 0334 (title "Cursor With Sort Params") when sort parameters are provided alongside a cursor.
func TestListTransactionValidations_SortParamsWithCursor_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use a dummy cursor (base64 encoded JSON) - doesn't need to be valid,
	// since the validation happens before cursor parsing
	dummyCursor := "eyJpZCI6IjEyMzQ1Njc4LTEyMzQtMTIzNC0xMjM0LTEyMzQ1Njc4OTAxMiJ9"

	tests := []struct {
		name     string
		query    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "sortBy-with-cursor",
			query:    "cursor=" + dummyCursor + "&sort_by=created_at",
			wantCode: "0334",
			wantMsg:  "",
		},
		{
			name:     "sortOrder-with-cursor",
			query:    "cursor=" + dummyCursor + "&sort_order=ASC",
			wantCode: "0334",
			wantMsg:  "",
		},
		{
			name:     "both-sort-params-with-cursor",
			query:    "cursor=" + dummyCursor + "&sort_by=created_at&sort_order=DESC",
			wantCode: "0334",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, "Cursor With Sort Params", errResp.Title)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// =============================================================================
// 3.3 GET /v1/validations - Invalid Filter Tests
// =============================================================================

// TestListTransactionValidations_InvalidDecision_ReturnsError verifies invalid decision filter handling.
// Returns code 0431 (title "Invalid Transaction Validation Filters") for invalid filter values.
func TestListTransactionValidations_InvalidDecision_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name     string
		query    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "invalid-decision-value",
			query:    "decision=INVALID",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "decision-lowercase",
			query:    "decision=allow",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "decision-approve",
			query:    "decision=APPROVE",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "decision-numeric",
			query:    "decision=123",
			wantCode: "0431",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, "Invalid Transaction Validation Filters", errResp.Title)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// TestListTransactionValidations_InvalidTransactionType_ReturnsError verifies invalid transactionType filter handling.
// Returns code 0431 (title "Invalid Transaction Validation Filters") for invalid filter values.
func TestListTransactionValidations_InvalidTransactionType_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name     string
		query    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "invalid-transactionType-value",
			query:    "transaction_type=INVALID",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "transactionType-lowercase",
			query:    "transaction_type=card",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "transactionType-cash",
			query:    "transaction_type=CASH",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "transactionType-numeric",
			query:    "transaction_type=123",
			wantCode: "0431",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, "Invalid Transaction Validation Filters", errResp.Title)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// TestListTransactionValidations_InvalidUUIDFilters_ReturnsError verifies invalid UUID filter handling.
// Returns code 0431 (title "Invalid Transaction Validation Filters") for invalid filter values.
func TestListTransactionValidations_InvalidUUIDFilters_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name     string
		query    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "invalid-accountId-uuid",
			query:    "account_id=not-a-uuid",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "invalid-matchedRuleId-uuid",
			query:    "matched_rule_id=invalid-uuid",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "invalid-exceededLimitId-uuid",
			query:    "exceeded_limit_id=12345",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "invalid-segmentId-uuid",
			query:    "segment_id=abc",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "invalid-portfolioId-uuid",
			query:    "portfolio_id=not-valid",
			wantCode: "0431",
			wantMsg:  "",
		},
		// NOTE: "uuid-without-dashes" is actually a valid UUID format accepted by google/uuid
		// so it won't fail validation. We test with a truly invalid format instead.
		{
			name:     "malformed-uuid-bad-hex",
			query:    "account_id=550e8400-e29b-41d4-a716-44665544000g",
			wantCode: "0431",
			wantMsg:  "",
		},
		{
			name:     "partial-uuid",
			query:    "account_id=550e8400-e29b",
			wantCode: "0431",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, "Invalid Transaction Validation Filters", errResp.Title)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// TestListTransactionValidations_InvalidDateFilters_ReturnsError verifies invalid date filter handling.
// Returns 0077 (title "Invalid Date Format Error") for malformed dates and 0083
// (title "Invalid Date Range Error") when start_date is after end_date.
func TestListTransactionValidations_InvalidDateFilters_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name      string
		query     string
		wantCode  string
		wantTitle string
		wantMsg   string
	}{
		{
			name:      "invalid-startDate-format",
			query:     "start_date=2024-01-01",
			wantCode:  "0077",
			wantTitle: "Invalid Date Format Error",
			wantMsg:   "",
		},
		{
			name:      "invalid-endDate-format",
			query:     "end_date=invalid-date",
			wantCode:  "0077",
			wantTitle: "Invalid Date Format Error",
			wantMsg:   "",
		},
		{
			name:      "startDate-after-endDate",
			query:     "start_date=2024-12-31T23:59:59Z&end_date=2024-01-01T00:00:00Z",
			wantCode:  "0083",
			wantTitle: "Invalid Date Range Error",
			wantMsg:   "",
		},
		{
			name:      "date-without-timezone",
			query:     "start_date=2024-01-01T12:00:00",
			wantCode:  "0077",
			wantTitle: "Invalid Date Format Error",
			wantMsg:   "",
		},
		{
			name:      "date-numeric",
			query:     "start_date=1704067200",
			wantCode:  "0077",
			wantTitle: "Invalid Date Format Error",
			wantMsg:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?"+tc.query, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.wantCode, errResp.Code, "Expected error code %s for query=%q", tc.wantCode, tc.query)
			assert.Equal(t, tc.wantTitle, errResp.Title, "Expected title %q for query=%q", tc.wantTitle, tc.query)
			assert.Equal(t, tc.wantMsg, errResp.Message)
		})
	}
}

// =============================================================================
// 3.4 GET /v1/validations - Multiple Invalid Parameters
// =============================================================================

// TestListTransactionValidations_MultipleInvalidParams_ReturnsFirstError verifies behavior with multiple errors.
// The handler should return an error for the first invalid parameter encountered.
func TestListTransactionValidations_MultipleInvalidParams_ReturnsFirstError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// When multiple parameters are invalid, the first validation failure is returned
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?limit=-1&decision=INVALID&sort_by=badField", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0331", errResp.Code)
	assert.Equal(t, "Pagination Limit Invalid", errResp.Title)
	// First validation is limit (based on validation order in handler)
	assert.Equal(t, "", errResp.Message)
}

// =============================================================================
// 3.5 GET /v1/validations/{id} - Valid UUID but Non-existent
// =============================================================================

// TestGetTransactionValidation_ValidUUIDButNotFound_Returns404 verifies 404 for non-existent validation.
func TestGetTransactionValidation_ValidUUIDButNotFound_Returns404(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Valid UUID format but doesn't exist in database
	// NOTE: The nil UUID (all zeros) is treated as invalid by the service, so we use a different non-existent UUID
	nonExistentID := "12345678-1234-1234-1234-123456789012"

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations/"+nonExistentID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0432", errResp.Code)
	assert.Equal(t, "Transaction Validation Not Found", errResp.Title)
	assert.Equal(t, "", errResp.Message)
}

// =============================================================================
// 3.6 GET /v1/validations - Authentication Tests
// =============================================================================

// TestGetTransactionValidation_WithoutAuth_Returns401 verifies authentication requirement.
func TestGetTransactionValidation_WithoutAuth_Returns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations/550e8400-e29b-41d4-a716-446655440000", nil)
	require.NoError(t, err)
	// No X-API-Key header set

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// TestListTransactionValidations_WithoutAuth_Returns401 verifies authentication requirement.
func TestListTransactionValidations_WithoutAuth_Returns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations", nil)
	require.NoError(t, err)
	// No X-API-Key header set

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// =============================================================================
// Pagination Validation Tests - Transaction Validations
// =============================================================================
// These tests verify that pagination parameters are properly validated according
// to the standardized validation pattern (codes 0080, 0081, 0331, 0332, 0334).
// =============================================================================

// TestListTransactionValidations_CursorWithSortBy_Rejected verifies code 0334: cursor cannot be used with sort_by.
func TestListTransactionValidations_CursorWithSortBy_Rejected(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?cursor=eyJpZCI6InRlc3QiLCJwbiI6dHJ1ZX0=&sort_by=created_at", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0334", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_CursorWithSortOrder_Rejected verifies code 0334: cursor cannot be used with sort_order.
func TestListTransactionValidations_CursorWithSortOrder_Rejected(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?cursor=eyJpZCI6InRlc3QiLCJwbiI6dHJ1ZX0=&sort_order=ASC", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0334", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_CursorWithBothSortParams_Rejected verifies code 0334: cursor cannot be used with both sort_by and sort_order.
func TestListTransactionValidations_CursorWithBothSortParams_Rejected(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?cursor=eyJpZCI6InRlc3QiLCJwbiI6dHJ1ZX0=&sort_by=created_at&sort_order=ASC", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0334", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_LimitExceeded verifies code 0080: limit cannot exceed maximum.
func TestListTransactionValidations_LimitExceeded(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?limit=1001", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0080", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_InvalidLimit verifies code 0331: limit must be at least 1.
func TestListTransactionValidations_InvalidLimit(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?limit=0", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0331", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_InvalidSortOrder_TRC0042 verifies code 0081: sort_order must be ASC or DESC.
func TestListTransactionValidations_InvalidSortOrder_TRC0042(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?sort_order=INVALID", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0081", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// TestListTransactionValidations_InvalidSortBy_TRC0043 verifies code 0332: sort_by must be in allowed list.
func TestListTransactionValidations_InvalidSortBy_TRC0043(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations?sort_by=invalidField", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0332", errResp.Code)
	assert.Empty(t, errResp.Message)
}

// =============================================================================
// 3.7 GET /v1/validations - Timeout/DeadlineExceeded Tests (code 0433)
// =============================================================================

// TestListTransactionValidations_Timeout_ReturnsTRC0252 verifies that when a query timeout occurs
// (deadline exceeded), the API returns 504 Gateway Timeout with error code 0433.
//
// Uses fault injection to simulate a timeout scenario. The fault injection middleware
// intercepts requests with X-Test-Fault-Injection header and returns code 0433.
//
// Expected response:
// - HTTP Status: 504 Gateway Timeout
// - Error Code: 0433 (list query timeout)
// - Error Title: "Gateway Timeout"
// - Error Message: mentions "query" to distinguish from the POST validation timeout
//
// KNOWN BUG (not synced): the endpoint currently returns 503 "Service Unavailable"
// with detail "internal error" instead of 504 "Gateway Timeout". Assertions are
// left expecting the correct 504 contract.
func TestListTransactionValidations_Timeout_ReturnsTRC0252(t *testing.T) {
	// Use fault injection to simulate timeout scenario
	resp, respBody := testutil.ListValidationsWithFaultInjection(t, "", testutil.FaultTimeout)
	defer resp.Body.Close()

	// Verify HTTP status is 504 Gateway Timeout
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"Expected 504 Gateway Timeout for deadline exceeded, got %d. Response: %s",
		resp.StatusCode, string(respBody))

	// Parse the error response
	errResp := testutil.ParseErrorResponse(t, respBody)

	// Verify code 0433 (ErrListValidationsTimeout) for list query timeout, distinct from the POST validation timeout
	assert.Equal(t, "0433", errResp.Code,
		"Expected error code 0433 (ErrListValidationsTimeout) for list query timeout, got %s",
		errResp.Code)

	// Verify the title indicates a timeout
	assert.Equal(t, "Gateway Timeout", errResp.Title,
		"Expected title 'Gateway Timeout' for deadline exceeded")

	// Verify the message mentions the list operation timeout
	assert.Contains(t, errResp.Message, "query",
		"Error message should mention 'query' to distinguish from validation timeout")
}

// TestListTransactionValidations_Timeout_WithFilters_ReturnsTRC0252 verifies timeout handling
// when complex filters are used (which might cause slower queries).
//
// Same KNOWN BUG as TestListTransactionValidations_Timeout_ReturnsTRC0252: the endpoint
// currently returns 503 "Service Unavailable" instead of the expected 504 "Gateway Timeout".
func TestListTransactionValidations_Timeout_WithFilters_ReturnsTRC0252(t *testing.T) {
	// Use filters that might cause a complex query
	queryParams := "decision=ALLOW&transaction_type=CARD&sort_by=processing_time_ms&sort_order=DESC"

	resp, respBody := testutil.ListValidationsWithFaultInjection(t, queryParams, testutil.FaultTimeout)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"Expected 504 Gateway Timeout, got %d. Response: %s",
		resp.StatusCode, string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	assert.Equal(t, "0433", errResp.Code,
		"Expected code 0433 for list query timeout, got %s", errResp.Code)
}
