// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package middleware provides HTTP middleware for the Tracer API.
package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// HeaderAPIKey is the HTTP header name for API key authentication.
// #nosec G101 -- HTTP header name, not a credential value.
const HeaderAPIKey = "X-API-Key"

// defaultAPIKeyLabel is the actor identifier recorded for API-key authenticated
// requests when the operator did not configure API_KEY_LABEL. Bootstrap applies
// this same default, but the middleware duplicates it so it is safe to use
// standalone (e.g. from tests).
// #nosec G101 -- audit actor identifier, not a credential value.
const defaultAPIKeyLabel = "tracer-default"

// Auth failure reasons for logging and metrics.
const (
	ReasonMissingAPIKey = "missing_api_key"
	ReasonInvalidAPIKey = "invalid_api_key"
)

// APIKeyConfig holds the configuration for API Key authentication.
type APIKeyConfig struct {
	// Key is the expected API key value for authentication.
	Key string

	// Enabled controls whether authentication is enforced.
	// When false, all requests pass through without validation (dev mode).
	Enabled bool

	// Label is the audit actor ID recorded after a successful API-key match.
	// When empty, the middleware falls back to defaultAPIKeyLabel so audit
	// rows always carry a non-empty actor identifier.
	Label string
}

// validateAPIKey checks if the provided API key is valid.
// Returns the failure reason ("missing_api_key" or "invalid_api_key") or empty string if valid.
// Uses constant-time comparison to prevent timing attacks.
func validateAPIKey(apiKey, expectedKey string) string {
	if apiKey == "" {
		return ReasonMissingAPIKey
	}

	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(expectedKey)) != 1 {
		return ReasonInvalidAPIKey
	}

	return ""
}

// APIKeyAuth creates a Fiber middleware handler that validates API key authentication.
//
// The middleware extracts the API key from the X-API-Key header and validates it
// against the configured key using constant-time comparison to prevent timing attacks.
//
// Security considerations:
//   - Uses crypto/subtle.ConstantTimeCompare to prevent timing attacks
//   - Returns the same error message for missing and invalid keys to prevent enumeration
//   - Never logs the API key value
//
// On a successful key match, the middleware stamps a contextutil.Principal of
// type "api_key" with ID=label onto the Fiber UserContext so the audit writer
// can attribute downstream actions to a specific deployment instead of falling
// back to the generic "system/svc_tracer" actor.
//
// When cfg.Enabled is false, the middleware passes all requests through without
// validation (useful for development environments) and does NOT stamp a Principal —
// dev-mode traffic must reach the audit writer with no authenticated identity
// so the system fallback kicks in transparently.
func APIKeyAuth(cfg APIKeyConfig) fiber.Handler {
	label := strings.TrimSpace(cfg.Label)
	if label == "" {
		label = defaultAPIKeyLabel
	}

	return func(c *fiber.Ctx) error {
		// Skip authentication if disabled (dev mode)
		if !cfg.Enabled {
			return c.Next()
		}

		if reason := validateAPIKey(c.Get(HeaderAPIKey), cfg.Key); reason != "" {
			return pkgHTTP.Unauthorized(c, "Unauthenticated", "Unauthorized", "API Key missing or invalid")
		}

		principal := contextutil.Principal{Type: string(model.ActorTypeAPIKey), ID: label}
		c.SetUserContext(contextutil.WithPrincipal(c.UserContext(), principal))

		return c.Next()
	}
}
