// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg"
)

func setupTestApp(handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/test", handler)
	return app
}

func TestUnauthorized(t *testing.T) {
	tests := []struct {
		name            string
		code            string
		title           string
		message         string
		expectedStatus  int
		expectedCode    string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name:            "Success - returns 401 with custom message",
			code:            "AUTH001",
			title:           "Unauthorized",
			message:         "Invalid credentials",
			expectedStatus:  http.StatusUnauthorized,
			expectedCode:    "AUTH001",
			expectedTitle:   "Unauthorized",
			expectedMessage: "Invalid credentials",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := setupTestApp(func(c *fiber.Ctx) error {
				return Unauthorized(c, tc.code, tc.title, tc.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedCode, result["code"])
			assert.Equal(t, tc.expectedTitle, result["title"])
			assert.Equal(t, tc.expectedMessage, result["message"])
		})
	}
}

func TestForbidden(t *testing.T) {
	t.Run("Success - returns 403 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return Forbidden(c, "FORBID001", "Forbidden", "Access denied")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "FORBID001", result["code"])
		assert.Equal(t, "Forbidden", result["title"])
		assert.Equal(t, "Access denied", result["message"])
	})
}

func TestBadRequest(t *testing.T) {
	t.Run("Success - returns 400 with custom body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return BadRequest(c, fiber.Map{"error": "invalid input"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "invalid input", result["error"])
	})
}

func TestBadRequestWithMessage(t *testing.T) {
	tests := []struct {
		name            string
		code            string
		title           string
		message         string
		expectedStatus  int
		expectedCode    string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name:            "returns 400 with structured error",
			code:            "TRC-1001",
			title:           "Validation Error",
			message:         "Field 'name' is required",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-1001",
			expectedTitle:   "Validation Error",
			expectedMessage: "Field 'name' is required",
		},
		{
			name:            "returns 400 with empty values",
			code:            "",
			title:           "",
			message:         "",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "",
			expectedTitle:   "",
			expectedMessage: "",
		},
		{
			name:            "message with special HTML characters",
			code:            "TRC-1001",
			title:           "Validation Error",
			message:         "<script>alert('xss')</script>",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-1001",
			expectedTitle:   "Validation Error",
			expectedMessage: "<script>alert('xss')</script>",
		},
		{
			name:            "message with Unicode/internationalized characters",
			code:            "TRC-1001",
			title:           "Validation Error",
			message:         "Erro: transação inválida! 中文 العربية",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-1001",
			expectedTitle:   "Validation Error",
			expectedMessage: "Erro: transação inválida! 中文 العربية",
		},
		{
			name:            "very long message to test size handling",
			code:            "TRC-1001",
			title:           "Validation Error",
			message:         strings.Repeat("a", 1000),
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-1001",
			expectedTitle:   "Validation Error",
			expectedMessage: strings.Repeat("a", 1000),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := setupTestApp(func(c *fiber.Ctx) error {
				return BadRequestWithMessage(c, tc.code, tc.title, tc.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedCode, result["code"])
			assert.Equal(t, tc.expectedTitle, result["title"])
			assert.Equal(t, tc.expectedMessage, result["message"])
		})
	}
}

func TestCreated(t *testing.T) {
	t.Run("Success - returns 201 with body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return Created(c, fiber.Map{"id": "123"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "123", result["id"])
	})
}

func TestOK(t *testing.T) {
	t.Run("Success - returns 200 with body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return OK(c, fiber.Map{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ok", result["status"])
	})
}

func TestNoContent(t *testing.T) {
	t.Run("Success - returns 204 without body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return NoContent(c)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

func TestAccepted(t *testing.T) {
	t.Run("Success - returns 202 with body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return Accepted(c, fiber.Map{"status": "processing"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "processing", result["status"])
	})
}

func TestPartialContent(t *testing.T) {
	t.Run("Success - returns 206 with body", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return PartialContent(c, fiber.Map{"data": "partial"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusPartialContent, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "partial", result["data"])
	})
}

func TestRangeNotSatisfiable(t *testing.T) {
	t.Run("Success - returns 416", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return RangeNotSatisfiable(c)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode)
	})
}

func TestNotFound(t *testing.T) {
	t.Run("Success - returns 404 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return NotFound(c, "NF001", "Not Found", "Resource not found")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "NF001", result["code"])
		assert.Equal(t, "Not Found", result["title"])
		assert.Equal(t, "Resource not found", result["message"])
	})
}

func TestConflict(t *testing.T) {
	t.Run("Success - returns 409 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return Conflict(c, "CONF001", "Conflict", "Resource already exists")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "CONF001", result["code"])
		assert.Equal(t, "Conflict", result["title"])
		assert.Equal(t, "Resource already exists", result["message"])
	})
}

func TestNotImplemented(t *testing.T) {
	t.Run("Success - returns 501 with message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return NotImplemented(c, "Feature not implemented")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, float64(http.StatusNotImplemented), result["code"])
		assert.Equal(t, "Not Implemented", result["title"])
		assert.Equal(t, "Feature not implemented", result["message"])
	})
}

func TestUnprocessableEntity(t *testing.T) {
	t.Run("Success - returns 422 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return UnprocessableEntity(c, "UE001", "Unprocessable", "Invalid entity")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "UE001", result["code"])
		assert.Equal(t, "Unprocessable", result["title"])
		assert.Equal(t, "Invalid entity", result["message"])
	})
}

func TestInternalServerError(t *testing.T) {
	t.Run("Success - returns 500 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return InternalServerError(c, "ISE001", "Internal Error", "Something went wrong")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ISE001", result["code"])
		assert.Equal(t, "Internal Error", result["title"])
		assert.Equal(t, "Something went wrong", result["message"])
	})
}

func TestGatewayTimeout(t *testing.T) {
	t.Run("Success - returns 504 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return GatewayTimeout(c, "TRC-0229", "Gateway Timeout", "validation timeout")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "TRC-0229", result["code"])
		assert.Equal(t, "Gateway Timeout", result["title"])
		assert.Equal(t, "validation timeout", result["message"])
	})
}

func TestServiceUnavailable(t *testing.T) {
	t.Run("Success - returns 503 with custom message", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return ServiceUnavailable(c, "TRC-0012", "Service Unavailable", "service temporarily unavailable")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "TRC-0012", result["code"])
		assert.Equal(t, "Service Unavailable", result["title"])
		assert.Equal(t, "service temporarily unavailable", result["message"])
	})
}

func TestJSONResponseError(t *testing.T) {
	t.Run("Success - returns custom status with error struct", func(t *testing.T) {
		app := setupTestApp(func(c *fiber.Ctx) error {
			return JSONResponseError(c, pkg.ResponseError{
				Code:    http.StatusBadRequest,
				Title:   "Bad Request",
				Message: "Invalid input",
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestJSONResponse(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           any
		expectedStatus int
	}{
		{
			name:           "Success - returns 200 with body",
			status:         http.StatusOK,
			body:           fiber.Map{"data": "test"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - returns 201 with body",
			status:         http.StatusCreated,
			body:           fiber.Map{"id": "123"},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Success - returns 500 with body",
			status:         http.StatusInternalServerError,
			body:           fiber.Map{"error": "internal"},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := setupTestApp(func(c *fiber.Ctx) error {
				return JSONResponse(c, tc.status, tc.body)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}
