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

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg"
)

func TestWithError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectedStatus  int
		expectedCode    string
		expectedTitle   string
		expectedMessage string
		expectedFields  map[string]any
	}{
		{
			name: "EntityNotFoundError maps to 404",
			err: &pkg.EntityNotFoundError{
				Code:    "TRC-0030",
				Title:   "Not Found",
				Message: "Rule not found",
			},
			expectedStatus:  http.StatusNotFound,
			expectedCode:    "TRC-0030",
			expectedTitle:   "Not Found",
			expectedMessage: "Rule not found",
		},
		{
			name: "EntityConflictError maps to 409",
			err: &pkg.EntityConflictError{
				Code:    "TRC-0031",
				Title:   "Conflict",
				Message: "Rule already exists",
			},
			expectedStatus:  http.StatusConflict,
			expectedCode:    "TRC-0031",
			expectedTitle:   "Conflict",
			expectedMessage: "Rule already exists",
		},
		{
			name: "ValidationError maps to 400",
			err: &pkg.ValidationError{
				Code:    "TRC-0001",
				Title:   "Validation Error",
				Message: "Invalid input",
			},
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-0001",
			expectedTitle:   "Validation Error",
			expectedMessage: "Invalid input",
		},
		{
			name: "UnprocessableOperationError maps to 422",
			err: &pkg.UnprocessableOperationError{
				Code:    "TRC-0040",
				Title:   "Unprocessable",
				Message: "Invalid state transition",
			},
			expectedStatus:  http.StatusUnprocessableEntity,
			expectedCode:    "TRC-0040",
			expectedTitle:   "Unprocessable",
			expectedMessage: "Invalid state transition",
		},
		{
			name: "UnauthorizedError maps to 401",
			err: &pkg.UnauthorizedError{
				Code:    "TRC-0002",
				Title:   "Unauthorized",
				Message: "API key required",
			},
			expectedStatus:  http.StatusUnauthorized,
			expectedCode:    "TRC-0002",
			expectedTitle:   "Unauthorized",
			expectedMessage: "API key required",
		},
		{
			name: "ForbiddenError maps to 403",
			err: &pkg.ForbiddenError{
				Code:    "TRC-0003",
				Title:   "Forbidden",
				Message: "Access denied",
			},
			expectedStatus:  http.StatusForbidden,
			expectedCode:    "TRC-0003",
			expectedTitle:   "Forbidden",
			expectedMessage: "Access denied",
		},
		{
			name: "ValidationKnownFieldsError maps to 400 with fields",
			err: &pkg.ValidationKnownFieldsError{
				Code:    "TRC-0001",
				Title:   "Validation Error",
				Message: "Field validation failed",
				Fields:  map[string]string{"name": "required"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-0001",
			expectedTitle:   "Validation Error",
			expectedMessage: "Field validation failed",
			expectedFields:  map[string]any{"name": "required"},
		},
		{
			name: "ValidationUnknownFieldsError maps to 400 with fields",
			err: &pkg.ValidationUnknownFieldsError{
				Code:    "TRC-0001",
				Title:   "Validation Error",
				Message: "Unknown fields",
				Fields:  pkg.UnknownFields{"unknown_field": "unexpected"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "TRC-0001",
			expectedTitle:   "Validation Error",
			expectedMessage: "Unknown fields",
			expectedFields:  map[string]any{"unknown_field": "unexpected"},
		},
		{
			name: "ResponseError uses provided status",
			err: &pkg.ResponseError{
				Code:    http.StatusTeapot,
				Title:   "I'm a teapot",
				Message: "Short and stout",
			},
			expectedStatus:  http.StatusTeapot,
			expectedTitle:   "I'm a teapot",
			expectedMessage: "Short and stout",
		},
		{
			name:           "Generic error maps to 500",
			err:            errors.New("unexpected error"),
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Nil error maps to 500 internal server error",
			err:            nil,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tc.err)
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

			if tc.expectedCode != "" {
				assert.Equal(t, tc.expectedCode, result["code"])
			}

			if tc.expectedTitle != "" {
				assert.Equal(t, tc.expectedTitle, result["title"])
			}

			if tc.expectedMessage != "" {
				assert.Equal(t, tc.expectedMessage, result["message"])
			}

			if tc.expectedFields != nil {
				fields, ok := result["fields"].(map[string]any)
				require.True(t, ok, "fields should be a map")
				assert.Equal(t, tc.expectedFields, fields)
			}
		})
	}
}
