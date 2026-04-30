// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
)

const trustedUpstreamAuthSentinel = "trusted-upstream-auth"

// ProtectedRouteOptions configures extra handlers that must run after auth
// succeeds but before the business handler executes.
type ProtectedRouteOptions struct {
	PostAuthMiddlewares []fiber.Handler
}

// ProtectedRouteChain builds a route chain where auth executes first, then any
// post-auth middlewares, and finally the business handlers.
func ProtectedRouteChain(authHandler fiber.Handler, options *ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	chain := make([]fiber.Handler, 0, 1+len(handlers))
	chain = append(chain, authHandler)

	if options != nil && len(options.PostAuthMiddlewares) > 0 {
		chain = append(chain, options.PostAuthMiddlewares...)
	}

	chain = append(chain, handlers...)

	return chain
}

// MarkTrustedAuthAssertion records a server-side auth assertion after an auth
// middleware has already succeeded. This enables downstream tenant middleware
// to safely use the ParseUnverified path only after trusted auth has run.
func MarkTrustedAuthAssertion() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if existingUserID, ok := c.Locals("user_id").(string); ok && existingUserID != "" {
			return c.Next()
		}

		accessToken := libHTTP.ExtractTokenFromHeader(c)
		if accessToken == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
		}

		token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
		}

		userID := firstNonEmptyStringClaim(claims, "sub", "owner", "user_id", "userId")
		if userID == "" {
			userID = trustedUpstreamAuthSentinel
		}

		c.Locals("user_id", userID)

		if tenantID := firstNonEmptyStringClaim(claims, "tenantId"); tenantID != "" && tmcore.IsValidTenantID(tenantID) {
			c.SetUserContext(tmcore.ContextWithTenantID(c.UserContext(), tenantID))
		}

		return c.Next()
	}
}

func firstNonEmptyStringClaim(claims jwt.MapClaims, keys ...string) string {
	for _, key := range keys {
		value, ok := claims[key].(string)
		if ok && value != "" {
			return value
		}
	}

	return ""
}
