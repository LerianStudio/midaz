// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnauthorized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 401 with custom fields",
			code:    "AUTH_001",
			title:   "Unauthorized",
			message: "Invalid credentials",
		},
		{
			name:    "Success - returns 401 with empty fields",
			code:    "",
			title:   "",
			message: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return Unauthorized(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestForbidden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 403 with custom fields",
			code:    "FORBIDDEN_001",
			title:   "Forbidden",
			message: "Access denied",
		},
		{
			name:    "Success - returns 403 with empty fields",
			code:    "",
			title:   "",
			message: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return Forbidden(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestBadRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     any
		expected string
	}{
		{
			name:     "Success - returns 400 with map body",
			body:     map[string]string{"error": "bad input"},
			expected: `{"error":"bad input"}`,
		},
		{
			name:     "Success - returns 400 with struct body",
			body:     struct{ Message string }{"validation failed"},
			expected: `{"Message":"validation failed"}`,
		},
		{
			name:     "Success - returns 400 with error message envelope",
			body:     errors.New("validation failed"),
			expected: `{"message":"validation failed"}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return BadRequest(c, tt.body)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(body))
		})
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 404 with custom fields",
			code:    "NOT_FOUND_001",
			title:   "Not Found",
			message: "Resource not found",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return NotFound(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 409 with custom fields",
			code:    "CONFLICT_001",
			title:   "Conflict",
			message: "Entity already exists",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return Conflict(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusConflict, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestUnprocessableEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 422 with custom fields",
			code:    "UNPROCESSABLE_001",
			title:   "Unprocessable Entity",
			message: "Cannot process request",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return UnprocessableEntity(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestInternalServerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		title   string
		message string
	}{
		{
			name:    "Success - returns 500 with custom fields",
			code:    "ISE_001",
			title:   "Internal Server Error",
			message: "Something went wrong",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return InternalServerError(c, tt.code, tt.title, tt.message)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]string
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.code, result["code"])
			assert.Equal(t, tt.title, result["title"])
			assert.Equal(t, tt.message, result["message"])
		})
	}
}

func TestJSONResponseError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          pkg.ResponseError
		expectedCode int
	}{
		{
			name: "Success - returns custom status code with ResponseError",
			err: pkg.ResponseError{
				Code:    http.StatusTeapot,
				Title:   "I'm a teapot",
				Message: "Cannot brew coffee",
			},
			expectedCode: http.StatusTeapot,
		},
		{
			name: "Success - returns 502 Bad Gateway",
			err: pkg.ResponseError{
				Code:    http.StatusBadGateway,
				Title:   "Bad Gateway",
				Message: "Upstream error",
			},
			expectedCode: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return JSONResponseError(c, tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result pkg.ResponseError
			require.NoError(t, json.Unmarshal(body, &result))
			assert.Equal(t, tt.err.Code, result.Code)
			assert.Equal(t, tt.err.Title, result.Title)
			assert.Equal(t, tt.err.Message, result.Message)
		})
	}
}
