// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// getStringField safely extracts a string field from a map, failing the test with a clear message if not found or wrong type.
func getStringField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key]
	require.True(t, ok, "field %q not found in response", key)
	s, ok := v.(string)
	require.True(t, ok, "field %q should be a string, got %T", key, v)
	return s
}

// =============================================================================
// 2.1 POST /v1/rules - Create Rule
// =============================================================================

// TestCreateRule_2_1_1_WithCompletePayload verifies rule creation with all fields populated.
func TestCreateRule_2_1_1_WithCompletePayload(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":        "High value transaction rule",
		"description": "Deny transactions over $10,000",
		"expression":  "amount > 1000000",
		"action":      "DENY",
		"scopes": []map[string]any{
			{
				"transactionType": "CARD",
				"accountId":       "550e8400-e29b-41d4-a716-446655440000",
			},
		},
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate ruleId (UUID format)
	ruleID, ok := result["ruleId"].(string)
	assert.True(t, ok, "ruleId should be a string")
	assert.Len(t, ruleID, 36, "ruleId should be a valid UUID (36 chars)")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, ruleID)

	// Validate fields (name is normalized to lowercase by server)
	assert.Equal(t, "high value transaction rule", result["name"])
	assert.Equal(t, "Deny transactions over $10,000", result["description"])
	assert.Equal(t, "amount > 1000000", result["expression"])
	assert.Equal(t, "DENY", result["action"])
	assert.Equal(t, "DRAFT", result["status"])

	// Validate scopes
	scopes, ok := result["scopes"].([]any)
	require.True(t, ok, "scopes should be an array")
	require.Len(t, scopes, 1)
	scope0, ok := scopes[0].(map[string]any)
	require.True(t, ok, "scope should be a map")
	assert.Equal(t, "CARD", scope0["transactionType"])
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", scope0["accountId"])

	// Validate timestamps
	assert.NotEmpty(t, result["createdAt"])
	assert.NotEmpty(t, result["updatedAt"])
	assert.Nil(t, result["deletedAt"])

	// Cleanup
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})
}

// TestCreateRule_2_1_2_WithMinimalPayload verifies rule creation with only required fields.
func TestCreateRule_2_1_2_WithMinimalPayload(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "Minimal rule",
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	ruleID := getStringField(t, result, "ruleId")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, ruleID)
	assert.Equal(t, "minimal rule", result["name"]) // normalized to lowercase
	assert.Nil(t, result["description"])
	assert.Equal(t, "true", result["expression"])
	assert.Equal(t, "ALLOW", result["action"])

	// Scopes should always be an array (empty when not provided)
	scopes, ok := result["scopes"].([]any)
	assert.True(t, ok, "scopes should be an array, got: %v", result["scopes"])
	assert.Empty(t, scopes)

	assert.Equal(t, "DRAFT", result["status"])
	assert.NotEmpty(t, result["createdAt"])
	assert.NotEmpty(t, result["updatedAt"])

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})
}

// TestCreateRule_2_1_3_WithMultipleScopes verifies rule creation with multiple scope definitions.
func TestCreateRule_2_1_3_WithMultipleScopes(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "Multi-scope rule",
		"expression": "amount > 500000",
		"action":     "REVIEW",
		"scopes": []map[string]any{
			{
				"transactionType": "CARD",
				"accountId":       "550e8400-e29b-41d4-a716-446655440001",
			},
			{
				"transactionType": "PIX",
				"merchantId":      "550e8400-e29b-41d4-a716-446655440002",
			},
			{
				"segmentId": "550e8400-e29b-41d4-a716-446655440003",
			},
		},
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	ruleID := getStringField(t, result, "ruleId")

	scopes, ok := result["scopes"].([]any)
	require.True(t, ok, "scopes should be an array")
	require.Len(t, scopes, 3)

	scope0, ok := scopes[0].(map[string]any)
	require.True(t, ok, "scope should be a map")
	assert.Equal(t, "CARD", scope0["transactionType"])
	assert.NotNil(t, scope0["accountId"])

	scope1, ok := scopes[1].(map[string]any)
	require.True(t, ok, "scope should be a map")
	assert.Equal(t, "PIX", scope1["transactionType"])
	assert.NotNil(t, scope1["merchantId"])

	scope2, ok := scopes[2].(map[string]any)
	require.True(t, ok, "scope should be a map")
	assert.NotNil(t, scope2["segmentId"])

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})
}

// TestCreateRule_2_1_4_WithEachActionType verifies all action enum values are accepted.
func TestCreateRule_2_1_4_WithEachActionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	actions := []string{"ALLOW", "DENY", "REVIEW"}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Action " + action + " rule",
				"expression": "true",
				"action":     action,
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

			var result map[string]any
			err = json.Unmarshal(respBody, &result)
			require.NoError(t, err)

			assert.Equal(t, action, result["action"])

			ruleID := getStringField(t, result, "ruleId")
			t.Cleanup(func() {
				testutil.CleanupRule(t, ruleID)
			})
		})
	}
}

// TestCreateRule_2_1_5_RejectsMissingName verifies validation of required field 'name'.
func TestCreateRule_2_1_5_RejectsMissingName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0353", errResp.Code, "Error code should be TRC-0106 for name required")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "name is required", errResp.Message, "Error message should indicate name is required")
}

// TestCreateRule_2_1_6_RejectsMissingExpression verifies validation of required field 'expression'.
func TestCreateRule_2_1_6_RejectsMissingExpression(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":   "Rule without expression",
		"action": "DENY",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0355", errResp.Code, "Error code should be TRC-0108 for expression required")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "expression is required", errResp.Message, "Error message should indicate expression is required")
}

// TestCreateRule_2_1_7_RejectsMissingAction verifies validation of required field 'action'.
func TestCreateRule_2_1_7_RejectsMissingAction(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "Rule without action",
		"expression": "true",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0357", errResp.Code, "Error code should be TRC-0110 for action required/invalid")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "action is required and must be one of [ALLOW, DENY, REVIEW]", errResp.Message, "Error message should indicate action is required")
}

// TestCreateRule_2_1_8_RejectsAllMissingRequiredFields verifies validation when multiple required fields are missing.
func TestCreateRule_2_1_8_RejectsAllMissingRequiredFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"description": "Invalid rule",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0353", errResp.Code, "Error code should be TRC-0106 for name required (first missing field)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "name is required", errResp.Message, "Error message should indicate name is required")
}

// TestCreateRule_2_1_9_RejectsEmptyName verifies validation of empty name string.
func TestCreateRule_2_1_9_RejectsEmptyName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "",
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0353", errResp.Code, "Error code should be TRC-0106 for empty name validation error")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "name is required", errResp.Message, "Error message should indicate name is required")
}

// TestCreateRule_2_1_10_RejectsNameExceedingMaxLength verifies validation of name field boundary (255 characters).
func TestCreateRule_2_1_10_RejectsNameExceedingMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		nameLength     int
		expectedStatus int
		shouldSucceed  bool
	}{
		{255, http.StatusCreated, true},
		{256, http.StatusBadRequest, false},
		{300, http.StatusBadRequest, false},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name_len_%d", tc.nameLength), func(t *testing.T) {
			name := strings.Repeat("a", tc.nameLength)
			reqBody := map[string]any{
				"name":       name,
				"expression": "true",
				"action":     "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expectedStatus, resp.StatusCode, "Response: %s", string(respBody))

			if tc.shouldSucceed {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0354", errResp.Code)
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "name")
			}
		})
	}
}

// TestCreateRule_2_1_11_RejectsEmptyExpression verifies validation of empty expression string.
func TestCreateRule_2_1_11_RejectsEmptyExpression(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "Empty expression rule",
		"expression": "",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0355", errResp.Code, "Error code should be TRC-0108 for empty expression validation error")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "expression is required", errResp.Message, "Error message should indicate expression is required")
}

// TestCreateRule_2_1_12_RejectsExpressionExceedingMaxLength verifies validation of expression field boundary (5000 characters).
func TestCreateRule_2_1_12_RejectsExpressionExceedingMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Build a valid CEL expression that's long enough
	// Using repeated "true && " pattern, then padding with spaces to exact length
	buildLongExpression := func(targetLen int) string {
		base := "true"
		segment := " && true"
		result := base
		for len(result)+len(segment) <= targetLen-len(" && true") {
			result += segment
		}
		// Pad with spaces to reach exact target length (spaces are valid in CEL)
		result += " && true"
		for len(result) < targetLen {
			result = result + " " // trailing spaces
		}
		return result[:targetLen]
	}

	tests := []struct {
		name           string
		exprLength     int
		expectedStatus int
		shouldSucceed  bool
	}{
		{"5000_chars", 5000, http.StatusCreated, true},
		{"5001_chars", 5001, http.StatusBadRequest, false},
		{"6000_chars", 6000, http.StatusBadRequest, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expr := buildLongExpression(tc.exprLength)
			reqBody := map[string]any{
				"name":       "Long expression rule " + tc.name,
				"expression": expr,
				"action":     "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expectedStatus, resp.StatusCode, "Response: %s", string(respBody))

			if tc.shouldSucceed {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0356", errResp.Code)
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "expression")
			}
		})
	}
}

// TestCreateRule_2_1_13_RejectsDescriptionExceedingMaxLength verifies validation of description field boundary (1000 characters).
func TestCreateRule_2_1_13_RejectsDescriptionExceedingMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		name           string
		descLength     int
		expectedStatus int
		shouldSucceed  bool
	}{
		{"1000_chars", 1000, http.StatusCreated, true},
		{"1001_chars", 1001, http.StatusBadRequest, false},
		{"1500_chars", 1500, http.StatusBadRequest, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc := strings.Repeat("a", tc.descLength)
			reqBody := map[string]any{
				"name":        "Description boundary rule " + tc.name,
				"description": desc,
				"expression":  "true",
				"action":      "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expectedStatus, resp.StatusCode, "Response: %s", string(respBody))

			if tc.shouldSucceed {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0359", errResp.Code, "Error code should be TRC-0112 for description too long")
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "description")
			}
		})
	}
}

// TestCreateRule_2_1_14_RejectsInvalidActionEnum verifies validation of action field with invalid enum value.
func TestCreateRule_2_1_14_RejectsInvalidActionEnum(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidActions := []string{"allow", "APPROVE", "invalid", ""}

	for _, action := range invalidActions {
		t.Run("action_"+action, func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Invalid action rule " + action,
				"expression": "true",
				"action":     action,
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0357", errResp.Code, "Error code should be TRC-0110 for invalid action enum")
			assert.Equal(t, "Validation Error", errResp.Title)
			assert.Contains(t, errResp.Message, "action")
		})
	}
}

// TestCreateRule_2_1_15_RejectsInvalidCELExpressionSyntax verifies validation of expression field with syntax errors.
func TestCreateRule_2_1_15_RejectsInvalidCELExpressionSyntax(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidExpressions := []string{
		"amount >",
		"amount ** 2",
		"(amount",
		"amount..",
	}

	for _, expr := range invalidExpressions {
		t.Run("expr_"+expr[:min(len(expr), 20)], func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Syntax error rule",
				"expression": expr,
				"action":     "DENY",
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0340", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid CEL expression syntax", errResp.Message)
		})
	}
}

// TestCreateRule_2_1_16_RejectsExpressionNotReturningBoolean verifies validation of expression type (must return boolean).
func TestCreateRule_2_1_16_RejectsExpressionNotReturningBoolean(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	nonBoolExpressions := []string{
		"amount",
		"amount + 100",
	}

	for _, expr := range nonBoolExpressions {
		t.Run("expr_"+expr[:min(len(expr), 20)], func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Non-boolean expression rule",
				"expression": expr,
				"action":     "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0341", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Expression must evaluate to boolean", errResp.Message)
		})
	}
}

// TestCreateRule_2_1_18_RejectsDuplicateRuleName verifies uniqueness constraint on rule name.
func TestCreateRule_2_1_18_RejectsDuplicateRuleName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use a unique name to avoid collisions with other tests, but reuse it
	// within this test to verify the duplicate-name constraint.
	ruleName := "Unique rule name " + testutil.RandomSuffix()

	// Create first rule
	reqBody := map[string]any{
		"name":       ruleName,
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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
	require.Equal(t, http.StatusCreated, resp.StatusCode, "First rule creation failed: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	firstRuleID := getStringField(t, result, "ruleId")

	t.Cleanup(func() {
		testutil.CleanupRule(t, firstRuleID)
	})

	// Try to create second rule with same name
	reqBody2 := map[string]any{
		"name":       ruleName,
		"expression": "false",
		"action":     "DENY",
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, resp2.StatusCode, "Response: %s", string(respBody2))

	errResp := testutil.ParseErrorResponse(t, respBody2)
	assert.Equal(t, "0441", errResp.Code)
	assert.Equal(t, "Conflict", errResp.Title)
	assert.Equal(t, "Rule name already exists in this context", errResp.Message)
}

// TestCreateRule_2_1_19_RejectsInvalidNameType verifies type validation for name field.
func TestCreateRule_2_1_19_RejectsInvalidNameType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidNames := []any{123, true, map[string]any{"name": "test"}, []string{"test"}}

	for i, name := range invalidNames {
		t.Run(fmt.Sprintf("name_type_%d", i), func(t *testing.T) {
			reqBody := map[string]any{
				"name":       name,
				"expression": "true",
				"action":     "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0047", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid request body", errResp.Message)
		})
	}

	// Test null separately
	t.Run("name_null", func(t *testing.T) {
		jsonBody := `{"name": null, "expression": "true", "action": "ALLOW"}`

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(jsonBody))
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
		assert.Equal(t, "0353", errResp.Code, "Error code should be TRC-0106 for null name validation error")
		assert.Equal(t, "Validation Error", errResp.Title)
		assert.Contains(t, errResp.Message, "name")
	})
}

// TestCreateRule_2_1_20_RejectsInvalidExpressionType verifies type validation for expression field.
func TestCreateRule_2_1_20_RejectsInvalidExpressionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidExprs := []any{123, true, map[string]any{"expr": "true"}, []string{"true"}}

	for i, expr := range invalidExprs {
		t.Run(fmt.Sprintf("expr_type_%d", i), func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Invalid expr type rule",
				"expression": expr,
				"action":     "ALLOW",
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0047", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid request body", errResp.Message)
		})
	}

	// Test null separately
	t.Run("expr_null", func(t *testing.T) {
		jsonBody := `{"name": "Null expr rule", "expression": null, "action": "ALLOW"}`

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(jsonBody))
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
		assert.Equal(t, "0355", errResp.Code, "Error code should be TRC-0108 for null expression validation error")
		assert.Equal(t, "Validation Error", errResp.Title)
		assert.Contains(t, errResp.Message, "expression")
	})
}

// TestCreateRule_2_1_21_RejectsInvalidActionType verifies type validation for action field.
func TestCreateRule_2_1_21_RejectsInvalidActionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidActions := []any{123, true, map[string]any{"action": "ALLOW"}, []string{"ALLOW"}}

	for i, action := range invalidActions {
		t.Run(fmt.Sprintf("action_type_%d", i), func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Invalid action type rule",
				"expression": "true",
				"action":     action,
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0047", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid request body", errResp.Message)
		})
	}

	// Test null separately
	t.Run("action_null", func(t *testing.T) {
		jsonBody := `{"name": "Null action rule", "expression": "true", "action": null}`

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", strings.NewReader(jsonBody))
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
		assert.Equal(t, "0357", errResp.Code, "Error code should be TRC-0110 for null action validation error")
		assert.Equal(t, "Validation Error", errResp.Title)
		assert.Contains(t, errResp.Message, "action")
	})
}

// TestCreateRule_2_1_22_RejectsInvalidScopesType verifies type validation for scopes field.
func TestCreateRule_2_1_22_RejectsInvalidScopesType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidScopes := []any{"CARD", 123, true, map[string]any{"type": "CARD"}}

	for i, scopes := range invalidScopes {
		t.Run(fmt.Sprintf("scopes_type_%d", i), func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "Invalid scopes type rule",
				"expression": "true",
				"action":     "ALLOW",
				"scopes":     scopes,
			}

			body, err := json.Marshal(reqBody)
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
			assert.Equal(t, "0047", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid request body", errResp.Message)
		})
	}
}

// TestCreateRule_2_1_23_RejectsScopeWithNoFieldsSet verifies validation that each scope must have at least one non-null field.
func TestCreateRule_2_1_23_RejectsScopeWithNoFieldsSet(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":       "Empty scope rule",
		"expression": "true",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{},
		},
	}

	body, err := json.Marshal(reqBody)
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
	assert.Equal(t, "0358", errResp.Code, "Error code should be TRC-0111 for empty scope")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "scope at index 0 must have at least one field set", errResp.Message, "Error message should indicate scope must have at least one field")
}

// TestCreateRule_2_1_24_RejectsScopeWithInvalidUUIDFormat verifies UUID format validation in scope fields.
func TestCreateRule_2_1_24_RejectsScopeWithInvalidUUIDFormat(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name   string
		field  string
		value  string
		expect int
	}{
		{"valid_accountId", "accountId", "550e8400-e29b-41d4-a716-446655440000", http.StatusCreated},
		{"invalid_accountId_not_uuid", "accountId", "not-a-uuid", http.StatusBadRequest},
		{"invalid_accountId_short", "accountId", "123", http.StatusBadRequest},
		{"invalid_accountId_bad_format", "accountId", "invalid-uuid-format", http.StatusBadRequest},
		{"invalid_segmentId", "segmentId", "invalid-uuid", http.StatusBadRequest},
		{"invalid_portfolioId", "portfolioId", "12345", http.StatusBadRequest},
		{"invalid_merchantId", "merchantId", "not-uuid", http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scope := map[string]any{
				tc.field: tc.value,
			}

			reqBody := map[string]any{
				"name":       "UUID validation rule " + tc.name,
				"expression": "true",
				"action":     "ALLOW",
				"scopes":     []map[string]any{scope},
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusCreated {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				// TRC-0003 is returned for JSON type mismatch errors (bad request)
				assert.Equal(t, "0047", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.NotEmpty(t, errResp.Message)
			}
		})
	}
}

// TestCreateRule_2_1_25_RejectsScopeWithInvalidTransactionTypeEnum verifies transactionType enum validation in scopes.
func TestCreateRule_2_1_25_RejectsScopeWithInvalidTransactionTypeEnum(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name            string
		transactionType string
		expect          int
	}{
		{"valid_CARD", "CARD", http.StatusCreated},
		{"valid_WIRE", "WIRE", http.StatusCreated},
		{"valid_PIX", "PIX", http.StatusCreated},
		{"valid_CRYPTO", "CRYPTO", http.StatusCreated},
		{"invalid_lowercase", "card", http.StatusBadRequest},
		{"invalid_CASH", "CASH", http.StatusBadRequest},
		{"invalid_random", "invalid", http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]any{
				"name":       "TransactionType validation rule " + tc.name,
				"expression": "true",
				"action":     "ALLOW",
				"scopes": []map[string]any{
					{"transactionType": tc.transactionType},
				},
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusCreated {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0358", errResp.Code, "Error code should be TRC-0111 for invalid transactionType enum")
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "transactionType")
			}
		})
	}
}

// TestCreateRule_2_1_26_RejectsSubTypeExceedingMaxLength verifies subType field boundary (50 characters) in scopes.
func TestCreateRule_2_1_26_RejectsSubTypeExceedingMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name       string
		subTypeLen int
		expect     int
	}{
		{"50_chars", 50, http.StatusCreated},
		{"51_chars", 51, http.StatusBadRequest},
		{"100_chars", 100, http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			subType := strings.Repeat("a", tc.subTypeLen)
			reqBody := map[string]any{
				"name":       "SubType boundary rule " + tc.name,
				"expression": "true",
				"action":     "ALLOW",
				"scopes": []map[string]any{
					{
						"transactionType": "CARD",
						"subType":         subType,
					},
				},
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusCreated {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0358", errResp.Code, "Error code should be TRC-0111 for scope field validation")
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "subType")
			}
		})
	}
}

// TestCreateRule_2_1_27_RejectsScopesExceedingMaxCount verifies scopes array boundary (100 items).
func TestCreateRule_2_1_27_RejectsScopesExceedingMaxCount(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name       string
		scopeCount int
		expect     int
	}{
		{"100_scopes", 100, http.StatusCreated},
		{"101_scopes", 101, http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scopes := make([]map[string]any, tc.scopeCount)
			for i := 0; i < tc.scopeCount; i++ {
				scopes[i] = map[string]any{"transactionType": "CARD"}
			}

			reqBody := map[string]any{
				"name":       "Max scopes rule " + tc.name,
				"expression": "true",
				"action":     "ALLOW",
				"scopes":     scopes,
			}

			body, err := json.Marshal(reqBody)
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

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusCreated {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				ruleID := getStringField(t, result, "ruleId")
				t.Cleanup(func() {
					testutil.CleanupRule(t, ruleID)
				})
			} else {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0360", errResp.Code, "Error code should be TRC-0113 for scopes exceeding max count")
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "scope")
			}
		})
	}
}

// TestCreateRule_2_1_29_RejectsWithoutAuthentication verifies authentication requirement for rule creation.
func TestCreateRule_2_1_29_RejectsWithoutAuthentication(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]any{
		"name":       "Unauthenticated rule",
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for missing API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// TestCreateRule_2_1_30_RejectsWithInvalidAPIKey verifies validation of invalid API key.
func TestCreateRule_2_1_30_RejectsWithInvalidAPIKey(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]any{
		"name":       "Invalid auth rule",
		"expression": "true",
		"action":     "ALLOW",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "invalid-api-key-12345")
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for invalid API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// TestCreateRule_2_1_31_WithWhitespaceOnlyDescription verifies handling of whitespace-only optional fields.
func TestCreateRule_2_1_31_WithWhitespaceOnlyDescription(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name":        "Whitespace description rule",
		"description": "   ",
		"expression":  "true",
		"action":      "ALLOW",
	}

	body, err := json.Marshal(reqBody)
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

	// Either accepted (201) or rejected (400) is valid per roteiro
	if resp.StatusCode == http.StatusCreated {
		var result map[string]any
		err = json.Unmarshal(respBody, &result)
		require.NoError(t, err)
		ruleID := getStringField(t, result, "ruleId")
		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleID)
		})
	} else {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))
	}
}

// =============================================================================
// 2.2 GET /v1/rules/{ruleId} - Get Rule by ID
// =============================================================================

// TestGetRule_2_2_1_RetrievesRuleByValidID verifies rule retrieval with existing rule ID.
func TestGetRule_2_2_1_RetrievesRuleByValidID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create a rule first
	createReqBody := map[string]any{
		"name":        "Retrievable rule",
		"description": "Test description",
		"expression":  "amount > 500000",
		"action":      "REVIEW",
		"scopes": []map[string]any{
			{"transactionType": "CARD"},
		},
	}

	createBody, err := json.Marshal(createReqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(createBody))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Create failed: %s", string(createRespBody))

	var createResult map[string]any
	err = json.Unmarshal(createRespBody, &createResult)
	require.NoError(t, err)
	ruleID := getStringField(t, createResult, "ruleId")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Now retrieve the rule
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, getResp.StatusCode, "Response: %s", string(getRespBody))

	var result map[string]any
	err = json.Unmarshal(getRespBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	// API normalizes name to lowercase
	assert.Equal(t, "retrievable rule", result["name"])
	assert.Equal(t, "Test description", result["description"])
	assert.Equal(t, "amount > 500000", result["expression"])
	assert.Equal(t, "REVIEW", result["action"])
	assert.Equal(t, "DRAFT", result["status"])

	scopes, ok := result["scopes"].([]any)
	require.True(t, ok)
	require.Len(t, scopes, 1)
	scope0, ok := scopes[0].(map[string]any)
	require.True(t, ok, "scope should be a map")
	assert.Equal(t, "CARD", scope0["transactionType"])

	assert.NotEmpty(t, result["createdAt"])
	assert.NotEmpty(t, result["updatedAt"])
	assert.Nil(t, result["deletedAt"])
}

// TestGetRule_2_2_2_RetrievesRuleWithMinimalFields verifies retrieval of rule with only required fields.
func TestGetRule_2_2_2_RetrievesRuleWithMinimalFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create minimal rule
	ruleID := testutil.CreateTestRuleWithExpression(t, "Minimal retrieval rule", "false", "DENY")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Retrieve the rule
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, getResp.StatusCode, "Response: %s", string(getRespBody))

	var result map[string]any
	err = json.Unmarshal(getRespBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	// API normalizes name to lowercase
	assert.Equal(t, "minimal retrieval rule", result["name"])
	assert.Nil(t, result["description"])
	assert.Equal(t, "DENY", result["action"])
	assert.Equal(t, "DRAFT", result["status"])

	scopes, ok := result["scopes"].([]any)
	require.True(t, ok)
	assert.Empty(t, scopes)
}

// TestGetRule_2_2_3_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestGetRule_2_2_3_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999", nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode, "Response: %s", string(getRespBody))

	errResp := testutil.ParseErrorResponse(t, getRespBody)
	assert.Equal(t, "0347", errResp.Code)
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestGetRule_2_2_4_RejectsInvalidUUIDFormatInPath verifies path parameter validation.
func TestGetRule_2_2_4_RejectsInvalidUUIDFormatInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidIDs := []string{
		"not-a-uuid",
		"123",
		"550e8400-e29b-41d4-a716-44665544000z", // invalid character 'z'
	}

	for _, id := range invalidIDs {
		t.Run("id_"+id, func(t *testing.T) {
			getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+id, nil)
			require.NoError(t, err)
			getReq.Header.Set("X-API-Key", apiKey)

			getResp, err := testutil.HTTPClient.Do(getReq)
			require.NoError(t, err)
			defer getResp.Body.Close()

			getRespBody, err := io.ReadAll(getResp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, getResp.StatusCode, "Response: %s", string(getRespBody))

			errResp := testutil.ParseErrorResponse(t, getRespBody)
			assert.Equal(t, "0065", errResp.Code)
			assert.Equal(t, "Invalid Path Parameter", errResp.Title)
			assert.Equal(t, "Invalid rule ID format", errResp.Message)
		})
	}
}

// TestGetRule_2_2_5_Returns404ForDeletedRule verifies deleted rules are not retrievable.
func TestGetRule_2_2_5_Returns404ForDeletedRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule
	ruleID := testutil.CreateTestRuleWithExpression(t, "Deletable rule", "true", "ALLOW")

	// Deactivate and delete
	testutil.DeactivateRule(t, ruleID)
	testutil.DeleteRuleViaAPI(t, ruleID)

	// Try to retrieve deleted rule
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode, "Response: %s", string(getRespBody))

	errResp := testutil.ParseErrorResponse(t, getRespBody)
	assert.Equal(t, "0347", errResp.Code)
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestGetRule_2_2_6_WithoutAuthenticationReturns401 verifies authentication requirement for rule retrieval.
func TestGetRule_2_2_6_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule
	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth test rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Try to retrieve without auth
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	// No X-API-Key header

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, getResp.StatusCode)

	respBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code)
	assert.Equal(t, "Unauthorized", errResp.Title)
	assert.Equal(t, "API Key missing or invalid", errResp.Message)

	// Verify it works with auth
	getReq2, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq2.Header.Set("X-API-Key", apiKey)

	getResp2, err := testutil.HTTPClient.Do(getReq2)
	require.NoError(t, err)
	defer getResp2.Body.Close()

	assert.Equal(t, http.StatusOK, getResp2.StatusCode)
}

// TestGetRule_2_2_7_IdempotentRetrieval verifies GET is idempotent.
func TestGetRule_2_2_7_IdempotentRetrieval(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule
	ruleID := testutil.CreateTestRuleWithExpression(t, "Idempotent retrieval rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// First retrieval
	getReq1, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq1.Header.Set("X-API-Key", apiKey)

	getResp1, err := testutil.HTTPClient.Do(getReq1)
	require.NoError(t, err)
	defer getResp1.Body.Close()

	getRespBody1, err := io.ReadAll(getResp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp1.StatusCode)

	var result1 map[string]any
	err = json.Unmarshal(getRespBody1, &result1)
	require.NoError(t, err)

	// Second retrieval
	getReq2, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq2.Header.Set("X-API-Key", apiKey)

	getResp2, err := testutil.HTTPClient.Do(getReq2)
	require.NoError(t, err)
	defer getResp2.Body.Close()

	getRespBody2, err := io.ReadAll(getResp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var result2 map[string]any
	err = json.Unmarshal(getRespBody2, &result2)
	require.NoError(t, err)

	// Verify responses are identical
	assert.Equal(t, result1["ruleId"], result2["ruleId"])
	assert.Equal(t, result1["name"], result2["name"])
	assert.Equal(t, result1["expression"], result2["expression"])
	assert.Equal(t, result1["action"], result2["action"])
	assert.Equal(t, result1["status"], result2["status"])
	assert.Equal(t, result1["updatedAt"], result2["updatedAt"])
}

// =============================================================================
// 2.3 GET /v1/rules - List Rules
// =============================================================================

// TestListRules_2_3_1_WithoutFilters verifies default listing behavior.
func TestListRules_2_3_1_WithoutFilters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create 3 test rules
	ruleIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		ruleIDs[i] = testutil.CreateTestRuleWithExpression(t, fmt.Sprintf("List test rule %d", i+1), "true", "ALLOW")
	}

	t.Cleanup(func() {
		for _, id := range ruleIDs {
			testutil.CleanupRule(t, id)
		}
	})

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	_, ok := result["rules"].([]any)
	require.True(t, ok, "rules should be an array")

	_, hasHasMore := result["hasMore"].(bool)
	assert.True(t, hasHasMore, "hasMore should be a boolean")
}

// TestListRules_2_3_2_WithDefaultPagination verifies default limit (100) is applied.
func TestListRules_2_3_2_WithDefaultPagination(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)
	assert.LessOrEqual(t, len(rules), 100)
}

// TestListRules_2_3_3_WithCustomLimit verifies custom limit parameter and pagination.
func TestListRules_2_3_3_WithCustomLimit(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create exactly 5 rules
	ruleIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		ruleIDs[i] = testutil.CreateTestRuleWithExpression(t, fmt.Sprintf("Pagination test rule %c", 'A'+i), "true", "REVIEW")
	}

	t.Cleanup(func() {
		for _, id := range ruleIDs {
			testutil.CleanupRule(t, id)
		}
	})

	// First page with limit=3
	req1, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?limit=3&action=REVIEW", nil)
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp1.StatusCode, "Response: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	rules1, ok := result1["rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules1, 3)
	assert.True(t, result1["hasMore"].(bool))
	assert.NotEmpty(t, result1["nextCursor"])

	cursor := getStringField(t, result1, "nextCursor")

	// Second page
	req2, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?limit=3&action=REVIEW&cursor="+cursor, nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "Response: %s", string(respBody2))

	var result2 map[string]any
	err = json.Unmarshal(respBody2, &result2)
	require.NoError(t, err)

	rules2, ok := result2["rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules2, 2)
	assert.False(t, result2["hasMore"].(bool))

	// Verify no duplicates
	allIDs := make(map[string]bool)
	for _, r := range rules1 {
		rule, ok := r.(map[string]any)
		require.True(t, ok, "rule should be a map")
		ruleID, ok := rule["ruleId"].(string)
		require.True(t, ok, "ruleId should be a string")
		allIDs[ruleID] = true
	}

	for _, r := range rules2 {
		rule, ok := r.(map[string]any)
		require.True(t, ok, "rule should be a map")
		id, ok := rule["ruleId"].(string)
		require.True(t, ok, "ruleId should be a string")
		assert.False(t, allIDs[id], "Duplicate rule ID found: %s", id)
		allIDs[id] = true
	}

	assert.Len(t, allIDs, 5)
}

// TestListRules_2_3_4_FiltersByStatus verifies status filter functionality.
func TestListRules_2_3_4_FiltersByStatus(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rules with different statuses
	draftRuleID := testutil.CreateTestRuleWithExpression(t, "Draft status rule", "true", "ALLOW")

	activeRuleID := testutil.CreateTestRuleWithExpression(t, "Active status rule", "true", "ALLOW")
	testutil.ActivateRule(t, activeRuleID)

	inactiveRuleID := testutil.CreateTestRuleWithExpression(t, "Inactive status rule", "true", "ALLOW")
	testutil.ActivateRule(t, inactiveRuleID)
	testutil.DeactivateRule(t, inactiveRuleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, draftRuleID)
		testutil.CleanupRule(t, activeRuleID)
		testutil.CleanupRule(t, inactiveRuleID)
	})

	statuses := []string{"DRAFT", "ACTIVE", "INACTIVE"}
	for _, status := range statuses {
		t.Run("status_"+status, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?status="+status, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

			var result map[string]any
			err = json.Unmarshal(respBody, &result)
			require.NoError(t, err)

			rules, ok := result["rules"].([]any)
			require.True(t, ok)

			for _, r := range rules {
				rule, ok := r.(map[string]any)
				require.True(t, ok, "rule should be a map")
				assert.Equal(t, status, rule["status"], "Rule should have status %s", status)
			}
		})
	}
}

// TestListRules_2_3_5_RejectsDELETEDStatusFilter verifies DELETED status is not allowed in filter.
func TestListRules_2_3_5_RejectsDELETEDStatusFilter(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?status=DELETED", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0082", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Contains(t, errResp.Message, "DELETED")
}

// TestListRules_2_3_6_FiltersByAction verifies action filter functionality.
func TestListRules_2_3_6_FiltersByAction(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rules with different actions
	allowRuleID := testutil.CreateTestRuleWithExpression(t, "Allow action rule", "true", "ALLOW")
	denyRuleID := testutil.CreateTestRuleWithExpression(t, "Deny action rule", "true", "DENY")
	reviewRuleID := testutil.CreateTestRuleWithExpression(t, "Review action rule", "true", "REVIEW")

	t.Cleanup(func() {
		testutil.CleanupRule(t, allowRuleID)
		testutil.CleanupRule(t, denyRuleID)
		testutil.CleanupRule(t, reviewRuleID)
	})

	actions := []string{"ALLOW", "DENY", "REVIEW"}
	for _, action := range actions {
		t.Run("action_"+action, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?action="+action, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

			var result map[string]any
			err = json.Unmarshal(respBody, &result)
			require.NoError(t, err)

			rules, ok := result["rules"].([]any)
			require.True(t, ok)

			for _, r := range rules {
				rule, ok := r.(map[string]any)
				require.True(t, ok, "rule should be a map")
				assert.Equal(t, action, rule["action"], "Rule should have action %s", action)
			}
		})
	}
}

// TestListRules_2_3_7_CombinesMultipleFilters verifies multiple query parameters work together.
func TestListRules_2_3_7_CombinesMultipleFilters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Combined filter rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?status=ACTIVE&action=ALLOW", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	for _, r := range rules {
		rule, ok := r.(map[string]any)
		require.True(t, ok, "rule should be a map")
		assert.Equal(t, "ACTIVE", rule["status"])
		assert.Equal(t, "ALLOW", rule["action"])
	}
}

// TestListRules_2_3_8_SortsByCreatedAtAscending verifies sort_by=created_at and sort_order=ASC.
func TestListRules_2_3_8_SortsByCreatedAtAscending(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_by=created_at&sort_order=ASC", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	if len(rules) > 1 {
		for i := 1; i < len(rules); i++ {
			prevRule, ok := rules[i-1].(map[string]any)
			require.True(t, ok, "rule should be a map")
			prev, ok := prevRule["createdAt"].(string)
			require.True(t, ok, "createdAt should be a string")
			currRule, ok := rules[i].(map[string]any)
			require.True(t, ok, "rule should be a map")
			curr, ok := currRule["createdAt"].(string)
			require.True(t, ok, "createdAt should be a string")
			assert.LessOrEqual(t, prev, curr, "createdAt should be in ascending order")
		}
	}
}

// TestListRules_2_3_9_SortsByCreatedAtDescending verifies sort_by=created_at and sort_order=DESC.
func TestListRules_2_3_9_SortsByCreatedAtDescending(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_by=created_at&sort_order=DESC", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	if len(rules) > 1 {
		for i := 1; i < len(rules); i++ {
			prevRule, ok := rules[i-1].(map[string]any)
			require.True(t, ok, "rule should be a map")
			prev, ok := prevRule["createdAt"].(string)
			require.True(t, ok, "createdAt should be a string")
			currRule, ok := rules[i].(map[string]any)
			require.True(t, ok, "rule should be a map")
			curr, ok := currRule["createdAt"].(string)
			require.True(t, ok, "createdAt should be a string")
			assert.GreaterOrEqual(t, prev, curr, "createdAt should be in descending order")
		}
	}
}

// TestListRules_2_3_10_SortsByName verifies sort_by=name with ascending and descending order.
func TestListRules_2_3_10_SortsByName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleIDs := []string{
		testutil.CreateTestRuleWithExpression(t, "Alpha sort rule", "true", "ALLOW"),
		testutil.CreateTestRuleWithExpression(t, "Beta sort rule", "true", "ALLOW"),
		testutil.CreateTestRuleWithExpression(t, "Gamma sort rule", "true", "ALLOW"),
	}

	t.Cleanup(func() {
		for _, id := range ruleIDs {
			testutil.CleanupRule(t, id)
		}
	})

	tests := []struct {
		sortOrder string
		ascending bool
	}{
		{"ASC", true},
		{"DESC", false},
	}

	for _, tc := range tests {
		t.Run("sortOrder_"+tc.sortOrder, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_by=name&sort_order="+tc.sortOrder, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

			var result map[string]any
			err = json.Unmarshal(respBody, &result)
			require.NoError(t, err)

			rules, ok := result["rules"].([]any)
			require.True(t, ok)

			if len(rules) > 1 {
				for i := 1; i < len(rules); i++ {
					prevRule, ok := rules[i-1].(map[string]any)
					require.True(t, ok, "rule should be a map")
					prev, ok := prevRule["name"].(string)
					require.True(t, ok, "name should be a string")
					currRule, ok := rules[i].(map[string]any)
					require.True(t, ok, "rule should be a map")
					curr, ok := currRule["name"].(string)
					require.True(t, ok, "name should be a string")
					if tc.ascending {
						assert.LessOrEqual(t, prev, curr)
					} else {
						assert.GreaterOrEqual(t, prev, curr)
					}
				}
			}
		})
	}
}

// TestListRules_2_3_11_SortsByStatus verifies sort_by=status.
func TestListRules_2_3_11_SortsByStatus(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_by=status&sort_order=ASC", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	if len(rules) > 1 {
		for i := 1; i < len(rules); i++ {
			prevRule, ok := rules[i-1].(map[string]any)
			require.True(t, ok, "rule should be a map")
			prev, ok := prevRule["status"].(string)
			require.True(t, ok, "status should be a string")
			currRule, ok := rules[i].(map[string]any)
			require.True(t, ok, "rule should be a map")
			curr, ok := currRule["status"].(string)
			require.True(t, ok, "status should be a string")
			assert.LessOrEqual(t, prev, curr)
		}
	}
}

// TestListRules_2_3_12_RejectsInvalidSortByValue verifies validation of sort_by parameter.
func TestListRules_2_3_12_RejectsInvalidSortByValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidSortBy := []string{"invalidColumn", "id", "expression"}

	for _, sortBy := range invalidSortBy {
		t.Run("sortBy_"+sortBy, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_by="+sortBy, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, "0332", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Contains(t, errResp.Message, "sort_by")
		})
	}
}

// TestListRules_2_3_13_RejectsInvalidSortOrderValue verifies validation of sort_order parameter.
func TestListRules_2_3_13_RejectsInvalidSortOrderValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		sortOrder string
		expect    int
	}{
		{"ASC", http.StatusOK},
		{"DESC", http.StatusOK},
		{"asc", http.StatusOK},
		{"desc", http.StatusOK},
		{"ASCENDING", http.StatusBadRequest},
		{"invalid", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run("sortOrder_"+tc.sortOrder, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?sort_order="+tc.sortOrder, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0081", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.NotEmpty(t, errResp.Message)
			}
		})
	}
}

// TestListRules_2_3_14_RejectsLimitBelowMinimum verifies limit parameter minimum boundary (1).
func TestListRules_2_3_14_RejectsLimitBelowMinimum(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		limit  string
		expect int
	}{
		{"1", http.StatusOK},
		{"0", http.StatusBadRequest},
		{"-1", http.StatusBadRequest},
		{"-100", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run("limit_"+tc.limit, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?limit="+tc.limit, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0331", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.NotEmpty(t, errResp.Message)
			}
		})
	}
}

// TestListRules_2_3_15_RejectsLimitAboveMaximum verifies limit parameter maximum boundary (100).
func TestListRules_2_3_15_RejectsLimitAboveMaximum(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		limit  string
		expect int
	}{
		{"100", http.StatusOK},
		{"101", http.StatusBadRequest},
		{"1000", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run("limit_"+tc.limit, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?limit="+tc.limit, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0080", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.NotEmpty(t, errResp.Message)
			}
		})
	}
}

// TestListRules_2_3_16_RejectsInvalidCursor verifies cursor validation.
func TestListRules_2_3_16_RejectsInvalidCursor(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	invalidCursors := []string{"invalid-cursor", "not-base64", "12345"}

	for _, cursor := range invalidCursors {
		t.Run("cursor_"+cursor, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?cursor="+cursor, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, "0333", errResp.Code)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "Invalid pagination cursor", errResp.Message)
		})
	}
}

// TestListRules_2_3_17_RejectsInvalidActionFilterValue verifies action filter validation.
func TestListRules_2_3_17_RejectsInvalidActionFilterValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		action string
		expect int
	}{
		{"ALLOW", http.StatusOK},
		{"allow", http.StatusBadRequest},
		{"APPROVE", http.StatusBadRequest},
		{"invalid", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run("action_"+tc.action, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?action="+tc.action, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0082", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.Contains(t, errResp.Message, "action")
			}
		})
	}
}

// TestListRules_2_3_18_RejectsInvalidStatusFilterValue verifies status filter validation.
func TestListRules_2_3_18_RejectsInvalidStatusFilterValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	tests := []struct {
		status string
		expect int
	}{
		{"DRAFT", http.StatusOK},
		{"draft", http.StatusBadRequest},
		{"PENDING", http.StatusBadRequest},
		{"invalid", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run("status_"+tc.status, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?status="+tc.status, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0082", errResp.Code)
				assert.Equal(t, "Bad Request", errResp.Title)
				assert.Contains(t, errResp.Message, "Status")
			}
		})
	}
}

// TestListRules_2_3_19_ReturnsEmptyListWhenNoRulesMatchFilters verifies behavior when filters match no rules.
func TestListRules_2_3_19_ReturnsEmptyListWhenNoRulesMatchFilters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?status=ACTIVE&action=REVIEW", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// rules should always be an array (empty when no matches)
	rules, ok := result["rules"].([]any)
	assert.True(t, ok, "rules should be an array, got: %v", result["rules"])
	assert.Empty(t, rules, "rules array should be empty")
	assert.False(t, result["hasMore"].(bool))
}

// TestListRules_2_3_20_WithoutAuthenticationReturns401 verifies authentication requirement for listing.
func TestListRules_2_3_20_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules", nil)
	require.NoError(t, err)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code)
	assert.Equal(t, "Unauthorized", errResp.Title)
	assert.Equal(t, "API Key missing or invalid", errResp.Message)
}

// =============================================================================
// 2.4 PATCH /v1/rules/{ruleId} - Update Rule
// =============================================================================

// TestUpdateRule_2_4_1_UpdatesRuleName verifies partial update of name field.
func TestUpdateRule_2_4_1_UpdatesRuleName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Original name", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"name": "Updated name",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "updated name", result["name"]) // normalized to lowercase
	assert.Equal(t, "true", result["expression"])
	assert.Equal(t, "ALLOW", result["action"])
}

// TestUpdateRule_2_4_2_UpdatesRuleDescription verifies partial update of description field.
func TestUpdateRule_2_4_2_UpdatesRuleDescription(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Description update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"description": "New description",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "New description", result["description"])
}

// TestUpdateRule_2_4_3_UpdatesRuleExpressionDraftOnly verifies expression update is allowed for DRAFT rules.
func TestUpdateRule_2_4_3_UpdatesRuleExpressionDraftOnly(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Draft update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"expression": "amount > 1000000",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "amount > 1000000", result["expression"])
	assert.Equal(t, "DRAFT", result["status"])
}

// TestUpdateRule_2_4_4_UpdatesRuleAction verifies action field can be updated.
func TestUpdateRule_2_4_4_UpdatesRuleAction(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Action update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"action": "DENY",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "DENY", result["action"])
}

// TestUpdateRule_2_4_5_UpdatesRuleScopes verifies scopes can be updated.
func TestUpdateRule_2_4_5_UpdatesRuleScopes(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule with initial scope
	createReqBody := map[string]any{
		"name":       "Scopes update rule",
		"expression": "true",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{"transactionType": "CARD"},
		},
	}

	createBody, err := json.Marshal(createReqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(createBody))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createResult map[string]any
	err = json.Unmarshal(createRespBody, &createResult)
	require.NoError(t, err)
	ruleID := getStringField(t, createResult, "ruleId")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Update scopes
	updateReqBody := map[string]any{
		"scopes": []map[string]any{
			{"transactionType": "PIX"},
			{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
		},
	}

	updateBody, err := json.Marshal(updateReqBody)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(updateBody))
	require.NoError(t, err)
	updateReq.Header.Set("X-API-Key", apiKey)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := testutil.HTTPClient.Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	updateRespBody, err := io.ReadAll(updateResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode, "Response: %s", string(updateRespBody))

	var updateResult map[string]any
	err = json.Unmarshal(updateRespBody, &updateResult)
	require.NoError(t, err)

	scopes, ok := updateResult["scopes"].([]any)
	require.True(t, ok)
	assert.Len(t, scopes, 2)

	// Collect scope values without relying on order
	var foundPIX, foundAccountID bool
	for _, s := range scopes {
		scope, ok := s.(map[string]any)
		require.True(t, ok, "scope element should be a map")
		if txType, ok := scope["transactionType"].(string); ok && txType == "PIX" {
			foundPIX = true
		}
		if accID, ok := scope["accountId"].(string); ok && accID == "550e8400-e29b-41d4-a716-446655440000" {
			foundAccountID = true
		}
	}
	assert.True(t, foundPIX, "Scopes should contain transactionType=PIX")
	assert.True(t, foundAccountID, "Scopes should contain accountId=550e8400-e29b-41d4-a716-446655440000")
}

// TestUpdateRule_2_4_6_UpdatesMultipleFieldsSimultaneously verifies multiple fields can be updated in single request.
func TestUpdateRule_2_4_6_UpdatesMultipleFieldsSimultaneously(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Multi-field update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"name":        "Multi-field update",
		"description": "Updated description",
		"expression":  "amount < 100000",
		"action":      "REVIEW",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "multi-field update", result["name"]) // normalized to lowercase
	assert.Equal(t, "Updated description", result["description"])
	assert.Equal(t, "amount < 100000", result["expression"])
	assert.Equal(t, "REVIEW", result["action"])
}

// TestUpdateRule_2_4_7_RejectsExpressionUpdateOnActiveRule verifies expression cannot be modified for ACTIVE rules.
func TestUpdateRule_2_4_7_RejectsExpressionUpdateOnActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Active expression rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"expression": "false",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0351", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Expression cannot be modified for non-DRAFT rules", errResp.Message)

	// Verify rule was not mutated
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var rule map[string]any
	err = json.Unmarshal(getRespBody, &rule)
	require.NoError(t, err)

	assert.Equal(t, "true", rule["expression"], "Expression should remain unchanged after rejected update")
	assert.Equal(t, "ACTIVE", rule["status"], "Status should remain ACTIVE")
}

// TestUpdateRule_2_4_8_RejectsExpressionUpdateOnInactiveRule verifies expression cannot be modified for INACTIVE rules.
func TestUpdateRule_2_4_8_RejectsExpressionUpdateOnInactiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Inactive expression rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"expression": "amount > 0",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0351", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Expression cannot be modified for non-DRAFT rules", errResp.Message)

	// Verify rule was not mutated
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var rule map[string]any
	err = json.Unmarshal(getRespBody, &rule)
	require.NoError(t, err)

	assert.Equal(t, "true", rule["expression"], "Expression should remain unchanged after rejected update")
	assert.Equal(t, "INACTIVE", rule["status"], "Status should remain INACTIVE")
}

// TestUpdateRule_2_4_9_AllowsNonExpressionUpdatesOnActiveRule verifies non-expression fields can be updated on ACTIVE rules.
func TestUpdateRule_2_4_9_AllowsNonExpressionUpdatesOnActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Active non-expr rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"name":        "Updated active rule name",
		"description": "Updated while active",
		"action":      "DENY",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "updated active rule name", result["name"]) // normalized to lowercase
	assert.Equal(t, "Updated while active", result["description"])
	assert.Equal(t, "DENY", result["action"])
	assert.Equal(t, "true", result["expression"])
	assert.Equal(t, "ACTIVE", result["status"])
}

// TestUpdateRule_2_4_10_RejectsEmptyUpdate verifies at least one field is required for update.
func TestUpdateRule_2_4_10_RejectsEmptyUpdate(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Empty update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0009", errResp.Code)
	assert.Equal(t, "Validation Error", errResp.Title)
	assert.Equal(t, "At least one field must be provided for update", errResp.Message)

	// Verify rule was not mutated
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var rule map[string]any
	err = json.Unmarshal(getRespBody, &rule)
	require.NoError(t, err)

	assert.Equal(t, "empty update rule", rule["name"], "Name should remain unchanged after rejected update")
	assert.Equal(t, "true", rule["expression"], "Expression should remain unchanged after rejected update")
	assert.Equal(t, "ALLOW", rule["action"], "Action should remain unchanged after rejected update")
	assert.Equal(t, "DRAFT", rule["status"], "Status should remain DRAFT")
}

// TestUpdateRule_2_4_11_RejectsDuplicateName verifies name uniqueness constraint on update.
func TestUpdateRule_2_4_11_RejectsDuplicateName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use unique names to avoid collisions with other tests
	suffix := testutil.RandomSuffix()
	nameA := "Unique name A " + suffix
	nameB := "Unique name B " + suffix

	ruleID1 := testutil.CreateTestRuleWithExpression(t, nameA, "true", "ALLOW")
	ruleID2 := testutil.CreateTestRuleWithExpression(t, nameB, "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID1)
		testutil.CleanupRule(t, ruleID2)
	})

	reqBody := map[string]any{
		"name": nameA,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID2, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0441", errResp.Code)
	assert.Equal(t, "Conflict", errResp.Title)
	assert.Equal(t, "Rule name already exists in this context", errResp.Message)
}

// TestUpdateRule_2_4_12_AllowsSameName verifies updating to the same name is allowed (idempotent).
func TestUpdateRule_2_4_12_AllowsSameName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Idempotent name", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"name": "Idempotent name",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "idempotent name", result["name"]) // normalized to lowercase
}

// TestUpdateRule_2_4_13_RejectsInvalidUUIDInPath verifies path parameter validation.
func TestUpdateRule_2_4_13_RejectsInvalidUUIDInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name": "Updated name",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/invalid-uuid", bytes.NewReader(body))
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
	assert.Equal(t, "0065", errResp.Code) // Invalid path parameter (UUID format)
	assert.Equal(t, "Invalid Path Parameter", errResp.Title)
	assert.Equal(t, "Invalid rule ID format", errResp.Message)
}

// TestUpdateRule_2_4_14_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestUpdateRule_2_4_14_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	reqBody := map[string]any{
		"name": "Updated name",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestUpdateRule_2_4_15_RejectsNameExceedingMaxLength verifies validation of name field boundary on update.
func TestUpdateRule_2_4_15_RejectsNameExceedingMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Name length rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	tests := []struct {
		nameLength int
		expect     int
	}{
		{255, http.StatusOK},
		{256, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name_len_%d", tc.nameLength), func(t *testing.T) {
			name := strings.Repeat("a", tc.nameLength)
			reqBody := map[string]any{
				"name": name,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expect, resp.StatusCode, "Response: %s", string(respBody))

			if tc.expect == http.StatusBadRequest {
				errResp := testutil.ParseErrorResponse(t, respBody)
				assert.Equal(t, "0354", errResp.Code)
				assert.Equal(t, "Validation Error", errResp.Title)
				assert.Contains(t, errResp.Message, "name")
			} else if tc.expect == http.StatusOK {
				var result map[string]any
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)
				name := getStringField(t, result, "name")
				assert.Len(t, name, tc.nameLength,
					"Updated rule name should have exactly %d characters", tc.nameLength)
			}
		})
	}
}

// TestUpdateRule_2_4_16_RejectsInvalidExpressionSyntaxOnUpdate verifies expression validation on update.
func TestUpdateRule_2_4_16_RejectsInvalidExpressionSyntaxOnUpdate(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Expression syntax rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"expression": "amount >>",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0340", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Invalid CEL expression syntax", errResp.Message)
}

// TestUpdateRule_2_4_17_RejectsExpressionNotReturningBooleanOnUpdate verifies expression type validation on update.
func TestUpdateRule_2_4_17_RejectsExpressionNotReturningBooleanOnUpdate(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Expression type rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"expression": "amount",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0341", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "Expression must evaluate to boolean", errResp.Message)
}

// TestUpdateRule_2_4_18_RejectsInvalidActionEnumOnUpdate verifies action validation on update.
func TestUpdateRule_2_4_18_RejectsInvalidActionEnumOnUpdate(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Action enum rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	invalidActions := []string{"APPROVE", "invalid"}

	for _, action := range invalidActions {
		t.Run("action_"+action, func(t *testing.T) {
			reqBody := map[string]any{
				"action": action,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
			assert.Equal(t, "0357", errResp.Code)
			assert.Equal(t, "Validation Error", errResp.Title)
			assert.Contains(t, errResp.Message, "action")
		})
	}
}

// TestUpdateRule_2_4_19_RejectsInvalidScopeFormatOnUpdate verifies scope validation on update.
func TestUpdateRule_2_4_19_RejectsInvalidScopeFormatOnUpdate(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Scope format rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"scopes": []map[string]any{
			{},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
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
	assert.Equal(t, "0358", errResp.Code)
	assert.Equal(t, "Validation Error", errResp.Title)
	assert.Contains(t, strings.ToLower(errResp.Message), "scope")
}

// TestUpdateRule_2_4_20_WithoutAuthenticationReturns401 verifies authentication requirement for update.
func TestUpdateRule_2_4_20_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth update rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	reqBody := map[string]any{
		"name": "Updated name",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for missing API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")

	// Verify it works with auth
	req2, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(body))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// =============================================================================
// 2.5 POST /v1/rules/{ruleId}/activate - Activate Rule
// =============================================================================

// TestActivateRule_2_5_1_ActivatesDraftRule verifies DRAFT rule can be activated.
func TestActivateRule_2_5_1_ActivatesDraftRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Activatable draft rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	assert.Equal(t, "ACTIVE", result["status"])
	assert.NotNil(t, result["activatedAt"])

	// Verify via GET
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var getResult map[string]any
	err = json.Unmarshal(getRespBody, &getResult)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", getResult["status"])
}

// TestActivateRule_2_5_2_ActivatesInactiveRule verifies INACTIVE rule can be reactivated.
func TestActivateRule_2_5_2_ActivatesInactiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Reactivatable rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", result["status"])
	assert.NotNil(t, result["activatedAt"])
}

// TestActivateRule_2_5_3_IdempotentActivationOfActiveRule verifies activating an ACTIVE rule succeeds (no-op).
func TestActivateRule_2_5_3_IdempotentActivationOfActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Already active rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Get original activatedAt
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var getResult map[string]any
	err = json.Unmarshal(getRespBody, &getResult)
	require.NoError(t, err)
	originalActivatedAt := getResult["activatedAt"]

	// Activate again
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", result["status"])
	assert.Equal(t, originalActivatedAt, result["activatedAt"])
}

// TestActivateRule_2_5_4_RevalidatesExpressionOnActivation verifies expression is validated before activation.
func TestActivateRule_2_5_4_RevalidatesExpressionOnActivation(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule with valid expression
	ruleID := testutil.CreateTestRuleWithExpression(t, "Revalidate rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Corrupt expression directly in database
	db := testutil.SetupIntegrationDB(t)

	_, err := db.ExecContext(context.Background(),
		"UPDATE rules SET expression = 'invalid >> syntax' WHERE id = $1", ruleID)
	require.NoError(t, err, "Failed to corrupt expression in database")

	// Try to activate
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.True(t, errResp.Code == "0340" || errResp.Code == "0341",
		"Expected TRC-0083 or TRC-0084, got %s", errResp.Code)
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.NotEmpty(t, errResp.Message)
}

// TestActivateRule_2_5_5_RejectsActivationOfDeletedRule verifies DELETED rules cannot be activated.
func TestActivateRule_2_5_5_RejectsActivationOfDeletedRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Deleted rule", "true", "ALLOW")
	testutil.DeactivateRule(t, ruleID)
	testutil.DeleteRuleViaAPI(t, ruleID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code)
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestActivateRule_2_5_6_RejectsInvalidUUIDInPath verifies path parameter validation.
func TestActivateRule_2_5_6_RejectsInvalidUUIDInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/not-a-uuid/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0065", errResp.Code) // Invalid path parameter (UUID format)
	assert.Equal(t, "Invalid Path Parameter", errResp.Title)
	assert.Equal(t, "Invalid rule ID format", errResp.Message)
}

// TestActivateRule_2_5_7_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestActivateRule_2_5_7_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestActivateRule_2_5_8_WithoutAuthenticationReturns401 verifies authentication requirement for activation.
func TestActivateRule_2_5_8_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth activate rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for missing API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// =============================================================================
// 2.6 POST /v1/rules/{ruleId}/deactivate - Deactivate Rule
// =============================================================================

// TestDeactivateRule_2_6_1_DeactivatesActiveRule verifies ACTIVE rule can be deactivated.
func TestDeactivateRule_2_6_1_DeactivatesActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Deactivatable active rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	assert.Equal(t, "INACTIVE", result["status"])
	assert.NotNil(t, result["deactivatedAt"])

	// Verify via GET
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var getResult map[string]any
	err = json.Unmarshal(getRespBody, &getResult)
	require.NoError(t, err)

	assert.Equal(t, "INACTIVE", getResult["status"])
}

// TestDeactivateRule_2_6_2_RejectsDeactivationOfDraftRule verifies DRAFT rule CANNOT be deactivated.
// State machine: DRAFT can only go to ACTIVE or DELETED, not INACTIVE.
func TestDeactivateRule_2_6_2_RejectsDeactivationOfDraftRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Draft rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// DRAFT → INACTIVE is not a valid transition
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Deactivate from DRAFT should return 400 - invalid transition")
}

// TestDeactivateRule_2_6_3_IdempotentDeactivationOfInactiveRule verifies deactivating an INACTIVE rule succeeds (no-op).
func TestDeactivateRule_2_6_3_IdempotentDeactivationOfInactiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Already inactive rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Get original deactivatedAt
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var getResult map[string]any
	err = json.Unmarshal(getRespBody, &getResult)
	require.NoError(t, err)
	originalDeactivatedAt := getResult["deactivatedAt"]

	// Deactivate again
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "INACTIVE", result["status"])
	assert.Equal(t, originalDeactivatedAt, result["deactivatedAt"])
}

// TestDeactivateRule_2_6_4_RejectsDeactivationOfDeletedRule verifies DELETED rules cannot be deactivated.
func TestDeactivateRule_2_6_4_RejectsDeactivationOfDeletedRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Deleted deactivation rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	testutil.DeleteRuleViaAPI(t, ruleID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code)
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestDeactivateRule_2_6_5_RejectsInvalidUUIDInPath verifies path parameter validation.
func TestDeactivateRule_2_6_5_RejectsInvalidUUIDInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/invalid-uuid/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0065", errResp.Code) // Invalid path parameter (UUID format)
	assert.Equal(t, "Invalid Path Parameter", errResp.Title)
	assert.Equal(t, "Invalid rule ID format", errResp.Message)
}

// TestDeactivateRule_2_6_6_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestDeactivateRule_2_6_6_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestDeactivateRule_2_6_7_WithoutAuthenticationReturns401 verifies authentication requirement for deactivation.
func TestDeactivateRule_2_6_7_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth deactivate rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for missing API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// =============================================================================
// 2.7 DELETE /v1/rules/{ruleId} - Delete Rule
// =============================================================================

// TestDeleteRule_2_7_1_DeletesInactiveRule verifies INACTIVE rule can be deleted.
func TestDeleteRule_2_7_1_DeletesInactiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Deletable inactive rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify via GET returns 404
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// TestDeleteRule_2_7_2_DeletesDraftRule verifies DRAFT rules CAN be deleted directly.
// State machine: DRAFT can go to ACTIVE or DELETED.
func TestDeleteRule_2_7_2_DeletesDraftRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Draft deletion rule", "true", "ALLOW")
	// No cleanup needed as we're deleting

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// DRAFT → DELETED is allowed
	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Delete DRAFT rule should return 204")
}

// TestDeleteRule_2_7_3_RejectsDeletionOfActiveRule verifies ACTIVE rules cannot be deleted directly.
func TestDeleteRule_2_7_3_RejectsDeletionOfActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Active deletion rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0349", errResp.Code) // Invalid rule status transition
	assert.Equal(t, "Invalid State Transition", errResp.Title)
	assert.Equal(t, "invalid status transition from ACTIVE to DELETED", errResp.Message)
}

// TestDeleteRule_2_7_4_RejectsDeletionOfAlreadyDeletedRule verifies already-deleted rules return 404.
func TestDeleteRule_2_7_4_RejectsDeletionOfAlreadyDeletedRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Already deleted rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	testutil.DeleteRuleViaAPI(t, ruleID)

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestDeleteRule_2_7_5_RejectsInvalidUUIDInPath verifies path parameter validation.
func TestDeleteRule_2_7_5_RejectsInvalidUUIDInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/not-a-uuid", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0065", errResp.Code) // Invalid path parameter (UUID format)
	assert.Equal(t, "Invalid Path Parameter", errResp.Title)
	assert.Equal(t, "Invalid rule ID format", errResp.Message)
}

// TestDeleteRule_2_7_6_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestDeleteRule_2_7_6_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestDeleteRule_2_7_7_WithoutAuthenticationReturns401 verifies authentication requirement for deletion.
func TestDeleteRule_2_7_7_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	// Create rule in DRAFT status (DRAFT can be deleted directly)
	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth delete rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code)
	assert.Equal(t, "Unauthorized", errResp.Title)
	assert.Equal(t, "API Key missing or invalid", errResp.Message)
}

// =============================================================================
// 2.8 POST /v1/rules/{ruleId}/draft - Draft Rule
// =============================================================================

// TestDraftRule_2_8_1_DraftsInactiveRule verifies INACTIVE rule can be transitioned to DRAFT.
func TestDraftRule_2_8_1_DraftsInactiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Draftable inactive rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	assert.Equal(t, "DRAFT", result["status"])
	assert.Nil(t, result["activatedAt"], "activatedAt should be nil after draft")
	assert.Nil(t, result["deactivatedAt"], "deactivatedAt should be nil after draft")

	// Verify via GET
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var getResult map[string]any
	err = json.Unmarshal(getRespBody, &getResult)
	require.NoError(t, err)

	assert.Equal(t, "DRAFT", getResult["status"])
}

// TestDraftRule_2_8_2_RejectsDraftOfActiveRule verifies ACTIVE rule CANNOT be drafted.
// State machine: ACTIVE can only go to INACTIVE.
func TestDraftRule_2_8_2_RejectsDraftOfActiveRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Active draft rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// ACTIVE → DRAFT is not a valid transition
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Draft from ACTIVE should return 400 - invalid transition")

	// Verify state was NOT mutated
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getRespBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "Response: %s", string(getRespBody))

	var result map[string]any
	err = json.Unmarshal(getRespBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", result["status"], "Rule should remain ACTIVE after rejected draft transition")
}

// TestDraftRule_2_8_3_IdempotentDraftOfDraftRule verifies drafting a DRAFT rule succeeds (no-op).
func TestDraftRule_2_8_3_IdempotentDraftOfDraftRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Already draft rule", "true", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Rule is already in DRAFT status after creation
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, ruleID, result["ruleId"])
	assert.Equal(t, "DRAFT", result["status"], "Status should remain DRAFT")
}

// TestDraftRule_2_8_4_RejectsInvalidUUIDInPath verifies path parameter validation.
func TestDraftRule_2_8_4_RejectsInvalidUUIDInPath(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/invalid-uuid/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0065", errResp.Code) // Invalid path parameter (UUID format)
	assert.Equal(t, "Invalid Path Parameter", errResp.Title)
	assert.Equal(t, "Invalid rule ID format", errResp.Message)
}

// TestDraftRule_2_8_5_Returns404ForNonExistentRuleID verifies error handling for missing resource.
func TestDraftRule_2_8_5_Returns404ForNonExistentRuleID(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/550e8400-e29b-41d4-a716-999999999999/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}

// TestDraftRule_2_8_6_WithoutAuthenticationReturns401 verifies authentication requirement for draft.
func TestDraftRule_2_8_6_WithoutAuthenticationReturns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Auth draft rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "Unauthenticated", errResp.Code, "Error code should be Unauthenticated for missing API key")
	assert.Equal(t, "Unauthorized", errResp.Title, "Error title should be Unauthorized")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "Error message should indicate API key issue")
}

// TestDraftRule_2_8_7_RejectsDraftOfDeletedRule verifies DELETED rules return 404.
func TestDraftRule_2_8_7_RejectsDraftOfDeletedRule(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	ruleID := testutil.CreateTestRuleWithExpression(t, "Deleted draft rule", "true", "ALLOW")
	testutil.ActivateRule(t, ruleID)
	testutil.DeactivateRule(t, ruleID)
	testutil.DeleteRuleViaAPI(t, ruleID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Soft-deleted rules are filtered by the repository, returning 404 (not found)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0347", errResp.Code) // Rule not found
	assert.Equal(t, "Not Found", errResp.Title)
	assert.Equal(t, "Rule not found", errResp.Message)
}
