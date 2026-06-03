// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Error Code Tests for Rule Handler
// =============================================================================
// These tests verify the error codes returned by rule handler operations.
//
// Error code mapping:
//   - Malformed JSON/invalid request body → TRC-0003 (Bad Request)
//   - Validation errors (invalid field values) → TRC-0001 (Validation Error)
// =============================================================================

// =============================================================================
// POST /v1/rules - Create Rule Error Code Tests
// =============================================================================

// TestCreateRule_MalformedJSON_ReturnsError verifies that completely malformed JSON returns an error.
// Returns TRC-0003 for malformed JSON/invalid request body
func TestCreateRule_MalformedJSON_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		body        string
		description string
	}{
		{
			name:        "missing_quotes_on_key",
			body:        `{name: "test rule", expression: "true", action: "ALLOW"}`,
			description: "JSON with unquoted keys",
		},
		{
			name:        "trailing_comma",
			body:        `{"name": "test rule", "expression": "true", "action": "ALLOW",}`,
			description: "JSON with trailing comma",
		},
		{
			name:        "unclosed_brace",
			body:        `{"name": "test rule", "expression": "true", "action": "ALLOW"`,
			description: "JSON with unclosed brace",
		},
		{
			name:        "single_quotes",
			body:        `{'name': 'test rule', 'expression': 'true', 'action': 'ALLOW'}`,
			description: "JSON with single quotes instead of double quotes",
		},
		{
			name:        "random_text",
			body:        `this is not json at all`,
			description: "Plain text instead of JSON",
		},
		{
			name:        "xml_instead_of_json",
			body:        `<rule><name>test</name></rule>`,
			description: "XML instead of JSON",
		},
		{
			name:        "incomplete_json_object",
			body:        `{"name": "test`,
			description: "Incomplete JSON string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			assert.Equal(t, "TRC-0003", errResp.Code, "Test case: %s - Expected TRC-0003 for malformed JSON", tc.description)
			assert.Equal(t, "Bad Request", errResp.Title, "Test case: %s", tc.description)
			assert.Equal(t, "Invalid request body", errResp.Message, "Test case: %s", tc.description)
		})
	}
}

// TestCreateRule_EmptyBody_ReturnsError verifies that an empty request body returns an error.
// Returns TRC-0003 for empty request body
func TestCreateRule_EmptyBody_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(""))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	assert.Equal(t, "TRC-0003", errResp.Code, "Expected TRC-0003 for empty body")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Invalid request body", errResp.Message)
}

// TestCreateRule_NullBody_ReturnsError verifies that a null JSON body returns an error.
// Returns TRC-0001 for null JSON body (valid JSON but missing required fields)
func TestCreateRule_NullBody_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader("null"))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// Note: "null" is valid JSON but missing required fields, so it's treated as a validation error
	// With specific error codes, the first missing required field error is returned
	assert.Equal(t, "TRC-0106", errResp.Code, "Expected TRC-0106 for null body (name is required)")
	assert.Equal(t, "Validation Error", errResp.Title)
	assert.Equal(t, "name is required", errResp.Message)
}

// TestCreateRule_ArrayInsteadOfObject_ReturnsError verifies that a JSON array instead of object returns an error.
// Returns TRC-0003 for array instead of object
func TestCreateRule_ArrayInsteadOfObject_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Valid JSON but wrong type (array instead of object)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(`[{"name": "test"}]`))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	assert.Equal(t, "TRC-0003", errResp.Code, "Expected TRC-0003 for array instead of object")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Invalid request body", errResp.Message)
}

// TestCreateRule_BinaryData_ReturnsError verifies that binary data returns an error.
// Returns TRC-0003 for binary data (not valid JSON)
func TestCreateRule_BinaryData_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Binary data that's not valid JSON
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(binaryData))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	assert.Equal(t, "TRC-0003", errResp.Code, "Expected TRC-0003 for binary data")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Invalid request body", errResp.Message)
}

// =============================================================================
// PATCH /v1/rules/{ruleId} - Update Rule Error Code Tests
// =============================================================================

// TestUpdateRule_InvalidBody_ReturnsError consolidates all PATCH body validation tests.
// Creates a single rule and runs all subtests against it for better isolation and performance.
func TestUpdateRule_InvalidBody_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create a single rule for all PATCH tests
	ruleID := testutil.CreateTestRuleWithExpression(t, "rule for patch error tests "+testutil.RandomSuffix(), "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Helper function to execute PATCH request and return response
	doPatchRequest := func(t *testing.T, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, respBody
	}

	// Subtest: Malformed JSON cases (TRC-0003)
	t.Run("malformed_json", func(t *testing.T) {
		malformedCases := []struct {
			name        string
			body        string
			description string
		}{
			{
				name:        "missing_quotes_on_key",
				body:        `{name: "updated rule"}`,
				description: "JSON with unquoted keys",
			},
			{
				name:        "trailing_comma",
				body:        `{"name": "updated rule",}`,
				description: "JSON with trailing comma",
			},
			{
				name:        "unclosed_brace",
				body:        `{"name": "updated rule"`,
				description: "JSON with unclosed brace",
			},
			{
				name:        "invalid_json",
				body:        `not valid json`,
				description: "Plain text instead of JSON",
			},
		}

		for _, tc := range malformedCases {
			t.Run(tc.name, func(t *testing.T) {
				statusCode, respBody := doPatchRequest(t, tc.body)

				assert.Equal(t, http.StatusBadRequest, statusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "TRC-0003", errResp.Code, "Test case: %s - Expected TRC-0003 for malformed JSON", tc.description)
				assert.Equal(t, "Bad Request", errResp.Title, "Test case: %s", tc.description)
				assert.Equal(t, "Invalid request body", errResp.Message, "Test case: %s", tc.description)
			})
		}
	})

	// Subtest: Empty body (TRC-0003)
	t.Run("empty_body", func(t *testing.T) {
		statusCode, respBody := doPatchRequest(t, "")

		assert.Equal(t, http.StatusBadRequest, statusCode, "Response: %s", string(respBody))

		errResp := testutil.ParseErrorResponse(t, respBody)
		assert.Equal(t, "TRC-0003", errResp.Code, "Expected TRC-0003 for empty body")
		assert.Equal(t, "Bad Request", errResp.Title)
		assert.Equal(t, "Invalid request body", errResp.Message)
	})

	// Subtest: Empty JSON object (TRC-0002)
	t.Run("empty_json_object", func(t *testing.T) {
		statusCode, respBody := doPatchRequest(t, "{}")

		assert.Equal(t, http.StatusBadRequest, statusCode, "Response: %s", string(respBody))

		errResp := testutil.ParseErrorResponse(t, respBody)
		// TRC-0002 for missing required fields (empty object has no fields to update)
		assert.Equal(t, "TRC-0002", errResp.Code, "Expected TRC-0002 for empty object (missing required fields)")
		assert.Equal(t, "Validation Error", errResp.Title)
		assert.Equal(t, "At least one field must be provided for update", errResp.Message)
	})
}

// =============================================================================
// GET /v1/rules - List Rules Error Code Tests
// =============================================================================

// TestListRules_InvalidQueryParameters_ReturnsError verifies error codes for invalid query parameters.
// Tests various validation scenarios for ListRules endpoint.
func TestListRules_InvalidQueryParameters_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name          string
		queryParams   string
		expectedCode  string
		expectedTitle string
		description   string
	}{
		{
			name:          "invalid_action_filter_lowercase",
			queryParams:   "action=allow",
			expectedCode:  "TRC-0006",
			expectedTitle: "Bad Request",
			description:   "Action filter with lowercase value",
		},
		{
			name:          "invalid_action_filter_value",
			queryParams:   "action=INVALID_ACTION",
			expectedCode:  "TRC-0006",
			expectedTitle: "Bad Request",
			description:   "Action filter with invalid enum value",
		},
		{
			name:          "invalid_status_filter_lowercase",
			queryParams:   "status=draft",
			expectedCode:  "TRC-0006",
			expectedTitle: "Bad Request",
			description:   "Status filter with lowercase value",
		},
		{
			name:          "invalid_status_filter_value",
			queryParams:   "status=INVALID_STATUS",
			expectedCode:  "TRC-0006",
			expectedTitle: "Bad Request",
			description:   "Status filter with invalid enum value",
		},
		{
			name:          "deleted_status_not_allowed",
			queryParams:   "status=DELETED",
			expectedCode:  "TRC-0006",
			expectedTitle: "Bad Request",
			description:   "DELETED status is not allowed in filter",
		},
		{
			name:          "limit_zero",
			queryParams:   "limit=0",
			expectedCode:  "TRC-0041",
			expectedTitle: "Bad Request",
			description:   "Limit of 0 is invalid",
		},
		{
			name:          "limit_negative",
			queryParams:   "limit=-1",
			expectedCode:  "TRC-0041",
			expectedTitle: "Bad Request",
			description:   "Negative limit is invalid",
		},
		{
			name:          "limit_exceeds_max",
			queryParams:   "limit=101",
			expectedCode:  "TRC-0040",
			expectedTitle: "Bad Request",
			description:   "Limit exceeds maximum of 100",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.expectedCode, errResp.Code, "Test case: %s - Expected %s", tc.description, tc.expectedCode)
			assert.Equal(t, tc.expectedTitle, errResp.Title, "Test case: %s", tc.description)
		})
	}
}

// TestListRules_ValidLimitBoundary_Succeeds verifies that valid boundary limit values succeed.
func TestListRules_ValidLimitBoundary_Succeeds(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		queryParams string
		description string
	}{
		{
			name:        "limit_min",
			queryParams: "limit=1",
			description: "Minimum valid limit",
		},
		{
			name:        "limit_max",
			queryParams: "limit=100",
			description: "Maximum valid limit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))
		})
	}
}

// TestListRules_InvalidCursor_ReturnsError verifies that invalid cursor returns proper error.
func TestListRules_InvalidCursor_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name         string
		cursor       string
		expectedCode string
		description  string
	}{
		{
			name:         "random_string",
			cursor:       "not-a-valid-cursor",
			expectedCode: "TRC-0044",
			description:  "Random string as cursor",
		},
		{
			name:         "invalid_base64",
			cursor:       "!!invalid-base64!!",
			expectedCode: "TRC-0044",
			description:  "Invalid base64 string",
		},
		{
			name:         "malformed_cursor",
			cursor:       "YWJjZGVm", // valid base64 but not a valid cursor format
			expectedCode: "TRC-0044",
			description:  "Valid base64 but invalid cursor format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?cursor="+tc.cursor, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.expectedCode, errResp.Code, "Test case: %s - Expected %s", tc.description, tc.expectedCode)
			assert.Equal(t, "Bad Request", errResp.Title, "Test case: %s", tc.description)
			assert.Contains(t, errResp.Message, "cursor", "Test case: %s - Error should mention cursor", tc.description)
		})
	}
}

// TestListRules_InvalidSortParameters_ReturnsError verifies that invalid sort parameters return proper errors.
func TestListRules_InvalidSortParameters_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name            string
		queryParams     string
		expectedCode    string
		expectedTitle   string
		expectedMessage string
		description     string
	}{
		{
			name:            "invalid_sort_column",
			queryParams:     "sort_by=invalid_column",
			expectedCode:    "TRC-0043",
			expectedTitle:   "Bad Request",
			expectedMessage: "sort_by must be one of [created_at updated_at name status]",
			description:     "Invalid sort column",
		},
		{
			name:            "invalid_sort_order",
			queryParams:     "sort_order=INVALID",
			expectedCode:    "TRC-0042",
			expectedTitle:   "Bad Request",
			expectedMessage: "sort_order must be ASC or DESC",
			description:     "Invalid sort order",
		},
		// NOTE: lowercase sort orders (asc, desc) are accepted and normalized by the API,
		// so they don't produce errors. We only test truly invalid values.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?"+tc.queryParams, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.expectedCode, errResp.Code, "Test case: %s - Expected %s", tc.description, tc.expectedCode)
			assert.Equal(t, tc.expectedTitle, errResp.Title, "Test case: %s", tc.description)
			assert.Equal(t, tc.expectedMessage, errResp.Message, "Test case: %s", tc.description)
		})
	}
}

// =============================================================================
// Edge Case Tests for Error Handling
// =============================================================================

// TestCreateRule_ValidJSONWithInvalidTypes_ReturnsBadRequest verifies that
// valid JSON with wrong types returns TRC-0003 (Bad Request) due to JSON type mismatch.
func TestCreateRule_ValidJSONWithInvalidTypes_ReturnsBadRequest(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "number_for_name",
			payload: map[string]any{
				"name":       123,
				"expression": "true",
				"action":     "ALLOW",
			},
		},
		{
			name: "boolean_for_expression",
			payload: map[string]any{
				"name":       "test rule",
				"expression": true,
				"action":     "ALLOW",
			},
		},
		{
			name: "array_for_action",
			payload: map[string]any{
				"name":       "test rule",
				"expression": "true",
				"action":     []string{"ALLOW"},
			},
		},
		{
			name: "object_for_name",
			payload: map[string]any{
				"name":       map[string]string{"value": "test"},
				"expression": "true",
				"action":     "ALLOW",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// TRC-0003 is returned for JSON type mismatch errors (bad request)
			assert.Equal(t, "TRC-0003", errResp.Code, "Expected TRC-0003 for type validation error")
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid request body", errResp.Message)
		})
	}
}

// TestCreateRule_LargePayload_HandlesCorrectly verifies handling of very large payloads.
func TestCreateRule_LargePayload_HandlesCorrectly(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create a very large name (much larger than max allowed 255)
	largeName := strings.Repeat("a", 10000)

	payload := map[string]any{
		"name":       largeName,
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// Validation error for name exceeding max length
	assert.Equal(t, "TRC-0107", errResp.Code, "Expected TRC-0107 for name too long")
	assert.Equal(t, "Validation Error", errResp.Title)
	assert.Contains(t, errResp.Message, "name")
}
