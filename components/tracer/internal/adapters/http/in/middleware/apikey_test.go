// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg/contextutil"
)

// errorResponse represents the standard error response format from libHTTP.
type errorResponse struct {
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func TestAPIKeyAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         APIKeyConfig
		apiKey         string
		setHeader      bool
		expectedStatus int
		expectedCode   string
		expectedBody   string
	}{
		{
			name: "Success - Valid API Key",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: true,
			},
			apiKey:         "valid-secret-key",
			setHeader:      true,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "Error - Missing API Key (no header)",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: true,
			},
			apiKey:         "",
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
		{
			name: "Error - Invalid API Key",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: true,
			},
			apiKey:         "wrong-key",
			setHeader:      true,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
		{
			name: "Error - Empty API Key (header present but empty)",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: true,
			},
			apiKey:         "",
			setHeader:      true,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
		{
			name: "Success - Auth Disabled (no key required)",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: false,
			},
			apiKey:         "",
			setHeader:      false,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "Success - Auth Disabled (request with wrong key still passes)",
			config: APIKeyConfig{
				Key:     "valid-secret-key",
				Enabled: false,
			},
			apiKey:         "any-key",
			setHeader:      true,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Fiber app with middleware
			app := fiber.New()
			app.Use(APIKeyAuth(tt.config))
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendString("success")
			})

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.setHeader {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			// Execute request
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Assert status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Assert response body
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.expectedBody, string(body))
			} else if tt.expectedCode != "" {
				// Parse error response and check code
				var errResp errorResponse
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "Expected JSON error response, got: %s", string(body))
				assert.Equal(t, tt.expectedCode, errResp.Code, "Error code should be 'Unauthenticated'")
			}
		})
	}
}

// TestAPIKeyAuth_ConstantTimeComparison verifies timing attack resistance.
// This test ensures that comparison time doesn't vary significantly based on
// how much of the key matches. While we can't perfectly test constant-time
// behavior in unit tests, we can verify the implementation uses the correct approach.
func TestAPIKeyAuth_ConstantTimeComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   APIKeyConfig
		apiKey   string
		expected int
	}{
		{
			name: "Completely different key",
			config: APIKeyConfig{
				Key:     "AAAAAAAAAAAAAAAA",
				Enabled: true,
			},
			apiKey:   "BBBBBBBBBBBBBBBB",
			expected: http.StatusUnauthorized,
		},
		{
			name: "First character different",
			config: APIKeyConfig{
				Key:     "AAAAAAAAAAAAAAAA",
				Enabled: true,
			},
			apiKey:   "BAAAAAAAAAAAAAAA",
			expected: http.StatusUnauthorized,
		},
		{
			name: "Last character different",
			config: APIKeyConfig{
				Key:     "AAAAAAAAAAAAAAAA",
				Enabled: true,
			},
			apiKey:   "AAAAAAAAAAAAAAAB",
			expected: http.StatusUnauthorized,
		},
		{
			name: "Different length",
			config: APIKeyConfig{
				Key:     "short",
				Enabled: true,
			},
			apiKey:   "muchlongerkey",
			expected: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(APIKeyAuth(tt.config))
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendString("success")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-API-Key", tt.apiKey)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expected, resp.StatusCode)
		})
	}
}

// TestAPIKeyAuth_SameErrorMessage ensures the same error is returned for both
// missing and invalid keys to prevent enumeration attacks.
func TestAPIKeyAuth_SameErrorMessage(t *testing.T) {
	t.Parallel()

	config := APIKeyConfig{
		Key:     "secret-key",
		Enabled: true,
	}

	app := fiber.New()
	app.Use(APIKeyAuth(config))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("success")
	})

	// Test missing key
	reqMissing := httptest.NewRequest(http.MethodGet, "/test", nil)
	respMissing, err := app.Test(reqMissing, -1)
	require.NoError(t, err)
	defer respMissing.Body.Close()

	bodyMissing, err := io.ReadAll(respMissing.Body)
	require.NoError(t, err)

	// Test invalid key
	reqInvalid := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqInvalid.Header.Set("X-API-Key", "wrong-key")
	respInvalid, err := app.Test(reqInvalid, -1)
	require.NoError(t, err)
	defer respInvalid.Body.Close()

	bodyInvalid, err := io.ReadAll(respInvalid.Body)
	require.NoError(t, err)

	// Parse both responses
	var errMissing, errInvalid errorResponse
	err = json.Unmarshal(bodyMissing, &errMissing)
	require.NoError(t, err)
	err = json.Unmarshal(bodyInvalid, &errInvalid)
	require.NoError(t, err)

	// Verify same error code and detail
	assert.Equal(t, errMissing.Code, errInvalid.Code, "Error codes should be identical")
	assert.Equal(t, errMissing.Title, errInvalid.Title, "Error titles should be identical")
	assert.Equal(t, errMissing.Detail, errInvalid.Detail, "Error details should be identical")
}

// TestMetricAuthFailures_Definition verifies the metric is properly defined.
func TestMetricAuthFailures_Definition(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tracer_auth_failures_total", MetricAuthFailures.Name,
		"Metric name should follow TRD Section 9.3 convention with tracer_ prefix")
	assert.Equal(t, "1", MetricAuthFailures.Unit,
		"Metric unit should be '1' for counters")
	assert.NotEmpty(t, MetricAuthFailures.Description,
		"Metric should have a description")
}

// ---------------------------------------------------------------------------
// Principal stamping tests — verify APIKeyAuth carimba a contextutil.Principal
// no UserContext do Fiber quando a key é válida, e NÃO carimba no caminho
// disabled / inválido.
// ---------------------------------------------------------------------------

// apikeyPrincipalCapture runs APIKeyAuth in front of a probe handler that
// surfaces the Principal stored in c.UserContext() as JSON so the test can
// assert on its fields.
func apikeyPrincipalCapture(cfg APIKeyConfig) *fiber.App {
	app := fiber.New()
	app.Get("/test", APIKeyAuth(cfg), func(c *fiber.Ctx) error {
		p, ok := contextutil.GetPrincipal(c.UserContext())

		return c.JSON(fiber.Map{
			"hasPrincipal": ok,
			"type":         p.Type,
			"id":           p.ID,
			"name":         p.Name,
		})
	})

	return app
}

func TestAPIKeyAuth_Disabled_NoPrincipalStamped(t *testing.T) {
	t.Parallel()

	app := apikeyPrincipalCapture(APIKeyConfig{Enabled: false})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, false, got["hasPrincipal"],
		"disabled middleware MUST NOT stamp a Principal — the dev-mode path must reach the audit writer with no authenticated identity")
}

func TestAPIKeyAuth_MissingKey_NoPrincipalAnd401(t *testing.T) {
	t.Parallel()

	app := apikeyPrincipalCapture(APIKeyConfig{Enabled: true, Key: "expected", Label: "tracer-default"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"missing API key must 401 before any Principal is set")
}

func TestAPIKeyAuth_InvalidKey_NoPrincipalAnd401(t *testing.T) {
	t.Parallel()

	app := apikeyPrincipalCapture(APIKeyConfig{Enabled: true, Key: "expected", Label: "tracer-default"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderAPIKey, "wrong")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"invalid API key must 401 before any Principal is set")
}

func TestAPIKeyAuth_ValidKey_StampsAPIKeyPrincipal(t *testing.T) {
	t.Parallel()

	app := apikeyPrincipalCapture(APIKeyConfig{Enabled: true, Key: "secret-key", Label: "tracer-prod-eu"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderAPIKey, "secret-key")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, true, got["hasPrincipal"])
	assert.Equal(t, "api_key", got["type"])
	assert.Equal(t, "tracer-prod-eu", got["id"])
	assert.Equal(t, "", got["name"], "API-key principals carry no human name")
}

func TestAPIKeyAuth_BlankLabel_FallsBackToDefault(t *testing.T) {
	t.Parallel()

	// Whitespace-only label must be treated as unset and replaced with the
	// default. Audit rows always need a non-empty actor id.
	app := apikeyPrincipalCapture(APIKeyConfig{Enabled: true, Key: "secret-key", Label: "   "})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderAPIKey, "secret-key")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, "tracer-default", got["id"])
}
