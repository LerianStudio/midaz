// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
)

// bearerHeader is the standard HTTP Authorization header name.
const bearerHeader = "Authorization"

// bearerPrefix is the case-insensitive scheme prefix used for Bearer tokens.
const bearerPrefix = "Bearer "

// bearerToken extracts the raw JWT from the Authorization header.
// Returns empty string when the header is missing or not a Bearer scheme.
func bearerToken(c *fiber.Ctx) string {
	header := c.Get(bearerHeader)
	if header == "" {
		return ""
	}

	if len(header) < len(bearerPrefix) {
		return ""
	}

	if !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}

	return strings.TrimSpace(header[len(bearerPrefix):])
}

// parseUnverifiedClaims decodes JWT claims WITHOUT verifying the signature.
//
// Safety rationale: lib-auth/v2's Authorize uses ParseUnverified on the same
// token to make its authorization-service round-trip. We use it here only to
// surface identity claims into the request context. Trust comes from the
// downstream Access Manager check — not from this parse — so an attacker
// crafting an arbitrary JWT cannot escalate beyond what lib-auth rejects.
func parseUnverifiedClaims(token string) (jwt.MapClaims, bool) {
	parser := jwt.NewParser()

	claims := jwt.MapClaims{}

	_, _, err := parser.ParseUnverified(token, &claims)
	if err != nil {
		return nil, false
	}

	return claims, true
}

// stringClaim returns the trimmed string value of a claim, or empty string if
// the claim is missing or not a string.
func stringClaim(claims jwt.MapClaims, key string) string {
	if claims == nil {
		return ""
	}

	v, ok := claims[key]
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(s)
}
