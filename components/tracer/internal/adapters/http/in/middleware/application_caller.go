// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/gofiber/fiber/v3"
)

// TokenTypeApplication is the "type" claim value of a machine-to-machine
// access token.
const TokenTypeApplication = "application"

// TokenCaller is the caller identity carried by the request's bearer token.
type TokenCaller struct {
	// Type is the token "type" claim ("application" or "normal-user").
	Type string
	// ClientID is the token "azp" claim: the client the token was issued to.
	ClientID string
}

// CallerResolver continues a request the guard admitted. ok is false when the
// guard does not authorize callers against the Access Manager or no caller
// identity can be read from the request, so a resolver must refuse on false.
type CallerResolver func(c fiber.Ctx, caller TokenCaller, ok bool) error

// AuthorizesCallers reports whether the guard authorizes every request
// against the Access Manager: plugin auth is enabled on a client that can
// reach it. Only then is a caller handed to a CallerResolver.
func (g *AuthGuard) AuthorizesCallers() bool {
	return g != nil && g.cfg.PluginAuthEnabled && g.authClient != nil && g.authClient.Enabled && g.authClient.Address != ""
}

// WithAuthorizedCaller returns the route handlers that authorize the request
// as resource:method and then pass its caller identity to resolve. The
// identity is readable only through this pair, so it is never read on a
// request the guard did not admit. A nil guard, or one that does not
// AuthorizesCallers, hands resolve no caller.
func (g *AuthGuard) WithAuthorizedCaller(resource, method string, resolve CallerResolver) []fiber.Handler {
	if !g.AuthorizesCallers() {
		refuse := func(c fiber.Ctx) error { return resolve(c, TokenCaller{}, false) }
		if g == nil {
			return []fiber.Handler{refuse}
		}

		return []fiber.Handler{g.With(resource, method, false), refuse}
	}

	return []fiber.Handler{
		g.With(resource, method, false),
		func(c fiber.Ctx) error {
			caller, ok := authorizedCaller(c)

			return resolve(c, caller, ok)
		},
	}
}

// authorizedCaller returns the identity of the request's bearer token. It
// prefers the Principal lib-auth published, which lib-auth withholds for an
// application token authorized under the legacy fabricated-role derivation;
// for that token it reads the "type" and "azp" claims without verifying the
// signature. Those claims are trusted only because the guard in front of it
// authorized this same token: the Access Manager introspects the token on
// /v1/authorize, so a forged or expired token never gets here. The token is
// read with the extraction lib-auth Authorize uses, so both read the same
// token. It reports false when no token can be read.
func authorizedCaller(c fiber.Ctx) (TokenCaller, bool) {
	if principal, ok := authMiddleware.PrincipalFromContext(c.Context()); ok {
		return TokenCaller{Type: principal.Type, ClientID: principal.ClientID}, true
	}

	claims, ok := parseUnverifiedClaims(libHTTP.ExtractTokenFromHeader(c))
	if !ok {
		return TokenCaller{}, false
	}

	kind, _ := claims["type"].(string)
	clientID, _ := claims["azp"].(string)

	return TokenCaller{Type: kind, ClientID: clientID}, true
}
