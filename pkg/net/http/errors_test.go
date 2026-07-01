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

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithError_TypedArms_WrappedAndUnwrapped asserts that every typed error arm
// resolves to the correct HTTP status and propagates the error's own Code in the
// envelope, both when passed directly and when wrapped via fmt.Errorf("...: %w").
func TestWithError_TypedArms_WrappedAndUnwrapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		expectedCode int
		expectedBody string
	}{
		{
			name:         "EntityNotFoundError -> 404",
			err:          pkg.EntityNotFoundError{Code: "0007", Title: "Not Found", Message: "missing"},
			expectedCode: http.StatusNotFound,
			expectedBody: `"code":"0007"`,
		},
		{
			name:         "EntityConflictError -> 409",
			err:          pkg.EntityConflictError{Code: "0001", Title: "Conflict", Message: "dup"},
			expectedCode: http.StatusConflict,
			expectedBody: `"code":"0001"`,
		},
		{
			name:         "ValidationError -> 400",
			err:          pkg.ValidationError{Code: "0099", Title: "Validation", Message: "bad"},
			expectedCode: http.StatusBadRequest,
			expectedBody: `"code":"0099"`,
		},
		{
			name:         "UnprocessableOperationError -> 422",
			err:          pkg.UnprocessableOperationError{Code: "0018", Title: "Unprocessable", Message: "nope"},
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: `"code":"0018"`,
		},
		{
			name:         "UnauthorizedError -> 401",
			err:          pkg.UnauthorizedError{Code: "0098", Title: "Unauthorized", Message: "no auth"},
			expectedCode: http.StatusUnauthorized,
			expectedBody: `"code":"0098"`,
		},
		{
			name:         "ForbiddenError -> 403",
			err:          pkg.ForbiddenError{Code: "0097", Title: "Forbidden", Message: "denied"},
			expectedCode: http.StatusForbidden,
			expectedBody: `"code":"0097"`,
		},
		{
			name:         "ValidationKnownFieldsError -> 400",
			err:          pkg.ValidationKnownFieldsError{Code: "0096", Title: "Known Fields", Message: "field bad"},
			expectedCode: http.StatusBadRequest,
			expectedBody: `"code":"0096"`,
		},
		{
			name:         "ValidationUnknownFieldsError -> 400",
			err:          pkg.ValidationUnknownFieldsError{Code: "0095", Title: "Unknown Fields", Message: "unknown field"},
			expectedCode: http.StatusBadRequest,
			expectedBody: `"code":"0095"`,
		},
		{
			name:         "FailedPreconditionError -> 500 with own code",
			err:          pkg.FailedPreconditionError{Code: "0094", Title: "Precondition", Message: "precondition failed"},
			expectedCode: http.StatusInternalServerError,
			expectedBody: `"code":"0094"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, mode := range []struct {
				label string
				wrap  bool
			}{
				{label: "unwrapped", wrap: false},
				{label: "wrapped", wrap: true},
			} {
				t.Run(mode.label, func(t *testing.T) {
					t.Parallel()

					err := tt.err
					if mode.wrap {
						err = fmt.Errorf("context: %w", tt.err)
					}

					app := fiber.New()
					app.Get("/test", func(c *fiber.Ctx) error {
						return WithError(c, err)
					})

					req := httptest.NewRequest(http.MethodGet, "/test", nil)
					resp, rerr := app.Test(req)
					require.NoError(t, rerr)
					defer resp.Body.Close()

					assert.Equal(t, tt.expectedCode, resp.StatusCode)

					body, _ := io.ReadAll(resp.Body)
					assert.Contains(t, string(body), tt.expectedBody)
				})
			}
		})
	}
}

// TestWithError_DeclarationOrderWins locks the order-dependence contract of
// WithError: the first matching arm in declaration order wins, and because the
// platform wrapper types (EntityNotFoundError, ValidationError, EntityConflictError)
// declare Unwrap, errors.As walks the chain. When a platform error is nested
// inside a sibling wrapper class, the OUTERMOST class must drive the returned
// status. This is a dormant hazard (production returns platform errors unwrapped),
// so this test exists to catch a future change that silently reorders the arms or
// starts nesting platform errors.
func TestWithError_DeclarationOrderWins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		expectedCode int
		expectedBody string
	}{
		{
			// Validation(400) is declared before Conflict's sibling Unprocessable(422):
			// outer ValidationError wins even though it wraps a 422 class.
			name: "ValidationError wrapping UnprocessableOperationError -> 400 (outer wins)",
			err: pkg.ValidationError{
				Code:    "0099",
				Title:   "Outer Validation",
				Message: "outer",
				Err:     pkg.UnprocessableOperationError{Code: "0018", Title: "Inner", Message: "inner"},
			},
			expectedCode: http.StatusBadRequest,
			expectedBody: `"code":"0099"`,
		},
		{
			// EntityNotFound(404) is declared first of all arms: it wins over a wrapped
			// ValidationError(400).
			name: "EntityNotFoundError wrapping ValidationError -> 404 (outer wins)",
			err: pkg.EntityNotFoundError{
				Code:    "0007",
				Title:   "Outer NotFound",
				Message: "outer",
				Err:     pkg.ValidationError{Code: "0099", Title: "Inner", Message: "inner"},
			},
			expectedCode: http.StatusNotFound,
			expectedBody: `"code":"0007"`,
		},
		{
			// EntityConflict(409) is declared before Validation(400): outer conflict wins.
			name: "EntityConflictError wrapping ValidationError -> 409 (outer wins)",
			err: pkg.EntityConflictError{
				Code:    "0001",
				Title:   "Outer Conflict",
				Message: "outer",
				Err:     pkg.ValidationError{Code: "0099", Title: "Inner", Message: "inner"},
			},
			expectedCode: http.StatusConflict,
			expectedBody: `"code":"0001"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestWithError_EntityConflictError_Returns409(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		expectedCode int
		expectedBody string
	}{
		{
			name: "alias unavailability returns 409",
			err: pkg.EntityConflictError{
				EntityType: "Account",
				Code:       constant.ErrAliasUnavailability.Error(),
				Title:      "Alias Unavailability Error",
				Message:    "The alias @test is already in use. Please choose a different alias and try again.",
			},
			expectedCode: http.StatusConflict,
			expectedBody: `"code":"0020"`,
		},
		{
			name: "duplicate ledger returns 409",
			err: pkg.EntityConflictError{
				EntityType: "Ledger",
				Code:       constant.ErrDuplicateLedger.Error(),
				Title:      "Duplicate Ledger Error",
				Message:    "A ledger with the name Test already exists.",
			},
			expectedCode: http.StatusConflict,
			expectedBody: `"code":"0001"`,
		},
		{
			name: "idempotency key conflict returns 409",
			err: pkg.EntityConflictError{
				EntityType: "Transaction",
				Code:       constant.ErrIdempotencyKey.Error(),
				Title:      "Duplicate Idempotency Key",
				Message:    "The idempotency key abc123 is already in use.",
			},
			expectedCode: http.StatusConflict,
			expectedBody: `"code":"0084"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestWithError_EntityNotFoundError_Returns404(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, pkg.EntityNotFoundError{
			EntityType: "Account",
			Code:       constant.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "Account not found.",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"code":"0007"`)
}

func TestWithError_ValidationError_Returns400(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, pkg.ValidationError{
			Code:    "0099",
			Title:   "Validation Error",
			Message: "Invalid input data",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"code":"0099"`)
}

func TestWithError_UnprocessableOperationError_Returns422(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, pkg.UnprocessableOperationError{
			Code:    constant.ErrInsufficientFunds.Error(),
			Title:   "Insufficient Funds",
			Message: "Account has insufficient funds.",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"code":"0018"`)
}

func TestWithError_UnauthorizedError_Returns401(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, pkg.UnauthorizedError{
			Code:    "0098",
			Title:   "Unauthorized",
			Message: "Invalid token",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"code":"0098"`)
}

func TestWithError_ForbiddenError_Returns403(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, pkg.ForbiddenError{
			Code:    "0097",
			Title:   "Forbidden",
			Message: "Access denied",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"code":"0097"`)
}
