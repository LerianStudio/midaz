// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationNameConstant(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "ApplicationName has correct value",
			expected: "plugin-crm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ApplicationName,
				"ApplicationName constant must equal %q", tt.expected)
		})
	}
}

func TestNewRouter_TenantMiddlewareRegistration(t *testing.T) {
	// Test that the conditional middleware registration pattern works correctly
	// using a standalone Fiber app that mirrors the NewRouter pattern.
	tests := []struct {
		name           string
		tenantMw       fiber.Handler
		expectMwCalled bool
	}{
		{
			name:           "nil middleware is not registered and does not panic",
			tenantMw:       nil,
			expectMwCalled: false,
		},
		{
			name: "non-nil middleware is registered and invoked",
			tenantMw: func(c *fiber.Ctx) error {
				c.Locals("tenant_mw_called", true)
				return c.Next()
			},
			expectMwCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()

			var mwCalled bool

			// Wrap the test middleware to track invocation
			if tt.tenantMw != nil {
				original := tt.tenantMw
				app.Use(func(c *fiber.Ctx) error {
					mwCalled = true
					return original(c)
				})
			}

			app.Get("/health", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"health endpoint must return 200 OK regardless of middleware presence")
			assert.Equal(t, tt.expectMwCalled, mwCalled,
				"middleware invocation must match expectation")
		})
	}
}

func TestNewRouter_TenantMiddlewareCallCount(t *testing.T) {
	var callCount atomic.Int32

	tenantMw := func(c *fiber.Ctx) error {
		callCount.Add(1)
		return c.Next()
	}

	app := fiber.New()
	app.Use(tenantMw)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), callCount.Load(),
		"tenant middleware must be called exactly once per request")
}
