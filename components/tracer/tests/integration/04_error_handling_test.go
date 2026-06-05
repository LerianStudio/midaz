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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Error Handling Tests - POST /v1/validations
// =============================================================================
//
// These tests verify input validation and error handling for the validation endpoint.
// Many error handling tests already exist in 01_validation_*.go.
// This file contains ONLY the critical tests from roteiro 04.4 that follow
// the naming conventions and patterns of other 04_*.go files.
//
// Tests from roteiro section 4.4:
//   - 4.4.1: Missing required field - requestId
//   - 4.4.2: Missing required field - amount
//   - 4.4.3: Missing required field - currency
//   - 4.4.4: Missing required field - transactionType
//   - 4.4.13b: Missing required field - transactionTimestamp
//   - 4.4.5: Invalid amount - zero value
//   - 4.4.6: Invalid amount - negative value
//   - 4.4.7: Invalid amount - string value
//
// Reference: API Design 3.2 POST /v1/validations Error Responses
//
// NOTE: Additional error handling tests exist in:
//   - 01_validation_test.go (tests 1.1.x series)
//   - 01_validation_post_error_codes_test.go
//   - 01_validation_get_error_codes_test.go
// =============================================================================

// TestValidation_ErrorHandling_MissingRequestId verifies 400 when requestId is missing.
// Test 4.4.1 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 4.1.1 ValidateTransaction, Change Log 1.3.2
func TestValidation_ErrorHandling_MissingRequestId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request WITHOUT requestId
	payload := map[string]any{
		// "requestId" intentionally omitted
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4601).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing requestId should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0220", errorResp.Code, "Error code should be TRC-0220")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "requestId is required", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_MissingAmount verifies 400 when amount is missing.
// Test 4.4.2 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 4.1.1 ValidateTransaction
func TestValidation_ErrorHandling_MissingAmount(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request WITHOUT amount
	payload := map[string]any{
		"requestId":       testutil.MustDeterministicUUID(4602).String(),
		"transactionType": "CARD",
		// "amount" intentionally omitted from JSON payload (server treats missing amount as invalid, triggering TRC-0222)
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4603).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing/zero amount should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0222", errorResp.Code, "Error code should be TRC-0222")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "amount must be positive", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_MissingCurrency verifies 400 when currency is missing.
// Test 4.4.3 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 4.1.1 ValidateTransaction
func TestValidation_ErrorHandling_MissingCurrency(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request WITHOUT currency
	payload := map[string]any{
		"requestId":       testutil.MustDeterministicUUID(4604).String(),
		"transactionType": "CARD",
		"amount":          "100.00",
		// "currency" intentionally omitted
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4605).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing currency should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0223", errorResp.Code, "Error code should be TRC-0223")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "currency is required", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_MissingTransactionType verifies 400 when transactionType is missing.
// Test 4.4.4 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 4.1.1 ValidateTransaction
func TestValidation_ErrorHandling_MissingTransactionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request WITHOUT transactionType
	payload := map[string]any{
		"requestId": testutil.MustDeterministicUUID(4606).String(),
		// "transactionType" intentionally omitted
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4607).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing transactionType should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0221", errorResp.Code, "Error code should be TRC-0221")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "transactionType must be one of [CARD, WIRE, PIX, CRYPTO]", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_MissingTransactionTimestamp verifies 400 when transactionTimestamp is missing.
// Test 4.4.13b from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 4.1.1 ValidateTransaction
func TestValidation_ErrorHandling_MissingTransactionTimestamp(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request WITHOUT transactionTimestamp
	payload := map[string]any{
		"requestId":       testutil.MustDeterministicUUID(4608).String(),
		"transactionType": "CARD",
		"amount":          "100.00",
		"currency":        "BRL",
		// "transactionTimestamp" intentionally omitted
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4609).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing transactionTimestamp should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0225", errorResp.Code, "Error code should be TRC-0225")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "transactionTimestamp is required", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_InvalidAmount_ZeroValue verifies 400 when amount is zero.
// Test 4.4.5 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 6.10 Monetary Amounts
func TestValidation_ErrorHandling_InvalidAmount_ZeroValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request with amount = 0
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4610).String(),
		"transactionType":      "CARD",
		"amount":               "0", // Zero is invalid per API Design 6.10
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4611).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Amount = 0 should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0222", errorResp.Code, "Error code should be TRC-0222")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "amount must be positive", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_InvalidAmount_NegativeValue verifies 400 when amount is negative.
// Test 4.4.6 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 6.10 Monetary Amounts
func TestValidation_ErrorHandling_InvalidAmount_NegativeValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request with negative amount
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4612).String(),
		"transactionType":      "CARD",
		"amount":               "-100.00", // Negative is invalid
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4613).String(),
			"type":      "checking",
			"status":    "active",
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

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Negative amount should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0222", errorResp.Code, "Error code should be TRC-0222")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "amount must be positive", errorResp.Message, "Error message should match exactly")
}

// TestValidation_ErrorHandling_InvalidAmount_StringValue verifies 400 when amount is string instead of integer.
// Test 4.4.7 from roteiro 04-rules-evaluation.md
// Reference: API Design 3.2, 6.10 Monetary Amounts
func TestValidation_ErrorHandling_InvalidAmount_StringValue(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request with amount as non-numeric string (JSON type mismatch)
	// Note: decimal.Decimal accepts numeric strings like "10000", so we use a truly invalid string
	jsonPayload := `{
		"requestId": "` + testutil.MustDeterministicUUID(4614).String() + `",
		"transactionType": "CARD",
		"amount": "not_a_number",
		"currency": "BRL",
		"transactionTimestamp": "` + testutil.FixedTime().Add(-1*time.Minute).Format(time.RFC3339) + `",
		"account": {
			"accountId": "` + testutil.MustDeterministicUUID(4615).String() + `",
			"type": "checking",
			"status": "active"
		}
	}`

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", strings.NewReader(jsonPayload))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"String value for amount should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "TRC-0003", errorResp.Code, "Error code should be TRC-0003 for type mismatch")
	assert.Equal(t, "Bad Request", errorResp.Title, "Error title should be Bad Request")
	assert.Contains(t, errorResp.Message, "invalid", "Error message should mention invalid request")
}

// TestValidation_ErrorHandling_InvalidCurrency_MixedCase verifies strict uppercase validation for currency.
// Test 4.4.9 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.9 Currency Code Validation
func TestValidation_ErrorHandling_InvalidCurrency_MixedCase(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name     string
		currency string
	}{
		{"mixed_case", "Brl"},
		{"lowercase", "brl"},
		{"partial_mixed", "BRl"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(4616).String(),
				"transactionType":      "CARD",
				"amount":               "100.00",
				"currency":             tc.currency, // Invalid case
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(4617).String(),
					"type":      "checking",
					"status":    "active",
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

			// VALIDATIONS
			require.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Non-uppercase currency '%s' should return 400 Bad Request", tc.currency)

			errorResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, "TRC-0224", errorResp.Code, "Error code should be TRC-0224 for invalid currency")
			assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
			assert.Equal(t, "currency must be valid ISO 4217 code (e.g., BRL, USD)", errorResp.Message, "Error message should match exactly")
		})
	}
}

// =============================================================================
// IMPLEMENTATION NOTES
// =============================================================================
//
// COVERAGE STATUS:
//
// This file implements 8 critical error handling tests from roteiro 04.4
// following the naming patterns of other 04_*.go files.
//
// ADDITIONAL ERROR HANDLING TESTS:
// The following tests from roteiro 04.4 are already covered in 01_validation_*.go:
//
//   ✅ 4.4.8  - Invalid currency lowercase (01_validation_test.go:1_1_51)
//   ✅ 4.4.10 - Invalid currency non-ISO (01_validation_test.go)
//   ✅ 4.4.11 - Invalid UUID requestId (01_validation_test.go:1_1_22)
//   ✅ 4.4.12 - Invalid UUID accountId (01_validation_test.go:1_1_23)
//   ✅ 4.4.13 - Invalid UUID segmentId (01_validation_test.go:1_1_24)
//   ✅ 4.4.14 - Invalid timestamp future (01_validation_test.go:1_1_14)
//   ✅ 4.4.15 - Valid timestamp clock skew (01_validation_test.go:1_1_36)
//   ✅ 4.4.16 - Missing authentication (TestValidation_RequiresAuthentication in 01_validation_test.go)
//   ✅ 4.4.17 - Invalid authentication (01_validation_test.go:1_1_31)
//   ✅ 4.4.18 - Payload too large (01_validation_test.go:1_1_9)
//   ✅ 4.4.19 - Service timeout (01_validation_test.go:1_1_27)
//   ✅ 4.4.20 - Service unavailable (01_validation_test.go:1_1_28)
//
// CROSS-REFERENCE:
// - 01_validation_test.go: Tests 1.1.x series (comprehensive error handling)
// - 01_validation_post_error_codes_test.go: Structured error code validation
// - 01_validation_get_error_codes_test.go: GET endpoint error handling
//
// Date: 2026-01-27
// Author: Claude Code (Automated Test Generation)
// =============================================================================
