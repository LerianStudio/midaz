// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePathParametersUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pathParam      string
		expectedStatus int
		expectError    bool
		expectLocals   bool
	}{
		{
			name:           "Success - Valid UUID",
			pathParam:      uuid.New().String(),
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
		{
			name:           "Error - Invalid UUID format",
			pathParam:      "invalid-uuid-format",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - Partial UUID",
			pathParam:      "550e8400-e29b-41d4",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - UUID with invalid characters",
			pathParam:      "550e8400-e29b-41d4-a716-44665544000g",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Success - UUID with uppercase letters",
			pathParam:      "550E8400-E29B-41D4-A716-446655440000",
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			var capturedID uuid.UUID
			var localsSet bool

			app.Get("/test/:id", ParsePathParametersUUID, func(c *fiber.Ctx) error {
				if id, ok := c.Locals(UUIDPathParameter).(uuid.UUID); ok {
					capturedID = id
					localsSet = true
				}
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test/"+tt.pathParam, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectLocals {
				assert.True(t, localsSet, "Expected locals to be set")
				assert.NotEqual(t, uuid.Nil, capturedID, "Expected valid UUID in locals")
			}

			if tt.expectError {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var errorResponse map[string]interface{}
				err = json.Unmarshal(body, &errorResponse)
				require.NoError(t, err)

				assert.Contains(t, errorResponse, "code")
			}
		})
	}
}

func TestParsePathParametersUUID_SpecificUUID(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	expectedUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	var capturedID uuid.UUID

	app.Get("/test/:id", ParsePathParametersUUID, func(c *fiber.Ctx) error {
		capturedID = c.Locals(UUIDPathParameter).(uuid.UUID)
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test/"+expectedUUID.String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, expectedUUID, capturedID)
}

func TestParsePathParametersUUID_WithDifferentRoutes(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	validUUID := uuid.New()

	app.Get("/templates/:id", ParsePathParametersUUID, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"type": "template", "id": c.Locals(UUIDPathParameter)})
	})

	app.Get("/reports/:id", ParsePathParametersUUID, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"type": "report", "id": c.Locals(UUIDPathParameter)})
	})

	tests := []struct {
		name         string
		route        string
		expectedType string
	}{
		{
			name:         "Template route",
			route:        "/templates/" + validUUID.String(),
			expectedType: "template",
		},
		{
			name:         "Report route",
			route:        "/reports/" + validUUID.String(),
			expectedType: "report",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.route, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response map[string]interface{}
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedType, response["type"])
		})
	}
}

func TestUUIDPathParameter_Constant(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "id", UUIDPathParameter)
}

func TestParseStringPathParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pathParam      string
		expectedStatus int
		expectError    bool
		expectLocals   bool
	}{
		{
			name:           "Success - snake_case data source ID",
			pathParam:      "midaz_onboarding",
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
		{
			name:           "Success - short data source ID",
			pathParam:      "pg_ds",
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
		{
			name:           "Success - hyphenated data source ID",
			pathParam:      "A-valid-ID",
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
		{
			name:           "Success - single letter",
			pathParam:      "x",
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectLocals:   true,
		},
		{
			name:           "Error - starts with number",
			pathParam:      "123abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - starts with underscore",
			pathParam:      "_invalid",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - special characters (semicolon)",
			pathParam:      "id;DROP",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - contains dots",
			pathParam:      "some.dotted.id",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
		{
			name:           "Error - URL encoded path traversal",
			pathParam:      "..%2Fetc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			expectLocals:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			const paramName = "dataSourceId"

			var capturedID string
			var localsSet bool

			app.Get("/data-sources/:dataSourceId", ParseStringPathParam(paramName), func(c *fiber.Ctx) error {
				if id, ok := c.Locals(paramName).(string); ok {
					capturedID = id
					localsSet = true
				}
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/data-sources/"+tt.pathParam, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectLocals {
				assert.True(t, localsSet, "Expected locals to be set")
				assert.Equal(t, tt.pathParam, capturedID, "Expected path param stored in locals")
			}

			if tt.expectError {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var errorResponse map[string]interface{}
				err = json.Unmarshal(body, &errorResponse)
				require.NoError(t, err)

				assert.Contains(t, errorResponse, "code")
			}
		})
	}
}

func TestParseStringPathParam_SpecificValue(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "dataSourceId"
	const expectedID = "midaz_onboarding"

	var capturedID string

	app.Get("/data-sources/:dataSourceId", ParseStringPathParam(paramName), func(c *fiber.Ctx) error {
		capturedID = c.Locals(paramName).(string)
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/data-sources/"+expectedID, nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, expectedID, capturedID)
}

func TestParseStringPathParam_MaxLength(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "dataSourceId"

	app.Get("/data-sources/:dataSourceId", ParseStringPathParam(paramName), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// 128 chars: 1 letter + 127 alphanumeric = exactly at limit
	validLong := "a" + strings.Repeat("b", 127)
	req := httptest.NewRequest(http.MethodGet, "/data-sources/"+validLong, nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 129 chars: exceeds limit
	tooLong := "a" + strings.Repeat("b", 128)
	req = httptest.NewRequest(http.MethodGet, "/data-sources/"+tooLong, nil)
	resp, err = app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// SecurityHeaders tests
// ---------------------------------------------------------------------------

func TestSecurityHeaders_Middleware(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(SecurityHeaders())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "0", resp.Header.Get("X-XSS-Protection"))
	assert.Equal(t, "max-age=31536000; includeSubDomains", resp.Header.Get("Strict-Transport-Security"))
}

// ---------------------------------------------------------------------------
// RecoverMiddleware tests
// ---------------------------------------------------------------------------

func TestRecoverMiddleware(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(RecoverMiddleware())
	app.Get("/panic", func(_ *fiber.Ctx) error {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	// The recover middleware should catch the panic and return 500
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// ParseUUIDPathParam with custom param name tests
// ---------------------------------------------------------------------------

func TestParseUUIDPathParam_CustomParamName(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "reportId"
	validUUID := uuid.New()
	var capturedID uuid.UUID

	app.Get("/reports/:reportId", ParseUUIDPathParam(paramName), func(c *fiber.Ctx) error {
		capturedID = c.Locals(paramName).(uuid.UUID)
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/reports/"+validUUID.String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, validUUID, capturedID)
}

// ---------------------------------------------------------------------------
// Edge-case tests for additional coverage
// ---------------------------------------------------------------------------

func TestParseUUIDPathParam_InvalidUUID_DifferentParamName(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "templateId"

	app.Get("/templates/:templateId", ParseUUIDPathParam(paramName), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/templates/not-a-uuid", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errorResponse map[string]interface{}
	err = json.Unmarshal(body, &errorResponse)
	require.NoError(t, err)

	assert.Contains(t, errorResponse, "code")
}

func TestParseStringPathParam_UnicodeCharacters(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "dataSourceId"

	app.Get("/data-sources/:dataSourceId", ParseStringPathParam(paramName), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// Unicode characters should fail the allowlist regex
	req := httptest.NewRequest(http.MethodGet, "/data-sources/caf\u00e9", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errorResponse map[string]interface{}
	err = json.Unmarshal(body, &errorResponse)
	require.NoError(t, err)

	assert.Contains(t, errorResponse, "code")
}

func TestParseStringPathParam_SlashCharacter(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	const paramName = "dataSourceId"

	app.Get("/data-sources/:dataSourceId", ParseStringPathParam(paramName), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// Path traversal attempt should fail the regex
	req := httptest.NewRequest(http.MethodGet, "/data-sources/a%2Fb", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	// Fiber may interpret %2F as a path separator and return 404, or it may
	// pass "a/b" to the handler and our regex rejects it with 400.
	// Either way, it should NOT be 200 OK.
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}
