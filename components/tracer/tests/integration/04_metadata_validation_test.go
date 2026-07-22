// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Metadata Validation Tests - POST /v1/validations
// =============================================================================
//
// These tests verify metadata field validation constraints:
//   - Maximum 50 entries per metadata map
//   - Keys: alphanumeric + underscore only
//   - Key length: maximum 64 characters
//
// Tests from roteiro section 4.8:
//   - 4.8.1: Metadata with exactly 50 entries (boundary test - valid)
//   - 4.8.2: Metadata exceeds 50 entries (invalid)
//   - 4.8.3: Metadata key with invalid characters
//   - 4.8.4: Metadata key exceeds 64 characters
//   - 4.8.5: Metadata key at exactly 64 characters (boundary test - valid)
//   - 4.8.6: Metadata with different value types (JSON-serializable)
//   - 4.8.7: Metadata key-value access in CEL expression
//
// Reference: API Design 6.16 MetadataMap
// =============================================================================

// TestValidation_Metadata_MaxEntries_BoundaryValid verifies 50 entries is accepted.
// Test 4.8.1 from roteiro
// Reference: API Design 6.16 MetadataMap (max 50 entries)
func TestValidation_Metadata_MaxEntries_BoundaryValid(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create metadata with exactly 50 entries (at boundary limit)
	metadata := make(map[string]any)
	for i := 1; i <= 50; i++ {
		key := fmt.Sprintf("field%02d", i)
		value := fmt.Sprintf("value%02d", i)
		metadata[key] = value
	}

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4301).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4302).String(),
			"type":      "checking",
			"status":    "active",
		},
		"metadata": metadata,
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Should be accepted at boundary (50 entries)
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"Request with exactly 50 metadata entries should be accepted. Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.NotEmpty(t, result["validationId"], "Should return valid response")
}

// TestValidation_Metadata_ExceedsMaxEntries verifies 51 entries is rejected.
// Test 4.8.2 from roteiro
// Reference: API Design 6.16 MetadataMap (max 50 entries)
func TestValidation_Metadata_ExceedsMaxEntries(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create metadata with 51 entries (exceeds limit)
	metadata := make(map[string]any)
	for i := 1; i <= 51; i++ {
		key := fmt.Sprintf("field%02d", i)
		value := fmt.Sprintf("value%02d", i)
		metadata[key] = value
	}

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4303).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4304).String(),
			"type":      "checking",
			"status":    "active",
		},
		"metadata": metadata,
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Request with 51 metadata entries should be rejected. Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0335", errResp.Code, "Expected 0335 for metadata entries exceeded")
	assert.Equal(t, "Metadata Entries Exceeded", errResp.Title, "Error title should match the metadata error")
	assert.Equal(t, "Metadata entries exceed maximum of 50.", testutil.ParseErrorResponse(t, respBody).Detail, "Error detail should match exactly")
}

// TestValidation_Metadata_KeyWithInvalidCharacters verifies invalid key chars are rejected.
// Test 4.8.3 from roteiro
// Reference: API Design 6.16 MetadataMap (keys: alphanumeric + underscore only)
func TestValidation_Metadata_KeyWithInvalidCharacters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		key         string
		description string
	}{
		{
			name:        "special_char_at",
			key:         "key@invalid",
			description: "Metadata key contains '@' character",
		},
		{
			name:        "special_char_dash",
			key:         "key-invalid",
			description: "Metadata key contains '-' character",
		},
		{
			name:        "special_char_dot",
			key:         "key.invalid",
			description: "Metadata key contains '.' character",
		},
		{
			name:        "special_char_space",
			key:         "key invalid",
			description: "Metadata key contains space",
		},
		{
			name:        "special_char_hash",
			key:         "key#invalid",
			description: "Metadata key contains '#' character",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(int64(4305 + i*2)).String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(4306 + i*2)).String(),
					"type":      "checking",
					"status":    "active",
				},
				"metadata": map[string]any{
					"valid_key": "value1",
					tc.key:      "value2", // Invalid key
				},
			}

			body, err := json.Marshal(payload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, "0336", errResp.Code,
				"Test case: %s - Expected 0336 for invalid metadata key characters", tc.description)
			assert.Equal(t, "Metadata Key Invalid Chars", errResp.Title, "Error title should match the metadata error")
			assert.Equal(t, "Metadata key contains invalid characters.", testutil.ParseErrorResponse(t, respBody).Detail, "Error detail should match exactly")
		})
	}
}

// TestValidation_Metadata_KeyExceedsMaxLength verifies key > 64 chars is rejected.
// Test 4.8.4 from roteiro
// Reference: API Design 6.16 MetadataMap (max 64 chars per key)
func TestValidation_Metadata_KeyExceedsMaxLength(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create metadata key with 65 characters (exceeds limit)
	longKey := strings.Repeat("a", 65)

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4315).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4316).String(),
			"type":      "checking",
			"status":    "active",
		},
		"metadata": map[string]any{
			longKey: "value",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Metadata key with 65 characters should be rejected. Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0050", errResp.Code, "Expected 0050 for metadata key length exceeded")
	assert.Equal(t, "Metadata Key Length Exceeded", errResp.Title, "Error title should match the metadata error")
	assert.Contains(t, testutil.ParseErrorResponse(t, respBody).Detail, "exceeds the maximum allowed length of", "Error detail should describe the key-length violation")
}

// TestValidation_Metadata_KeyAtMaxLength_BoundaryValid verifies 64 chars is accepted.
// Test 4.8.5 from roteiro
// Reference: API Design 6.16 MetadataMap (max 64 chars per key)
func TestValidation_Metadata_KeyAtMaxLength_BoundaryValid(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create metadata key with exactly 64 characters (at boundary limit)
	key64chars := strings.Repeat("a", 64)

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4317).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4318).String(),
			"type":      "checking",
			"status":    "active",
		},
		"metadata": map[string]any{
			key64chars: "value",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Should be accepted at boundary (64 characters)
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"Metadata key with exactly 64 characters should be accepted. Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.NotEmpty(t, result["validationId"], "Should return valid response")
}

// TestValidation_Metadata_DifferentValueTypes verifies metadata accepts all JSON-serializable types.
// Test 4.8.6 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.16 MetadataMap ("Values: Any JSON-serializable type")
func TestValidation_Metadata_DifferentValueTypes(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request with metadata containing various JSON types
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4319).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4320).String(),
			"type":      "checking",
			"status":    "active",
		},
		"metadata": map[string]any{
			"string_value":  "text",
			"number_value":  12345,
			"boolean_value": true,
			"float_value":   123.45,
			"nested_object": map[string]any{
				"key":    "value",
				"nested": map[string]any{"deep": "data"},
			},
			"array_value": []string{"item1", "item2", "item3"},
			"null_value":  nil,
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// VALIDATIONS: All value types should be accepted
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"Metadata with different JSON types should be accepted. Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.NotEmpty(t, result["validationId"], "Should return valid response")
}

// TestValidation_Metadata_CELExpressionAccess verifies CEL can access metadata key-value pairs.
// Test 4.8.7 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.16 MetadataMap
func TestValidation_Metadata_CELExpressionAccess(t *testing.T) {
	// PRECONDITIONS: Create rule that accesses metadata fields
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Metadata CEL Access Rule",
		`size(metadata) > 0 && metadata["channel"] == "mobile" && metadata["user_tier"] == "gold"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation with matching metadata
	payload := testutil.CreateBasicValidationPayload()
	payload["metadata"] = map[string]any{
		"channel":     "mobile",
		"user_tier":   "gold",
		"app_version": "3.2.1",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule should match (CEL successfully accessed metadata)
	assert.Equal(t, "ALLOW", result["decision"])
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.Contains(t, matchedRuleIDs, ruleID,
		"Rule should match - CEL can access metadata key-value pairs")

	// EXECUTION 2: Send validation with non-matching metadata
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["metadata"] = map[string]any{
		"channel":   "web",    // Different value
		"user_tier": "silver", // Different value
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule should NOT match (metadata values differ)
	matchedRuleIDs2, ok := result2["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotContains(t, matchedRuleIDs2, ruleID,
		"Rule should NOT match when metadata values differ")
}
