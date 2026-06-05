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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
)

// =============================================================================
// Authentication & Authorization Tests (Category 6)
// =============================================================================
//
// These tests verify API Key authentication behavior:
// - Valid/invalid/missing API key handling
// - Protected vs public endpoint access
// - Security edge cases (injection, leakage)
//
// Error Response Structure:
//   code: "Unauthenticated"
//   title: "Unauthorized"
//   message: "API Key missing or invalid"
// =============================================================================

// validPayload returns a valid validation request payload for auth tests.
func validPayload(t *testing.T) []byte {
	t.Helper()

	payload := map[string]any{
		"requestId":            uuid.New().String(),
		"transactionType":      "PIX",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Format(time.RFC3339),
		"account": map[string]any{
			"accountId": "550e8400-e29b-41d4-a716-446655440001",
			"type":      "checking",
			"status":    "active",
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err, "failed to marshal validPayload")

	return data
}

// assertAuthError validates the standard authentication error response.
func assertAuthError(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected 401 Unauthorized, body: %s", string(body))

	var errResp testutil.ErrorResponse
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err, "failed to parse error response")

	assert.Equal(t, "Unauthenticated", errResp.Code, "unexpected error code")
	assert.Equal(t, "Unauthorized", errResp.Title, "unexpected error title")
	assert.Equal(t, "API Key missing or invalid", errResp.Message, "unexpected error message")
}

// =============================================================================
// 6.1.1 Accepts valid API Key
// =============================================================================

func TestAuth_6_1_1_AcceptsValidAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", testutil.GetAPIKey())

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "expected 201 Created, body: %s", string(body))

	var result map[string]any
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "failed to parse response")

	// Validate response fields
	assert.Contains(t, result, "validationId", "missing validationId")
	assert.Contains(t, result, "requestId", "missing requestId")
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision should be a string")
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, decision, "invalid decision value")
}

// =============================================================================
// 6.1.2 Rejects missing API Key
// =============================================================================

func TestAuth_6_1_2_RejectsMissingAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assertAuthError(t, resp, body)
}

// =============================================================================
// 6.1.3 Rejects invalid API Key
// =============================================================================

func TestAuth_6_1_3_RejectsInvalidAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "invalid-key-that-does-not-exist")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assertAuthError(t, resp, body)
}

// =============================================================================
// 6.1.4 Rejects empty API Key
// =============================================================================

func TestAuth_6_1_4_RejectsEmptyAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assertAuthError(t, resp, body)
}

// =============================================================================
// 6.1.5 API Key with leading/trailing whitespace (auto-trimmed per RFC 7230)
// =============================================================================
// RFC 7230 defines: header-field = field-name ":" OWS field-value OWS
// Where OWS (Optional WhiteSpace) MUST be ignored. Trimming is correct behavior.

func TestAuth_6_1_5_WhitespaceAPIKeyAccepted(t *testing.T) {
	validKey := testutil.GetAPIKey()

	tests := []struct {
		name   string
		apiKey string
	}{
		{"leading whitespace", " " + validKey},
		{"trailing whitespace", validKey + " "},
		{"leading and trailing whitespace", " " + validKey + " "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", tc.apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// RFC 7230: OWS (Optional WhiteSpace) around field-value must be trimmed
			assert.Equal(t, http.StatusCreated, resp.StatusCode,
				"API should accept key with whitespace (trimmed per RFC 7230), body: %s", string(body))

			// Verify response contains expected fields
			var result map[string]any
			err = json.Unmarshal(body, &result)
			require.NoError(t, err, "response should be valid JSON")
			assert.Contains(t, result, "validationId", "response should contain validationId")
			assert.Contains(t, result, "decision", "response should contain decision")
		})
	}
}

// =============================================================================
// 6.1.6 Header name case sensitivity
// =============================================================================

func TestAuth_6_1_6_HeaderCaseInsensitivity(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
	}{
		{"lowercase", "x-api-key"},
		{"mixed case", "X-Api-Key"},
		{"uppercase", "X-API-KEY"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.headerName, testutil.GetAPIKey())

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusCreated, resp.StatusCode,
				"header %s should be accepted (HTTP headers are case-insensitive), body: %s", tc.headerName, string(body))
		})
	}
}

// =============================================================================
// 6.1.7 Rejects malformed API Key formats
// =============================================================================

func TestAuth_6_1_7_RejectsMalformedAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
	}{
		{"XSS characters", "invalid<script>alert('xss')</script>key"},
		{"excessively long (1001 chars)", strings.Repeat("a", 1001)},
		{"only whitespace", "   "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", tc.apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assertAuthError(t, resp, body)
		})
	}
}

// =============================================================================
// 6.1.8 Handles duplicate X-API-Key headers
// =============================================================================

func TestAuth_6_1_8_DuplicateAPIKeyHeaders(t *testing.T) {
	validKey := testutil.GetAPIKey()
	invalidKey := "invalid-key"

	tests := []struct {
		name        string
		firstKey    string
		secondKey   string
		description string
	}{
		{"both valid same key", validKey, validKey, "both headers have same valid key"},
		{"first valid second invalid", validKey, invalidKey, "first header valid, second invalid"},
		{"first invalid second valid", invalidKey, validKey, "first header invalid, second valid"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Add("X-API-Key", tc.firstKey)
			req.Header.Add("X-API-Key", tc.secondKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Document actual behavior - Go's http middleware uses the first header value
			t.Logf("Duplicate headers test '%s': status=%d, body=%s", tc.name, resp.StatusCode, string(body))

			// Go's http middleware uses the first header value for authentication
			if tc.firstKey == validKey {
				assert.Equal(t, http.StatusCreated, resp.StatusCode, "first valid header should authenticate")
			} else {
				assertAuthError(t, resp, body)
			}
		})
	}
}

// =============================================================================
// 6.1.9 Rejects missing API Key on all protected endpoints
// =============================================================================

func TestAuth_6_1_9_ProtectedEndpointsRequireAuth(t *testing.T) {
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	ruleBody := `{"name":"Test Rule","description":"Test","expression":"amount > 100000","action":"REVIEW"}`
	limitBody := `{"name":"Test Limit","limitType":"DAILY","maxAmount":"100000.00","currency":"BRL","scopes":[{"accountId":"550e8400-e29b-41d4-a716-446655440000"}]}`
	validationBody := string(validPayload(t))

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		// Validations endpoints
		{"GET /v1/validations", http.MethodGet, "/v1/validations", ""},
		{"GET /v1/validations/{id}", http.MethodGet, "/v1/validations/" + testUUID, ""},
		{"POST /v1/validations", http.MethodPost, "/v1/validations", validationBody},

		// Rules endpoints
		{"GET /v1/rules", http.MethodGet, "/v1/rules", ""},
		{"GET /v1/rules/{id}", http.MethodGet, "/v1/rules/" + testUUID, ""},
		{"POST /v1/rules", http.MethodPost, "/v1/rules", ruleBody},
		{"PATCH /v1/rules/{id}", http.MethodPatch, "/v1/rules/" + testUUID, `{"name":"Updated"}`},
		{"DELETE /v1/rules/{id}", http.MethodDelete, "/v1/rules/" + testUUID, ""},
		{"POST /v1/rules/{id}/activate", http.MethodPost, "/v1/rules/" + testUUID + "/activate", ""},
		{"POST /v1/rules/{id}/deactivate", http.MethodPost, "/v1/rules/" + testUUID + "/deactivate", ""},

		// Limits endpoints
		{"GET /v1/limits", http.MethodGet, "/v1/limits", ""},
		{"GET /v1/limits/{id}", http.MethodGet, "/v1/limits/" + testUUID, ""},
		{"GET /v1/limits/{id}/usage", http.MethodGet, "/v1/limits/" + testUUID + "/usage", ""},
		{"POST /v1/limits", http.MethodPost, "/v1/limits", limitBody},
		{"PATCH /v1/limits/{id}", http.MethodPatch, "/v1/limits/" + testUUID, `{"name":"Updated"}`},
		{"DELETE /v1/limits/{id}", http.MethodDelete, "/v1/limits/" + testUUID, ""},
		{"POST /v1/limits/{id}/activate", http.MethodPost, "/v1/limits/" + testUUID + "/activate", ""},
		{"POST /v1/limits/{id}/deactivate", http.MethodPost, "/v1/limits/" + testUUID + "/deactivate", ""},

		// Audit events endpoints
		{"GET /v1/audit-events", http.MethodGet, "/v1/audit-events", ""},
		{"GET /v1/audit-events/{id}", http.MethodGet, "/v1/audit-events/" + testUUID, ""},
		{"GET /v1/audit-events/{id}/verify", http.MethodGet, "/v1/audit-events/" + testUUID + "/verify", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			}

			req, err := http.NewRequest(tc.method, testutil.GetBaseURL()+tc.path, bodyReader)
			require.NoError(t, err)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			// No X-API-Key header - testing missing auth

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assertAuthError(t, resp, body)
		})
	}
}

// =============================================================================
// 6.1.10 Public endpoints do not require auth
// =============================================================================

func TestAuth_6_1_10_PublicEndpointsNoAuthRequired(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		expectedStatus  int
		validateBody    func(t *testing.T, body []byte)
		validateHeaders func(t *testing.T, resp *http.Response)
	}{
		{
			name:           "/health",
			path:           "/health",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				// lib-commons Ping() returns plain text "healthy"
				assert.Equal(t, "healthy", string(body), "health endpoint should return 'healthy'")
			},
		},
		{
			name:           "/readyz",
			path:           "/readyz",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)
				status, ok := result["status"].(string)
				require.True(t, ok, "status should be a string")
				assert.Contains(t, []string{"healthy", "unhealthy"}, status,
					"readyz status must use canonical vocabulary")
				assert.Contains(t, result, "checks", "should have checks map")
			},
		},
		{
			name:           "/version",
			path:           "/version",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				// lib-commons Version() returns {version, requestDate}
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)
				assert.Contains(t, result, "version", "should have version field")
				assert.Contains(t, result, "requestDate", "should have requestDate field")
			},
		},
		{
			name:           "/swagger/index.html",
			path:           "/swagger/index.html",
			expectedStatus: http.StatusOK,
			validateHeaders: func(t *testing.T, resp *http.Response) {
				contentType := resp.Header.Get("Content-Type")
				assert.Contains(t, contentType, "text/html", "swagger index should return HTML")
			},
		},
		{
			name:           "/swagger/doc.json",
			path:           "/swagger/doc.json",
			expectedStatus: http.StatusOK,
			validateHeaders: func(t *testing.T, resp *http.Response) {
				contentType := resp.Header.Get("Content-Type")
				assert.Contains(t, contentType, "application/json", "swagger doc should return JSON")
			},
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)
				// OpenAPI spec should have these fields
				assert.True(t, result["openapi"] != nil || result["swagger"] != nil, "should be valid OpenAPI spec")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, testutil.GetBaseURL()+tc.path, nil)
			require.NoError(t, err)
			// No X-API-Key header

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode,
				"public endpoint %s should not require auth, body: %s", tc.path, string(body))

			if tc.validateHeaders != nil {
				tc.validateHeaders(t, resp)
			}
			if tc.validateBody != nil {
				tc.validateBody(t, body)
			}
		})
	}
}

// =============================================================================
// 6.1.11 Public endpoints ignore invalid API Key
// =============================================================================

func TestAuth_6_1_11_PublicEndpointsIgnoreInvalidKey(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{"/health", "/health", http.StatusOK},
		{"/readyz", "/readyz", http.StatusOK},
		{"/version", "/version", http.StatusOK},
		{"/swagger/index.html", "/swagger/index.html", http.StatusOK},
		{"/swagger/doc.json", "/swagger/doc.json", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, testutil.GetBaseURL()+tc.path, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", "invalid-key-that-does-not-exist")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode,
				"public endpoint %s should ignore invalid API key, body: %s", tc.path, string(body))
		})
	}
}

// =============================================================================
// 6.1.12 Dev mode allows disabling auth
// =============================================================================
// This test restarts the server with API_KEY_ENABLED=false to verify
// that authentication can be disabled for development environments.
// WARNING: This test is NOT safe for parallel execution.

func TestAuth_6_1_12_DevModeAuthDisabled(t *testing.T) {
	// Restart server with auth disabled
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"API_KEY_ENABLED": "false",
	})
	require.NoError(t, err, "failed to restart server with auth disabled")

	tests := []struct {
		name   string
		apiKey string
	}{
		{"without API Key header", ""},
		{"with invalid API Key", "any-key-should-be-ignored"},
		{"with valid API Key", testutil.GetAPIKey()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if tc.apiKey != "" {
				req.Header.Set("X-API-Key", tc.apiKey)
			}

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusCreated, resp.StatusCode,
				"with API_KEY_ENABLED=false, request should succeed regardless of API key, body: %s", string(body))

			var result map[string]any
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)
			assert.Contains(t, result, "validationId", "response should contain validationId")
			assert.Contains(t, result, "decision", "response should contain decision")
		})
	}

	// Restore original config
	err = cleanup()
	require.NoError(t, err, "failed to restore server config")

	// Post-cleanup verification: auth should be re-enabled
	t.Run("post-cleanup auth restored", func(t *testing.T) {
		// Request without API key should now be rejected
		reqNoKey, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
		require.NoError(t, err)
		reqNoKey.Header.Set("Content-Type", "application/json")

		respNoKey, err := testutil.HTTPClient.Do(reqNoKey)
		require.NoError(t, err)
		defer respNoKey.Body.Close()

		require.Equal(t, http.StatusUnauthorized, respNoKey.StatusCode,
			"after cleanup, request without API key should return 401")

		// Request with valid API key should succeed
		reqWithKey, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
		require.NoError(t, err)
		reqWithKey.Header.Set("Content-Type", "application/json")
		reqWithKey.Header.Set("X-API-Key", testutil.GetAPIKey())

		respWithKey, err := testutil.HTTPClient.Do(reqWithKey)
		require.NoError(t, err)
		defer respWithKey.Body.Close()

		require.Equal(t, http.StatusCreated, respWithKey.StatusCode,
			"after cleanup, request with valid API key should return 201")
	})
}

// =============================================================================
// 6.1.13 Rejects API Key with special byte sequences
// =============================================================================

func TestAuth_6_1_13_SpecialByteSequences(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		description string
	}{
		{"null byte", "valid-prefix\x00malicious-suffix", "API Key with null byte injection"},
		{"unicode cyrillic", "апи-ключ-кириллица", "API Key with Cyrillic characters"},
		{"emoji", "api-key-🔑-emoji", "API Key with emoji"},
		{"newline", "first-line\nsecond-line", "API Key with newline character"},
		{"CRLF injection", "first-part\r\nX-Injected-Header: malicious", "API Key with CRLF injection attempt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", bytes.NewReader(validPayload(t)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", tc.apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			// Note: Some HTTP clients/servers may reject at transport level
			if err != nil {
				t.Logf("Transport-level rejection for %s: %v (this is acceptable - defense in depth)", tc.name, err)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Either 401 or transport rejection is acceptable
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"special byte sequence should be rejected, body: %s", string(body))
		})
	}
}

// =============================================================================
// 6.1.14 Validates error messages do not expose internal details
// =============================================================================

func TestAuth_6_1_14_NoInternalDetailsInErrors(t *testing.T) {
	dangerousPatterns := []string{
		"runtime error:",
		"panic:",
		"/internal/",
		"/home/",
		"/var/",
		"stack trace",
		"goroutine",
		"DATABASE_URL",
		"API_KEY",
		".go:",
	}

	validBody := string(validPayload(t))

	tests := []struct {
		name         string
		apiKey       string
		body         string
		expectedCode int
	}{
		{"invalid API Key", "trigger-internal-error-attempt", validBody, http.StatusUnauthorized},
		{"invalid API Key with malformed JSON", "invalid-key", "{malformed json that is not valid}", http.StatusUnauthorized},
		// Very long API keys are rejected at HTTP level (431) before reaching auth middleware
		{"very long API Key (10000 chars)", strings.Repeat("x", 10000), validBody, http.StatusRequestHeaderFieldsTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", tc.apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedCode, resp.StatusCode,
				"should return %d, got %d, body: %s", tc.expectedCode, resp.StatusCode, string(body))

			// Check for dangerous patterns in response
			bodyStr := string(body)
			for _, pattern := range dangerousPatterns {
				assert.NotContains(t, bodyStr, pattern,
					"error response should not contain internal details: %s", pattern)
			}
		})
	}
}

// =============================================================================
// 6.1.15 Error precedence: authentication vs validation errors
// =============================================================================

func TestAuth_6_1_15_AuthErrorPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		description string
	}{
		{
			name:        "invalid auth + invalid JSON",
			contentType: "application/json",
			body:        "{this is not valid json",
			description: "malformed JSON should not matter - auth error first",
		},
		{
			name:        "invalid auth + missing required fields",
			contentType: "application/json",
			body:        "{}",
			description: "missing fields should not matter - auth error first",
		},
		{
			name:        "invalid auth + invalid UUID and values",
			contentType: "application/json",
			body:        `{"requestId":"not-a-valid-uuid","transactionType":"INVALID_TYPE","amount":"-100.00","currency":"INVALID"}`,
			description: "validation errors should not matter - auth error first",
		},
		{
			name:        "invalid auth + wrong Content-Type",
			contentType: "text/plain",
			body:        "Some plain text body",
			description: "wrong content type should not matter - auth error first",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/validations", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("X-API-Key", "invalid-key")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// MUST be 401, NOT 400 or 415
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"auth error (401) must take precedence over validation error (400/415), body: %s", string(body))

			assertAuthError(t, resp, body)
		})
	}
}
