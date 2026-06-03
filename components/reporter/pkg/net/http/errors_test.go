// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/reporter/pkg"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithError_WrappedErrors verifies that WithError correctly identifies
// domain error types even when they are wrapped with fmt.Errorf("%w").
// The current implementation uses a type-switch (err.(type)) which only
// matches exact concrete types and FAILS on wrapped errors. The fix
// requires using errors.As() for each domain error type.
//
// REFACTOR-008: These tests MUST FAIL against the current code because
// the type-switch cannot unwrap errors.
func TestWithError_WrappedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		err                error
		expectedStatusCode int
		expectedBodyCode   string
	}{
		{
			name: "wrapped EntityNotFoundError returns 404",
			err: fmt.Errorf("repository layer: %w", pkg.EntityNotFoundError{
				Code:    "TPL-0011",
				Title:   "Entity Not Found",
				Message: "report not found",
			}),
			expectedStatusCode: http.StatusNotFound,
			expectedBodyCode:   "TPL-0011",
		},
		{
			name: "wrapped EntityConflictError returns 409",
			err: fmt.Errorf("service layer: %w", pkg.EntityConflictError{
				Code:    "TPL-0040",
				Title:   "Duplicate Request In Flight",
				Message: "conflict detected",
			}),
			expectedStatusCode: http.StatusConflict,
			expectedBodyCode:   "TPL-0040",
		},
		{
			name: "wrapped ValidationError returns 400",
			err: fmt.Errorf("handler parse: %w", pkg.ValidationError{
				Code:    "TPL-0001",
				Title:   "Missing required fields",
				Message: "field X is required",
			}),
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyCode:   "TPL-0001",
		},
		{
			name: "wrapped UnprocessableOperationError returns 422",
			err: fmt.Errorf("business rule: %w", pkg.UnprocessableOperationError{
				Code:    "TPL-0099",
				Title:   "Unprocessable",
				Message: "cannot process this operation",
			}),
			expectedStatusCode: http.StatusUnprocessableEntity,
			expectedBodyCode:   "TPL-0099",
		},
		{
			name: "wrapped UnauthorizedError returns 401",
			err: fmt.Errorf("auth middleware: %w", pkg.UnauthorizedError{
				Code:    "TPL-0401",
				Title:   "Unauthorized",
				Message: "missing credentials",
			}),
			expectedStatusCode: http.StatusUnauthorized,
			expectedBodyCode:   "TPL-0401",
		},
		{
			name: "wrapped ForbiddenError returns 403",
			err: fmt.Errorf("authz check: %w", pkg.ForbiddenError{
				Code:    "TPL-0403",
				Title:   "Forbidden",
				Message: "insufficient permissions",
			}),
			expectedStatusCode: http.StatusForbidden,
			expectedBodyCode:   "TPL-0403",
		},
		{
			name: "double-wrapped EntityNotFoundError returns 404",
			err: fmt.Errorf("handler: %w", fmt.Errorf("service: %w", pkg.EntityNotFoundError{
				Code:    "TPL-0011",
				Title:   "Entity Not Found",
				Message: "deep wrapped not found",
			})),
			expectedStatusCode: http.StatusNotFound,
			expectedBodyCode:   "TPL-0011",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatusCode, resp.StatusCode,
				"expected status %d for %s but got %d; body: %s",
				tt.expectedStatusCode, tt.name, resp.StatusCode, string(body))

			assert.Contains(t, string(body), tt.expectedBodyCode,
				"response body should contain error code %s; body: %s",
				tt.expectedBodyCode, string(body))
		})
	}
}

// TestIsBusinessError verifies that IsBusinessError correctly identifies all
// business/domain error types and rejects non-business errors.
func TestIsBusinessError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "EntityNotFoundError is business error",
			err:      pkg.EntityNotFoundError{Code: "E001", Title: "Not Found", Message: "not found"},
			expected: true,
		},
		{
			name:     "EntityConflictError is business error",
			err:      pkg.EntityConflictError{Code: "E002", Title: "Conflict", Message: "conflict"},
			expected: true,
		},
		{
			name:     "ValidationKnownFieldsError is business error",
			err:      pkg.ValidationKnownFieldsError{Code: "E003", Title: "Validation", Message: "bad fields"},
			expected: true,
		},
		{
			name:     "ValidationUnknownFieldsError is business error",
			err:      pkg.ValidationUnknownFieldsError{Code: "E004", Title: "Validation", Message: "unknown fields"},
			expected: true,
		},
		{
			name:     "ValidationError is business error",
			err:      pkg.ValidationError{Code: "E005", Title: "Validation", Message: "invalid"},
			expected: true,
		},
		{
			name:     "UnprocessableOperationError is business error",
			err:      pkg.UnprocessableOperationError{Code: "E006", Title: "Unprocessable", Message: "cannot process"},
			expected: true,
		},
		{
			name:     "UnauthorizedError is business error",
			err:      pkg.UnauthorizedError{Code: "E007", Title: "Unauthorized", Message: "no auth"},
			expected: true,
		},
		{
			name:     "ForbiddenError is business error",
			err:      pkg.ForbiddenError{Code: "E008", Title: "Forbidden", Message: "denied"},
			expected: true,
		},
		{
			name:     "wrapped EntityNotFoundError is business error",
			err:      fmt.Errorf("service: %w", pkg.EntityNotFoundError{Code: "E001", Title: "Not Found", Message: "wrapped"}),
			expected: true,
		},
		{
			name:     "generic error is not business error",
			err:      fmt.Errorf("database connection failed"),
			expected: false,
		},
		{
			name:     "nil error is not business error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsBusinessError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWithError_UnwrappedErrors verifies that the current type-switch still
// works correctly for direct (unwrapped) errors. These tests should PASS
// with both the old and new implementation, acting as regression guards.
func TestWithError_UnwrappedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		err                error
		expectedStatusCode int
	}{
		{
			name: "direct EntityNotFoundError returns 404",
			err: pkg.EntityNotFoundError{
				Code:    "TPL-0011",
				Title:   "Entity Not Found",
				Message: "report not found",
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "direct EntityConflictError returns 409",
			err: pkg.EntityConflictError{
				Code:    "TPL-0040",
				Title:   "Conflict",
				Message: "already exists",
			},
			expectedStatusCode: http.StatusConflict,
		},
		{
			name: "direct ValidationError returns 400",
			err: pkg.ValidationError{
				Code:    "TPL-0001",
				Title:   "Validation Error",
				Message: "invalid input",
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "direct UnprocessableOperationError returns 422",
			err: pkg.UnprocessableOperationError{
				Code:    "TPL-0099",
				Title:   "Unprocessable",
				Message: "cannot process",
			},
			expectedStatusCode: http.StatusUnprocessableEntity,
		},
		{
			name: "direct UnauthorizedError returns 401",
			err: pkg.UnauthorizedError{
				Code:    "TPL-0401",
				Title:   "Unauthorized",
				Message: "no auth",
			},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name: "direct ForbiddenError returns 403",
			err: pkg.ForbiddenError{
				Code:    "TPL-0403",
				Title:   "Forbidden",
				Message: "denied",
			},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "unknown error returns 500",
			err:                fmt.Errorf("unexpected database failure"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatusCode, resp.StatusCode,
				"expected status %d for %s but got %d",
				tt.expectedStatusCode, tt.name, resp.StatusCode)
		})
	}
}
