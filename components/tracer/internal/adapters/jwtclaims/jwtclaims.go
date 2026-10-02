// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package jwtclaims reads bearer tokens and their claims for every tracer
// transport, so the HTTP listener and the gRPC reservation seam extract
// identity the same way.
//
// Claims are parsed WITHOUT verifying the signature. lib-auth parses the same
// token unverified for its Access Manager round trip; trust comes from that
// authorization, never from this parse, so a forged token cannot reach beyond
// what lib-auth refuses.
package jwtclaims

import (
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
)

// bearerPrefix is the case-insensitive scheme prefix of a bearer token.
const bearerPrefix = "bearer "

// BearerToken returns the trimmed token of an HTTP Authorization header value,
// or "" when the value does not use the Bearer scheme.
func BearerToken(header string) string {
	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}

	return strings.TrimSpace(header[len(bearerPrefix):])
}

// StripBearer returns the trimmed token of a gRPC authorization metadata value,
// with or without the Bearer scheme, as lib-auth reads it on gRPC.
func StripBearer(value string) string {
	raw := strings.TrimSpace(value)
	if len(raw) >= len(bearerPrefix) && strings.EqualFold(raw[:len(bearerPrefix)], bearerPrefix) {
		raw = strings.TrimSpace(raw[len(bearerPrefix):])
	}

	return raw
}

// ParseUnverified decodes the claims of token without verifying its signature.
// It reports false for an empty or malformed token.
func ParseUnverified(token string) (jwt.MapClaims, bool) {
	if token == "" {
		return nil, false
	}

	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
		return nil, false
	}

	return claims, true
}

// String returns the trimmed string value of key, or "" when the claim is
// absent or not a string.
func String(claims jwt.MapClaims, key string) string {
	value, _ := claims[key].(string)

	return strings.TrimSpace(value)
}
