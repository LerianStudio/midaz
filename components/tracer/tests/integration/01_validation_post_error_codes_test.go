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
// Error Code Tests for Validation Handler (POST /v1/validations)
// =============================================================================
//
// These tests verify the error handling behavior for the validation
// endpoint POST /v1/validations. Errors use the RFC 9457 problem+json
// envelope with numeric codes from pkg/constant/errors.go; the `title` is the
// humanized sentinel name and the human-readable text is carried in `detail`
// (the retired `message` field is empty).
//
// Error Code Mapping (numeric registry):
//   - Body parse failure (malformed JSON / wrong type) → 0094 (ErrInvalidRequestBody), title "Bad Request"
//   - Empty body → generic 400 with no code, title "Bad Request"
//   - ErrValidationRequestIDRequired → 0413
//   - ErrValidationInvalidTransactionType → 0414
//   - ErrValidationAmountNonPositive → 0415
//   - ErrValidationCurrencyRequired → 0416
//   - ErrValidationInvalidCurrency → 0417
//   - ErrValidationTimestampRequired → 0418
//   - ErrValidationTimestampFuture → 0419
//   - ErrValidationAccountRequired → 0420
//   - ErrValidationSubTypeTooLong → 0425
// =============================================================================

// =============================================================================
// 1. Malformed JSON Tests - POST /v1/validations
// =============================================================================
// These test cases verify behavior when the request body cannot be parsed as JSON.
// Returns 0094 (ErrInvalidRequestBody) with title "Bad Request" for malformed JSON.
// =============================================================================

// TestValidation_MalformedJSON_ReturnsError verifies that completely malformed JSON returns an error.
// 0094 (ErrInvalidRequestBody): Bad Request for malformed JSON
func TestValidation_MalformedJSON_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		body        string
		description string
	}{
		{
			name:        "truncated_json",
			body:        `{"requestId": "550e8400-e29b-41d4-a716-446655440000", "transactionType":`,
			description: "JSON truncated mid-value",
		},
		{
			name:        "missing_quotes_on_keys",
			body:        `{requestId: "550e8400-e29b-41d4-a716-446655440000", transactionType: "CARD"}`,
			description: "JSON with unquoted keys",
		},
		{
			name:        "trailing_comma",
			body:        `{"requestId": "550e8400-e29b-41d4-a716-446655440000", "transactionType": "CARD",}`,
			description: "JSON with trailing comma",
		},
		{
			name:        "unclosed_brace",
			body:        `{"requestId": "550e8400-e29b-41d4-a716-446655440000", "transactionType": "CARD"`,
			description: "JSON with unclosed brace",
		},
		{
			name:        "single_quotes",
			body:        `{'requestId': '550e8400-e29b-41d4-a716-446655440000', 'transactionType': 'CARD'}`,
			description: "JSON with single quotes instead of double quotes",
		},
		{
			name:        "random_text",
			body:        `this is not json at all`,
			description: "Plain text instead of JSON",
		},
		{
			name:        "xml_instead_of_json",
			body:        `<validation><requestId>550e8400-e29b-41d4-a716-446655440000</requestId></validation>`,
			description: "XML instead of JSON",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", strings.NewReader(tc.body))
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

			// 0094 (ErrInvalidRequestBody): Bad Request for malformed JSON.
			// Human-readable text lives in the RFC 9457 `detail`; the retired `message` field is empty.
			assert.Equal(t, "0094", errResp.Code, "Test case: %s - Expected 0094 for malformed JSON", tc.description)
			assert.Equal(t, "Bad Request", errResp.Title, "Test case: %s", tc.description)
			assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail, "Test case: %s", tc.description)
		})
	}
}

// TestValidation_BinaryData_ReturnsError verifies that binary data returns an error.
// 0094 (ErrInvalidRequestBody): Bad Request for binary data
func TestValidation_BinaryData_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Binary data that's not valid JSON
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(binaryData))
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

	// 0094 (ErrInvalidRequestBody): Bad Request for binary data
	assert.Equal(t, "0094", errResp.Code, "Expected 0094 for binary data")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail)
}

// TestValidation_EmptyBody_ReturnsError verifies that an empty request body returns an error.
// Empty body: generic 400 Bad Request with no domain error code
func TestValidation_EmptyBody_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", strings.NewReader(""))
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

	// Empty body: generic 400 Bad Request carrying no domain error code.
	assert.Equal(t, "", errResp.Code, "Empty body returns a generic 400 with no domain error code")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "", errResp.Message)
}

// TestValidation_NullBody_ReturnsError verifies that a null JSON body returns an error.
// "null" is valid JSON but triggers validation errors for missing required fields.
func TestValidation_NullBody_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", strings.NewReader("null"))
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

	// "null" is valid JSON but missing required fields - first validation fails on requestId.
	// 0413 (ErrValidationRequestIDRequired); title is the humanized sentinel name.
	assert.Equal(t, "0413", errResp.Code, "Expected 0413 for null body (requestId is required)")
	assert.Equal(t, "Validation Request IDRequired", errResp.Title)
	assert.Equal(t, "RequestId is required.", errResp.Detail)
}

// TestValidation_ArrayInsteadOfObject_ReturnsError verifies that a JSON array returns an error.
// Valid JSON but wrong type - BodyParser fails to unmarshal array into struct.
func TestValidation_ArrayInsteadOfObject_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Valid JSON but wrong type (array instead of object)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", strings.NewReader(`[{"requestId": "550e8400-e29b-41d4-a716-446655440000"}]`))
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

	// 0094 (ErrInvalidRequestBody): fails to unmarshal array into struct
	assert.Equal(t, "0094", errResp.Code, "Expected 0094 for array instead of object (body parsing error)")
	assert.Equal(t, "Bad Request", errResp.Title)
	assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail)
}

// =============================================================================
// 2. Field Validation Tests - POST /v1/validations
// =============================================================================
// These test cases verify behavior when request has valid JSON but fails field validation.
// Each field validation error uses a specific numeric error code (0413 to 0430).
// =============================================================================

// TestValidation_MissingRequestID_ReturnsError verifies that missing requestId returns an error.
// 0413: requestId is required
func TestValidation_MissingRequestID_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Valid JSON with all fields except requestId
	payload := map[string]any{
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2001).String(),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0413 (ErrValidationRequestIDRequired): requestId is required
	assert.Equal(t, "0413", errResp.Code, "Expected 0413 for missing requestId")
	assert.Equal(t, "Validation Request IDRequired", errResp.Title)
	assert.Equal(t, "RequestId is required.", errResp.Detail)
}

// TestValidation_InvalidTransactionType_ReturnsError verifies invalid transactionType returns an error.
// 0414: transactionType must be one of [CARD, WIRE, PIX, CRYPTO]
func TestValidation_InvalidTransactionType_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name            string
		transactionType string
		description     string
	}{
		{
			name:            "invalid_value",
			transactionType: "INVALID",
			description:     "Unknown transaction type value",
		},
		{
			name:            "lowercase_card",
			transactionType: "card",
			description:     "Lowercase transaction type",
		},
		{
			name:            "empty_string",
			transactionType: "",
			description:     "Empty transaction type",
		},
		{
			name:            "cash_type",
			transactionType: "CASH",
			description:     "CASH is not a valid transaction type",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(int64(2002 + i*2)).String(),
				"transactionType":      tc.transactionType,
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(2003 + i*2)).String(),
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// 0414 (ErrValidationInvalidTransactionType): transactionType must be one of [CARD, WIRE, PIX, CRYPTO]
			assert.Equal(t, "0414", errResp.Code, "Test case: %s - Expected 0414 for invalid transactionType", tc.description)
			assert.Equal(t, "Validation Invalid Transaction Type", errResp.Title)
			assert.Equal(t, "Invalid transactionType.", errResp.Detail)
		})
	}
}

// TestValidation_AmountNonPositive_ReturnsError verifies that zero or negative amount returns an error.
// 0415: amount must be positive
func TestValidation_AmountNonPositive_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		amount      int64
		description string
	}{
		{
			name:        "zero_amount",
			amount:      0,
			description: "Amount is zero",
		},
		{
			name:        "negative_amount",
			amount:      -1,
			description: "Amount is negative",
		},
		{
			name:        "large_negative",
			amount:      -9999999,
			description: "Large negative amount",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(int64(2010 + i*2)).String(),
				"transactionType":      "CARD",
				"amount":               tc.amount,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(2011 + i*2)).String(),
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// 0415 (ErrValidationAmountNonPositive): amount must be positive
			assert.Equal(t, "0415", errResp.Code, "Test case: %s - Expected 0415 for non-positive amount", tc.description)
			assert.Equal(t, "Validation Amount Non Positive", errResp.Title)
			assert.Equal(t, "Amount must be positive.", errResp.Detail)
		})
	}
}

// TestValidation_MissingCurrency_ReturnsError verifies that missing currency returns an error.
// 0416: currency is required
func TestValidation_MissingCurrency_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2016).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2017).String(),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0416 (ErrValidationCurrencyRequired): currency is required
	assert.Equal(t, "0416", errResp.Code, "Expected 0416 for missing currency")
	assert.Equal(t, "Validation Currency Required", errResp.Title)
	assert.Equal(t, "Currency is required.", errResp.Detail)
}

// TestValidation_InvalidCurrency_ReturnsError verifies that invalid currency code returns an error.
// 0417: currency must be valid ISO 4217 code (e.g., BRL, USD)
func TestValidation_InvalidCurrency_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		currency    string
		description string
	}{
		{
			name:        "lowercase_brl",
			currency:    "brl",
			description: "Lowercase currency code",
		},
		{
			name:        "invalid_length_4_chars",
			currency:    "USDD",
			description: "Currency code with 4 characters",
		},
		{
			name:        "invalid_length_2_chars",
			currency:    "US",
			description: "Currency code with 2 characters",
		},
		{
			name:        "numeric_currency",
			currency:    "123",
			description: "Numeric currency code",
		},
		{
			name:        "invalid_but_formatted",
			currency:    "XYZ",
			description: "Properly formatted but invalid ISO 4217 code",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(int64(2018 + i*2)).String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             tc.currency,
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(2019 + i*2)).String(),
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// 0417 (ErrValidationInvalidCurrency): currency must be valid ISO 4217 code (e.g., BRL, USD)
			assert.Equal(t, "0417", errResp.Code, "Test case: %s - Expected 0417 for invalid currency", tc.description)
			assert.Equal(t, "Validation Invalid Currency", errResp.Title)
			assert.Equal(t, "Currency must be valid ISO 4217.", errResp.Detail)
		})
	}
}

// TestValidation_MissingTimestamp_ReturnsError verifies that missing timestamp returns an error.
// 0418: transactionTimestamp is required
func TestValidation_MissingTimestamp_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	payload := map[string]any{
		"requestId":       testutil.MustDeterministicUUID(2028).String(),
		"transactionType": "CARD",
		"amount":          100,
		"currency":        "BRL",
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2029).String(),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0418 (ErrValidationTimestampRequired): transactionTimestamp is required
	assert.Equal(t, "0418", errResp.Code, "Expected 0418 for missing timestamp")
	assert.Equal(t, "Validation Timestamp Required", errResp.Title)
	assert.Equal(t, "Timestamp is required.", errResp.Detail)
}

// TestValidation_FutureTimestamp_ReturnsError verifies that future timestamp returns an error.
// 0419: transactionTimestamp cannot be in the future
func TestValidation_FutureTimestamp_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use a timestamp 1 hour in the future (well beyond clock skew tolerance)
	// NOTE: This test intentionally uses time.Now() to verify real-time future validation
	futureTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2030).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": futureTime,
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2031).String(),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0419 (ErrValidationTimestampFuture): transactionTimestamp cannot be in the future
	assert.Equal(t, "0419", errResp.Code, "Expected 0419 for future timestamp")
	assert.Equal(t, "Validation Timestamp Future", errResp.Title)
	assert.Equal(t, "Timestamp cannot be in the future.", errResp.Detail)
}

// TestValidation_FutureTimestamp_SmallClockSkew_IsAccepted verifies that small
// clock skew (2 seconds in the future) is tolerated by the API.
// This documents the API's clock skew tolerance behavior.
func TestValidation_FutureTimestamp_SmallClockSkew_IsAccepted(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use a timestamp just 2 seconds in the future (within clock skew tolerance)
	// NOTE: This test intentionally uses time.Now() to verify real-time clock skew tolerance
	futureTime := time.Now().Add(2 * time.Second).Format(time.RFC3339)

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2032).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": futureTime,
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2033).String(),
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

	// Small clock skew (2 seconds) is tolerated - API accepts the request
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"Small clock skew (2 seconds) should be tolerated. Response: %s", string(respBody))

	// Verify we got a valid validation response
	var validationResp map[string]any
	err = json.Unmarshal(respBody, &validationResp)
	require.NoError(t, err)
	assert.NotEmpty(t, validationResp["validationId"], "Response should contain validationId")
	assert.NotEmpty(t, validationResp["decision"], "Response should contain decision")
}

// TestValidation_MissingAccount_ReturnsError verifies that missing account object returns an error.
// 0420: account is required
func TestValidation_MissingAccount_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2034).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0420 (ErrValidationAccountRequired): account is required
	assert.Equal(t, "0420", errResp.Code, "Expected 0420 for missing account")
	assert.Equal(t, "Validation Account Required", errResp.Title)
	assert.Equal(t, "Account is required.", errResp.Detail)
}

// TestValidation_EmptyAccountObject_ReturnsError verifies that empty account object returns an error.
// 0420: account is required
func TestValidation_EmptyAccountObject_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2035).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account":              map[string]any{}, // Empty account object (missing accountId)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0420 (ErrValidationAccountRequired): account is required
	assert.Equal(t, "0420", errResp.Code, "Expected 0420 for empty account object")
	assert.Equal(t, "Validation Account Required", errResp.Title)
	assert.Equal(t, "Account is required.", errResp.Detail)
}

// TestValidation_SubTypeTooLong_ReturnsError verifies that subType exceeding max length returns an error.
// 0425: subType exceeds maximum length of 50 characters
func TestValidation_SubTypeTooLong_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create a subType longer than 50 characters
	longSubType := strings.Repeat("a", 51)

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2036).String(),
		"transactionType":      "CARD",
		"subType":              longSubType,
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2037).String(),
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)

	// 0425 (ErrValidationSubTypeTooLong): subType exceeds maximum length of 50 characters
	assert.Equal(t, "0425", errResp.Code, "Expected 0425 for subType too long")
	assert.Equal(t, "Validation Sub Type Too Long", errResp.Title)
	assert.Equal(t, "SubType exceeds maximum length of 50 characters.", errResp.Detail)
}

// =============================================================================
// 3. Authentication Tests - POST /v1/validations
// =============================================================================

// TestValidation_WithoutAuth_Returns401 verifies that missing API key returns 401.
func TestValidation_WithoutAuth_Returns401(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(2038).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(2039).String(),
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header set

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
// 4. Edge Case Tests - POST /v1/validations
// =============================================================================

// TestValidation_InvalidUUIDFormat_ReturnsError verifies that invalid UUID format returns an error.
func TestValidation_InvalidUUIDFormat_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		requestID   string
		description string
	}{
		{
			name:        "not_a_uuid",
			requestID:   "not-a-uuid",
			description: "Plain string instead of UUID",
		},
		{
			name:        "partial_uuid",
			requestID:   "550e8400-e29b-41d4",
			description: "Partial UUID",
		},
		{
			name:        "uuid_with_extra_chars",
			requestID:   "550e8400-e29b-41d4-a716-446655440000-extra",
			description: "UUID with extra characters",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            tc.requestID,
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(2040 + i)).String(),
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// UUID parsing errors happen during body parsing → 0094 (ErrInvalidRequestBody)
			assert.Equal(t, "0094", errResp.Code, "Test case: %s - Expected 0094 for invalid UUID (body parsing error)", tc.description)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail, "Test case: %s", tc.description)
		})
	}
}

// TestValidation_InvalidTimestampFormat_ReturnsError verifies that invalid timestamp format returns an error.
func TestValidation_InvalidTimestampFormat_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		timestamp   string
		description string
	}{
		{
			name:        "date_only",
			timestamp:   "2024-01-15",
			description: "Date without time",
		},
		{
			name:        "no_timezone",
			timestamp:   "2024-01-15T10:30:00",
			description: "DateTime without timezone",
		},
		{
			name:        "unix_timestamp",
			timestamp:   "1704067200",
			description: "Unix timestamp as string",
		},
		{
			name:        "invalid_format",
			timestamp:   "15/01/2024 10:30:00",
			description: "Date in wrong format",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"requestId":            testutil.MustDeterministicUUID(int64(2043 + i*2)).String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": tc.timestamp,
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(int64(2044 + i*2)).String(),
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Test case: %s - Response: %s", tc.description, string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// Timestamp parsing errors happen during body parsing → 0094 (ErrInvalidRequestBody)
			assert.Equal(t, "0094", errResp.Code, "Test case: %s - Expected 0094 for invalid timestamp (body parsing error)", tc.description)
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail, "Test case: %s", tc.description)
		})
	}
}

// TestValidation_ValidJSONWithWrongTypes_ReturnsError verifies that valid JSON with wrong types returns an error.
func TestValidation_ValidJSONWithWrongTypes_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "string_for_amount",
			payload: map[string]any{
				"requestId":            testutil.MustDeterministicUUID(2051).String(),
				"transactionType":      "CARD",
				"amount":               "not_a_number", // Non-numeric string that decimal.Decimal rejects
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(2052).String(),
				},
			},
		},
		{
			name: "array_for_account",
			payload: map[string]any{
				"requestId":            testutil.MustDeterministicUUID(2053).String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account":              []string{testutil.MustDeterministicUUID(2054).String()}, // Array instead of object
			},
		},
		{
			name: "number_for_transactionType",
			payload: map[string]any{
				"requestId":            testutil.MustDeterministicUUID(2055).String(),
				"transactionType":      123, // Number instead of string
				"amount":               100,
				"currency":             "BRL",
				"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
				"account": map[string]any{
					"accountId": testutil.MustDeterministicUUID(2056).String(),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.payload)
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)

			// 0094 (ErrInvalidRequestBody) is returned for JSON type mismatch errors (bad request)
			assert.Equal(t, "0094", errResp.Code, "Expected 0094 for type validation error")
			assert.Equal(t, "Bad Request", errResp.Title)
			assert.Equal(t, "The request body is malformed or contains invalid JSON. Please verify the syntax and try again.", errResp.Detail)
		})
	}
}

// =============================================================================
// 5. Error Code Documentation
// =============================================================================
//
// Error Code Mapping for Validation Handler (numeric registry, pkg/constant/errors.go):
// ------------------------------------------
// 0094: ErrInvalidRequestBody - Body parsing errors (malformed JSON, wrong type, unparseable request), title "Bad Request"
// 0413: ErrValidationRequestIDRequired - requestId is required
// 0414: ErrValidationInvalidTransactionType - transactionType invalid
// 0415: ErrValidationAmountNonPositive - amount must be positive
// 0416: ErrValidationCurrencyRequired - currency is required
// 0417: ErrValidationInvalidCurrency - currency invalid ISO 4217
// 0418: ErrValidationTimestampRequired - transactionTimestamp is required
// 0419: ErrValidationTimestampFuture - transactionTimestamp in future
// 0420: ErrValidationAccountRequired - account is required
// 0422: ErrValidationTimeout - validation timeout
// 0423: ErrValidationSegmentIDRequired - segment.id is required
// 0424: ErrValidationPortfolioIDRequired - portfolio.id is required
// 0425: ErrValidationSubTypeTooLong - subType exceeds 50 characters
// 0426: ErrValidationInvalidAccountType - account.type invalid
// 0427: ErrValidationInvalidAccountStatus - account.status invalid
// 0428: ErrValidationInvalidMerchantCategory - merchant.category invalid
// 0429: ErrValidationInvalidMerchantCountry - merchant.country invalid
// 0430: ErrValidationMerchantIDRequired - merchant.id is required
//
// Metadata validation errors:
// 0050: ErrMetadataKeyLengthExceeded - metadata key too long
// 0335: ErrMetadataEntriesExceeded - too many metadata entries
// 0336: ErrMetadataKeyInvalidChars - metadata key invalid chars
