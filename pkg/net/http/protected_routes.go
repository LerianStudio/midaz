// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"sync"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/baggage"
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
	var warnNonUUIDTenantOnce sync.Once

	return func(c fiber.Ctx) error {
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
			ctx := withTenantIDBaggage(tmcore.ContextWithTenantID(c.Context(), tenantID), tenantID)

			// The telemetry middleware labels per-tenant metrics only from an
			// identity attested here; a client-supplied header or baggage member
			// is never promoted to that trust level. tmcore.IsValidTenantID
			// admits any slug, so the UUID parse is what decides whether the
			// claim can carry the attestation.
			if parsed, parseErr := uuid.Parse(tenantID); parseErr == nil {
				ctx = libObservability.ContextWithAuthenticatedTenant(ctx, parsed, firstNonEmptyStringClaim(claims, "tenantSlug"))
			} else {
				// TODO(#2450): a non-UUID tenantId claim carries no UUID
				// anywhere else in the token, so the attestation cannot be built
				// and per-tenant metrics stay unlabelled for that tenant. Tenants
				// provisioned in the WorkOS window (2026-02-13 to 2026-04-01) got
				// the WorkOS org id as their Casdoor org name and were never
				// backfilled. Decide with the team whether to backfill
				// tokenAttributes.tenantId or to widen the attestation to a
				// validated string.
				warnNonUUIDTenantOnce.Do(func() {
					libObservability.NewLoggerFromContext(ctx).Log(ctx, libLog.LevelWarn,
						"Tenant claim is not a UUID; per-tenant metrics will not be labelled for it")
				})
			}

			c.SetContext(ctx)
		}

		return c.Next()
	}
}

// withTenantIDBaggage seeds tenant.id into the standard OTel baggage, which
// lib-observability's span processor copies onto every application span it
// starts afterwards. lib-commons' tenant middleware writes the same member, but
// it is not on every chain — metadata-index carries the auth assertion without
// it — so seeding here is what makes the span attribute a property of being
// authenticated rather than of resolving a tenant database.
func withTenantIDBaggage(ctx context.Context, tenantID string) context.Context {
	member, err := baggage.NewMember(obsconst.AttrKeyTenantID, tenantID)
	if err != nil {
		return ctx
	}

	bag, err := baggage.FromContext(ctx).SetMember(member)
	if err != nil {
		return ctx
	}

	return baggage.ContextWithBaggage(ctx, bag)
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
