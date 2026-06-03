// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v3/components/tracer/pkg/net/http"
)

// CodeUnauthorizedMissingSub is the response code returned when a Bearer token
// parses but lacks the required `sub` claim. Per OIDC Core 1.0 §2 and RFC 9068
// §2.2 `sub` is REQUIRED on ID Tokens and JWT access tokens. Without `sub` the
// audit writer cannot attribute the action to a principal — failing loud here
// is preferable to silently falling back to a generic system actor, which was
// the structural failure mode the Taura audit flagged.
//
// Mirrors constant.ErrUnauthorizedMissingSub on the Go side so support /
// clients can pattern-match the TRC-XXXX taxonomy in error responses.
const CodeUnauthorizedMissingSub = "TRC-0350"

// AuthGuardConfig holds all configuration for the auth guard.
type AuthGuardConfig struct {
	APIKey        string
	APIKeyEnabled bool
	// APIKeyLabel is the identifier recorded as the audit actor when a request
	// authenticates via the API key. Bootstrap defaults this to
	// "tracer-default" when API_KEY_LABEL is unset.
	APIKeyLabel       string
	PluginAuthEnabled bool
	AppName           string
}

// AuthGuard manages authentication middleware based on configuration flags.
//
// Auth priority: Plugin Auth > API Key.
//   - If PluginAuthEnabled, endpoints use plugin auth
//   - Otherwise, endpoints fall back to API key auth
type AuthGuard struct {
	apiKeyAuth fiber.Handler
	authClient *authMiddleware.AuthClient
	cfg        AuthGuardConfig
}

// NewAuthGuard creates a new AuthGuard with the given configuration.
// Returns nil if authClient is nil when PluginAuthEnabled is true, since
// Protect() would dereference it to call Authorize(). Callers must check
// for nil return and handle accordingly.
func NewAuthGuard(cfg AuthGuardConfig, authClient *authMiddleware.AuthClient) *AuthGuard {
	if cfg.PluginAuthEnabled && authClient == nil {
		return nil
	}

	return &AuthGuard{
		apiKeyAuth: APIKeyAuth(APIKeyConfig{
			Key:     cfg.APIKey,
			Enabled: cfg.APIKeyEnabled,
			Label:   cfg.APIKeyLabel,
		}),
		authClient: authClient,
		cfg:        cfg,
	}
}

// Protect returns auth middleware with plugin auth priority.
// Returns pluginAuth if enabled, otherwise apiKeyAuth.
//
// When plugin auth is enabled, JWT-claims extraction runs BEFORE lib-auth's
// Authorize. Rationale: lib-auth.Authorize uses ParseUnverified internally on
// the same token, so attempting to extract claims for a malformed token is
// equally "safe" (no signature trust is ever placed on extraction). Running
// before Authorize lets us populate the Principal that downstream handlers
// share — if Authorize then rejects the request, the Principal is simply
// never observed.
//
// Extraction is STRICT about the `sub` claim: a token that parses but lacks
// `sub` is rejected with 401 BEFORE reaching lib-auth, failing loud rather
// than letting the audit writer silently fall back to a generic system actor.
func (g *AuthGuard) Protect(resource, method string) fiber.Handler {
	if g.cfg.PluginAuthEnabled {
		authorize := g.authClient.Authorize(g.cfg.AppName, resource, method)

		return func(c *fiber.Ctx) error {
			rejected, err := extractPrincipalFromBearer(c)
			if rejected {
				return err
			}

			return authorize(c)
		}
	}

	return g.apiKeyAuth
}

// With returns the appropriate auth middleware for a route.
// When forceAPIKeyAuth is true, returns API key auth directly (bypassing plugin auth).
// Otherwise, returns Protect (plugin auth if enabled, else API key).
//
//	api.Post("/rules", guard.With("rules", "post", false), handler)
//	api.Post("/validations", guard.With("validations", "post", cfg.APIKeyOnlyValidation), handler)
func (g *AuthGuard) With(resource, method string, forceAPIKeyAuth bool) fiber.Handler {
	if forceAPIKeyAuth {
		return g.apiKeyAuth
	}

	return g.Protect(resource, method)
}

// extractPrincipalFromBearer parses the Authorization: Bearer <token> header
// and stamps a contextutil.Principal onto the Fiber UserContext.
//
// Return values:
//   - rejected: true when the middleware has WRITTEN a 401 response and the
//     caller MUST NOT call any subsequent handler (lib-auth's Authorize or
//     c.Next()). Used as a short-circuit signal because pkgHTTP.Unauthorized
//     itself returns nil on success.
//   - err: the value returned by pkgHTTP.Unauthorized (typically nil); always
//     propagated as-is to Fiber.
//
// Behavior:
//
//   - No Authorization header or non-Bearer scheme → returns (false, nil).
//     lib-auth's Authorize will subsequently return 401 "Missing Token".
//     No behavior change for unauthenticated requests.
//
//   - Bearer header present but token unparseable (corrupt structure) →
//     returns (false, nil). lib-auth will reject the token downstream with a
//     more specific error from the Access Manager.
//
//   - Token parses but `sub` claim is missing/empty → writes a 401 with code
//     UNAUTHORIZED_MISSING_SUB and returns (true, …). Per OIDC Core 1.0 §2
//     and RFC 9068 §2.2 `sub` is REQUIRED. Without `sub` the action cannot be
//     attributed and would default to the generic system actor — the exact
//     failure mode Taura flagged.
//
//   - Token parses with a non-empty `sub` → stamps a user Principal on the
//     context and returns (false, nil). lib-auth still performs full
//     signature / audience / expiry validation downstream; only on its
//     approval will the audit writer observe the Principal.
//
// ParseUnverified is deliberate — lib-auth uses the same primitive internally
// for its authorization round-trip, so no additional trust is placed on this
// extraction step.
func extractPrincipalFromBearer(c *fiber.Ctx) (rejected bool, err error) {
	token := bearerToken(c)
	if token == "" {
		return false, nil
	}

	claims, ok := parseUnverifiedClaims(token)
	if !ok {
		return false, nil
	}

	sub := stringClaim(claims, "sub")
	if sub == "" {
		writeErr := pkgHTTP.Unauthorized(
			c,
			CodeUnauthorizedMissingSub,
			"Unauthorized",
			"Bearer token is missing the required 'sub' claim; identity cannot be attributed.",
		)

		return true, writeErr
	}

	name := stringClaim(claims, "preferred_username")
	if name == "" {
		name = stringClaim(claims, "email")
	}

	principal := contextutil.Principal{Type: string(model.ActorTypeUser), ID: sub, Name: name}
	c.SetUserContext(contextutil.WithPrincipal(c.UserContext(), principal))

	return false, nil
}
