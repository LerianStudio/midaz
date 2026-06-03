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

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Error Code Tests for Transaction Validation Handler
// =============================================================================
//
// These tests document the current error handling behavior for the transaction
// validation endpoints (GET /v1/validations and GET /v1/validations/{id}).
//
// Current Behavior:
// - Invalid path parameter (UUID) returns TRC-0007 (ErrInvalidPathParameter)
//   with title "Invalid Path Parameter"
// - Invalid query parameter (non-numeric, malformed) returns TRC-0006
//   (ErrInvalidQueryParameter) with title "Invalid Query Parameter"
// - Invalid filter values (out of range, invalid enum) returns TRC-0250
//   (ErrInvalidTransactionValidationFilters) with title "Invalid Transaction Validation Filters"
//
// NOTE: These tests document CURRENT behavior to establish a baseline for
// future error code improvements. Update expected codes when the handler is fixed.
// =============================================================================

// =============================================================================
// 3.1 GET /v1/validations/{id} - Invalid ID Tests
// =============================================================================

// TestGetTransactionValidation_InvalidID_ReturnsError verifies invalid UUID handling in path parameter.
// Returns TRC-0007 for invalid path parameters
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
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
		},
		{
			name:     "numeric-id",
			id:       "12345",
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
		},
		{
			name:     "partial-uuid",
			id:       "550e8400-e29b-41d4",
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
		},
		{
			name:     "uuid-with-extra-chars",
			id:       "550e8400-e29b-41d4-a716-446655440000-extra",
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
		},
		{
			name:     "uppercase-invalid-uuid",
			id:       "NOT-A-UUID-FORMAT",
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
		},
		{
			name:     "malformed-uuid-bad-chars",
			id:       "550e8400-e29b-41d4-a716-44665544000g",
			wantCode: "TRC-0007",
			wantMsg:  "Invalid transaction validation ID format",
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
// Returns TRC-0006 for invalid query parameters (non-numeric limit, decimal)
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
			wantCode:  "TRC-0041",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "limit must be at least 1",
		},
		{
			name:      "limit-negative",
			query:     "limit=-1",
			wantCode:  "TRC-0041",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "limit must be at least 1",
		},
		{
			name:      "limit-exceeds-max",
			query:     "limit=1001",
			wantCode:  "TRC-0040",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "limit must not exceed 1000",
		},
		{
			name:      "limit-non-numeric",
			query:     "limit=abc",
			wantCode:  "TRC-0006",
			wantTitle: "Invalid Query Parameter",
			wantMsg:   "Invalid query parameters",
		},
		{
			name:      "limit-decimal",
			query:     "limit=10.5",
			wantCode:  "TRC-0006",
			wantTitle: "Invalid Query Parameter",
			wantMsg:   "Invalid query parameters",
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
// Returns TRC-0043 for invalid sort_by field and TRC-0042 for invalid sort_order value.
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
			wantCode:  "TRC-0043",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "sort_by must be one of [created_at processing_time_ms]",
		},
		{
			name:      "invalid-sortOrder-value",
			query:     "sort_order=INVALID",
			wantCode:  "TRC-0042",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "sort_order must be ASC or DESC",
		},
		{
			name:      "sortOrder-lowercase-invalid",
			query:     "sort_order=ascending",
			wantCode:  "TRC-0042",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "sort_order must be ASC or DESC",
		},
		{
			name:      "sortBy-numeric",
			query:     "sort_by=123",
			wantCode:  "TRC-0043",
			wantTitle: "Invalid Transaction Validation Filters",
			wantMsg:   "sort_by must be one of [created_at processing_time_ms]",
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
// Returns TRC-0045 when sort parameters are provided alongside a cursor.
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
			wantCode: "TRC-0045",
			wantMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name:     "sortOrder-with-cursor",
			query:    "cursor=" + dummyCursor + "&sort_order=ASC",
			wantCode: "TRC-0045",
			wantMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name:     "both-sort-params-with-cursor",
			query:    "cursor=" + dummyCursor + "&sort_by=created_at&sort_order=DESC",
			wantCode: "TRC-0045",
			wantMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
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

// =============================================================================
// 3.3 GET /v1/validations - Invalid Filter Tests
// =============================================================================

// TestListTransactionValidations_InvalidDecision_ReturnsError verifies invalid decision filter handling.
// Returns TRC-0250 for invalid transaction validation filters
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
			wantCode: "TRC-0250",
			wantMsg:  "decision must be one of [ALLOW, DENY, REVIEW]",
		},
		{
			name:     "decision-lowercase",
			query:    "decision=allow",
			wantCode: "TRC-0250",
			wantMsg:  "decision must be one of [ALLOW, DENY, REVIEW]",
		},
		{
			name:     "decision-approve",
			query:    "decision=APPROVE",
			wantCode: "TRC-0250",
			wantMsg:  "decision must be one of [ALLOW, DENY, REVIEW]",
		},
		{
			name:     "decision-numeric",
			query:    "decision=123",
			wantCode: "TRC-0250",
			wantMsg:  "decision must be one of [ALLOW, DENY, REVIEW]",
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
// Returns TRC-0250 for invalid transaction validation filters
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
			wantCode: "TRC-0250",
			wantMsg:  "transaction_type must be one of [CARD, WIRE, PIX, CRYPTO]",
		},
		{
			name:     "transactionType-lowercase",
			query:    "transaction_type=card",
			wantCode: "TRC-0250",
			wantMsg:  "transaction_type must be one of [CARD, WIRE, PIX, CRYPTO]",
		},
		{
			name:     "transactionType-cash",
			query:    "transaction_type=CASH",
			wantCode: "TRC-0250",
			wantMsg:  "transaction_type must be one of [CARD, WIRE, PIX, CRYPTO]",
		},
		{
			name:     "transactionType-numeric",
			query:    "transaction_type=123",
			wantCode: "TRC-0250",
			wantMsg:  "transaction_type must be one of [CARD, WIRE, PIX, CRYPTO]",
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
// Returns TRC-0250 for invalid transaction validation filters
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
			wantCode: "TRC-0250",
			wantMsg:  "account_id must be a valid UUID",
		},
		{
			name:     "invalid-matchedRuleId-uuid",
			query:    "matched_rule_id=invalid-uuid",
			wantCode: "TRC-0250",
			wantMsg:  "matched_rule_id must be a valid UUID",
		},
		{
			name:     "invalid-exceededLimitId-uuid",
			query:    "exceeded_limit_id=12345",
			wantCode: "TRC-0250",
			wantMsg:  "exceeded_limit_id must be a valid UUID",
		},
		{
			name:     "invalid-segmentId-uuid",
			query:    "segment_id=abc",
			wantCode: "TRC-0250",
			wantMsg:  "segment_id must be a valid UUID",
		},
		{
			name:     "invalid-portfolioId-uuid",
			query:    "portfolio_id=not-valid",
			wantCode: "TRC-0250",
			wantMsg:  "portfolio_id must be a valid UUID",
		},
		// NOTE: "uuid-without-dashes" is actually a valid UUID format accepted by google/uuid
		// so it won't fail validation. We test with a truly invalid format instead.
		{
			name:     "malformed-uuid-bad-hex",
			query:    "account_id=550e8400-e29b-41d4-a716-44665544000g",
			wantCode: "TRC-0250",
			wantMsg:  "account_id must be a valid UUID",
		},
		{
			name:     "partial-uuid",
			query:    "account_id=550e8400-e29b",
			wantCode: "TRC-0250",
			wantMsg:  "account_id must be a valid UUID",
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
// Returns TRC-0250 for invalid transaction validation filters
func TestListTransactionValidations_InvalidDateFilters_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name     string
		query    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "invalid-startDate-format",
			query:    "start_date=2024-01-01",
			wantCode: "TRC-0020",
			wantMsg:  `start_date must be in RFC3339 format with timezone (e.g., 2026-01-28T10:30:00Z). Invalid value: "2024-01-01"`,
		},
		{
			name:     "invalid-endDate-format",
			query:    "end_date=invalid-date",
			wantCode: "TRC-0020",
			wantMsg:  `end_date must be in RFC3339 format with timezone (e.g., 2026-01-28T10:30:00Z). Invalid value: "invalid-date"`,
		},
		{
			name:     "startDate-after-endDate",
			query:    "start_date=2024-12-31T23:59:59Z&end_date=2024-01-01T00:00:00Z",
			wantCode: "TRC-0023",
			wantMsg:  "end_date must be on or after start_date",
		},
		{
			name:     "date-without-timezone",
			query:    "start_date=2024-01-01T12:00:00",
			wantCode: "TRC-0020",
			wantMsg:  `start_date must be in RFC3339 format with timezone (e.g., 2026-01-28T10:30:00Z). Invalid value: "2024-01-01T12:00:00"`,
		},
		{
			name:     "date-numeric",
			query:    "start_date=1704067200",
			wantCode: "TRC-0020",
			wantMsg:  `start_date must be in RFC3339 format with timezone (e.g., 2026-01-28T10:30:00Z). Invalid value: "1704067200"`,
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
	assert.Equal(t, "TRC-0041", errResp.Code)
	assert.Equal(t, "Invalid Transaction Validation Filters", errResp.Title)
	// First validation is limit (based on validation order in handler)
	assert.Equal(t, "limit must be at least 1", errResp.Message)
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
	assert.Equal(t, "TRC-0251", errResp.Code)
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Transaction validation not found", errResp.Message)
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
// to the standardized validation pattern (TRC-0040 through TRC-0045).
// =============================================================================

// TestListTransactionValidations_CursorWithSortBy_Rejected verifies TRC-0045: cursor cannot be used with sort_by.
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
	assert.Equal(t, "TRC-0045", errResp.Code)
	assert.Contains(t, errResp.Message, "sort_by and sort_order cannot be used with cursor")
}

// TestListTransactionValidations_CursorWithSortOrder_Rejected verifies TRC-0045: cursor cannot be used with sort_order.
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
	assert.Equal(t, "TRC-0045", errResp.Code)
	assert.Contains(t, errResp.Message, "sort_by and sort_order cannot be used with cursor")
}

// TestListTransactionValidations_CursorWithBothSortParams_Rejected verifies TRC-0045: cursor cannot be used with both sort_by and sort_order.
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
	assert.Equal(t, "TRC-0045", errResp.Code)
	assert.Contains(t, errResp.Message, "sort_by and sort_order cannot be used with cursor")
}

// TestListTransactionValidations_LimitExceeded verifies TRC-0040: limit cannot exceed maximum.
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
	assert.Equal(t, "TRC-0040", errResp.Code)
	assert.Contains(t, errResp.Message, "limit must not exceed 1000")
}

// TestListTransactionValidations_InvalidLimit verifies TRC-0041: limit must be at least 1.
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
	assert.Equal(t, "TRC-0041", errResp.Code)
	assert.Contains(t, errResp.Message, "limit must be at least 1")
}

// TestListTransactionValidations_InvalidSortOrder_TRC0042 verifies TRC-0042: sort_order must be ASC or DESC.
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
	assert.Equal(t, "TRC-0042", errResp.Code)
	assert.Contains(t, errResp.Message, "sort_order must be ASC or DESC")
}

// TestListTransactionValidations_InvalidSortBy_TRC0043 verifies TRC-0043: sort_by must be in allowed list.
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
	assert.Equal(t, "TRC-0043", errResp.Code)
	assert.Contains(t, errResp.Message, "sort_by must be one of [created_at processing_time_ms]")
}

// =============================================================================
// 3.7 GET /v1/validations - Timeout/DeadlineExceeded Tests (TRC-0252)
// =============================================================================

// TestListTransactionValidations_Timeout_ReturnsTRC0252 verifies that when a query timeout occurs
// (deadline exceeded), the API returns 504 Gateway Timeout with error code TRC-0252.
//
// Uses fault injection to simulate a timeout scenario. The fault injection middleware
// intercepts requests with X-Test-Fault-Injection header and returns TRC-0252.
//
// Expected response:
// - HTTP Status: 504 Gateway Timeout
// - Error Code: TRC-0252 (list query timeout)
// - Error Title: "Gateway Timeout"
// - Error Message: mentions "query" to distinguish from POST validation timeout (TRC-0229)
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

	// Verify TRC-0252 for list query timeout (distinct from TRC-0229 for POST validation timeout)
	assert.Equal(t, "TRC-0252", errResp.Code,
		"Expected error code TRC-0252 (CodeListValidationsTimeout) for list query timeout, got %s",
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
// TDD RED Phase: Same failure expected as TestListTransactionValidations_Timeout_ReturnsTRC0252.
func TestListTransactionValidations_Timeout_WithFilters_ReturnsTRC0252(t *testing.T) {
	// Use filters that might cause a complex query
	queryParams := "decision=ALLOW&transaction_type=CARD&sort_by=processing_time_ms&sort_order=DESC"

	resp, respBody := testutil.ListValidationsWithFaultInjection(t, queryParams, testutil.FaultTimeout)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"Expected 504 Gateway Timeout, got %d. Response: %s",
		resp.StatusCode, string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// TDD RED Phase: Will fail - expecting TRC-0252, will get TRC-0229
	assert.Equal(t, "TRC-0252", errResp.Code,
		"Expected TRC-0252 for list query timeout, got %s", errResp.Code)
}
