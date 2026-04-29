// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"
	"net/http/httptest"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectedRouteChain_RunsPostAuthMiddlewareAfterAuth(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	chain := ProtectedRouteChain(
		func(c *fiber.Ctx) error {
			c.Locals("auth_ran", true)
			return c.Next()
		},
		&ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{func(c *fiber.Ctx) error {
				assert.Equal(t, true, c.Locals("auth_ran"))
				c.Locals("post_auth_ran", true)
				return c.Next()
			}},
		},
		func(c *fiber.Ctx) error {
			assert.Equal(t, true, c.Locals("post_auth_ran"))
			return c.SendStatus(fiber.StatusNoContent)
		},
	)

	app.Get("/test", chain...)

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}

func TestMarkTrustedAuthAssertion_SetsTrustedLocalsAndTenantContext(t *testing.T) {
	t.Parallel()

	token := mustUnsignedToken(t, jwt.MapClaims{
		"sub":      "user-123",
		"tenantId": "tenant_123",
	})

	app := fiber.New()
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"userID":   c.Locals("user_id"),
			"tenantID": tmcore.GetTenantIDContext(c.UserContext()),
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestMarkTrustedAuthAssertion_UsesSentinelWhenIdentityClaimMissing(t *testing.T) {
	t.Parallel()

	token := mustUnsignedToken(t, jwt.MapClaims{"tenantId": "tenant_123"})

	app := fiber.New()
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString(c.Locals("user_id").(string))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func mustUnsignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	return signed
}
