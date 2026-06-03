// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT builds a signed JWT for tests. Signature is irrelevant for the
// ParseUnverified path, but a structurally valid token is required.
func makeJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte("test-only-signing-key"))
	require.NoError(t, err)

	return s
}

// captureBearerToken runs bearerToken inside a Fiber handler and returns the
// extracted value for the test to assert on.
func captureBearerToken(t *testing.T, authHeader string) string {
	t.Helper()

	app := fiber.New()

	var got string

	app.Get("/", func(c *fiber.Ctx) error {
		got = bearerToken(c)
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	return got
}

func TestBearerToken_NoHeader_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, captureBearerToken(t, ""))
}

func TestBearerToken_NonBearerScheme_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, captureBearerToken(t, "Basic abc"))
}

func TestBearerToken_ShorterThanPrefix_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, captureBearerToken(t, "Bear"))
}

func TestBearerToken_CaseInsensitivePrefix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "my-token", captureBearerToken(t, "bEaReR my-token"))
}

func TestBearerToken_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "spaced-token", captureBearerToken(t, "Bearer   spaced-token   "))
}

func TestParseUnverifiedClaims_ValidToken(t *testing.T) {
	t.Parallel()

	tok := makeJWT(t, jwt.MapClaims{"sub": "user-123", "preferred_username": "alice"})

	claims, ok := parseUnverifiedClaims(tok)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims["sub"])
	assert.Equal(t, "alice", claims["preferred_username"])
}

func TestParseUnverifiedClaims_CorruptToken_ReturnsFalse(t *testing.T) {
	t.Parallel()
	_, ok := parseUnverifiedClaims("not.a.jwt")
	assert.False(t, ok)
}

func TestParseUnverifiedClaims_EmptyToken_ReturnsFalse(t *testing.T) {
	t.Parallel()
	_, ok := parseUnverifiedClaims("")
	assert.False(t, ok)
}

func TestStringClaim_Present(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "user-123", stringClaim(jwt.MapClaims{"sub": "user-123"}, "sub"))
}

func TestStringClaim_Missing(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", stringClaim(jwt.MapClaims{}, "sub"))
}

func TestStringClaim_NilClaims(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", stringClaim(nil, "sub"))
}

func TestStringClaim_NonStringValue_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", stringClaim(jwt.MapClaims{"sub": 42}, "sub"))
}

func TestStringClaim_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "alice", stringClaim(jwt.MapClaims{"preferred_username": "  alice  "}, "preferred_username"))
}
