// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package jwtclaims

import (
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT builds a signed JWT. The signature is irrelevant to ParseUnverified,
// but the token must be well formed.
func makeJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-only-signing-key"))
	require.NoError(t, err)

	return token
}

func TestBearerToken(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		header string
		want   string
	}{
		"empty header":            {header: "", want: ""},
		"non-bearer scheme":       {header: "Basic abc", want: ""},
		"shorter than the prefix": {header: "Bear", want: ""},
		"bare token":              {header: "my-token", want: ""},
		"case-insensitive prefix": {header: "bEaReR my-token", want: "my-token"},
		"trims the token":         {header: "Bearer   spaced-token   ", want: "spaced-token"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, BearerToken(tt.header))
		})
	}
}

func TestStripBearer(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		want  string
	}{
		"empty value":             {value: "", want: ""},
		"blank value":             {value: "   ", want: ""},
		"bare token":              {value: "my-token", want: "my-token"},
		"bearer prefix":           {value: "Bearer my-token", want: "my-token"},
		"case-insensitive prefix": {value: "bEaReR my-token", want: "my-token"},
		"surrounding whitespace":  {value: "  Bearer   spaced-token  ", want: "spaced-token"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, StripBearer(tt.value))
		})
	}
}

func TestParseUnverified_ValidToken(t *testing.T) {
	t.Parallel()

	claims, ok := ParseUnverified(makeJWT(t, jwt.MapClaims{"sub": "user-123", "preferred_username": "alice"}))
	require.True(t, ok)
	assert.Equal(t, "user-123", claims["sub"])
	assert.Equal(t, "alice", claims["preferred_username"])
}

func TestParseUnverified_CorruptToken_ReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := ParseUnverified("not.a.jwt")
	assert.False(t, ok)
}

func TestParseUnverified_EmptyToken_ReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := ParseUnverified("")
	assert.False(t, ok)
}

func TestString(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		claims jwt.MapClaims
		want   string
	}{
		"present":          {claims: jwt.MapClaims{"sub": "user-123"}, want: "user-123"},
		"missing":          {claims: jwt.MapClaims{}, want: ""},
		"nil claims":       {claims: nil, want: ""},
		"non-string value": {claims: jwt.MapClaims{"sub": 42}, want: ""},
		"trims whitespace": {claims: jwt.MapClaims{"sub": "  alice  "}, want: "alice"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, String(tt.claims, "sub"))
		})
	}
}
