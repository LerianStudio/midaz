// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
