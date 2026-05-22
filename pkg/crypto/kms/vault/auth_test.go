// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMethod_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   AuthMethod
		expected string
	}{
		{
			name:     "approle returns approle",
			method:   AuthMethodAppRole,
			expected: "approle",
		},
		{
			name:     "token returns token",
			method:   AuthMethodToken,
			expected: "token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.method.String())
		})
	}
}

func TestAuthMethod_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   AuthMethod
		expected bool
	}{
		{
			name:     "approle is valid",
			method:   AuthMethodAppRole,
			expected: true,
		},
		{
			name:     "token is valid",
			method:   AuthMethodToken,
			expected: true,
		},
		{
			name:     "empty is invalid",
			method:   "",
			expected: false,
		},
		{
			name:     "unknown is invalid",
			method:   "unknown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.method.IsValid())
		})
	}
}

func TestParseAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expected      AuthMethod
		expectError   bool
		errorContains string
	}{
		{
			name:     "approle lowercase",
			input:    "approle",
			expected: AuthMethodAppRole,
		},
		{
			name:     "APPROLE uppercase",
			input:    "APPROLE",
			expected: AuthMethodAppRole,
		},
		{
			name:     "AppRole mixed case",
			input:    "AppRole",
			expected: AuthMethodAppRole,
		},
		{
			name:     "token lowercase",
			input:    "token",
			expected: AuthMethodToken,
		},
		{
			name:     "TOKEN uppercase",
			input:    "TOKEN",
			expected: AuthMethodToken,
		},
		{
			name:     "whitespace is trimmed",
			input:    "  approle  ",
			expected: AuthMethodAppRole,
		},
		{
			name:          "empty string returns error",
			input:         "",
			expectError:   true,
			errorContains: "invalid auth method",
		},
		{
			name:          "invalid method returns error",
			input:         "ldap",
			expectError:   true,
			errorContains: "invalid auth method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			method, err := ParseAuthMethod(tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, method)
			}
		})
	}
}

func TestValidateAuthCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        AuthMethod
		roleID        string
		secretID      string
		token         string
		expectError   bool
		errorContains string
	}{
		{
			name:     "approle with valid credentials",
			method:   AuthMethodAppRole,
			roleID:   "role-123",
			secretID: "secret-456",
		},
		{
			name:          "approle missing role_id",
			method:        AuthMethodAppRole,
			roleID:        "",
			secretID:      "secret-456",
			expectError:   true,
			errorContains: "role_id",
		},
		{
			name:          "approle missing secret_id",
			method:        AuthMethodAppRole,
			roleID:        "role-123",
			secretID:      "",
			expectError:   true,
			errorContains: "secret_id",
		},
		{
			name:          "approle missing both",
			method:        AuthMethodAppRole,
			roleID:        "",
			secretID:      "",
			expectError:   true,
			errorContains: "role_id",
		},
		{
			name:          "approle whitespace-only role_id",
			method:        AuthMethodAppRole,
			roleID:        "   ",
			secretID:      "secret-456",
			expectError:   true,
			errorContains: "role_id",
		},
		{
			name:   "token with valid token",
			method: AuthMethodToken,
			token:  "hvs.test-token-123",
		},
		{
			name:          "token missing token",
			method:        AuthMethodToken,
			token:         "",
			expectError:   true,
			errorContains: "token",
		},
		{
			name:          "token whitespace-only token",
			method:        AuthMethodToken,
			token:         "   ",
			expectError:   true,
			errorContains: "token",
		},
		{
			name:          "unknown method returns error",
			method:        "unknown",
			expectError:   true,
			errorContains: "unsupported auth method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAuthCredentials(tt.method, tt.roleID, tt.secretID, tt.token)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAuthCredentials_AppRoleIgnoresToken(t *testing.T) {
	t.Parallel()

	// AppRole auth should succeed even with token present (token is ignored)
	err := ValidateAuthCredentials(AuthMethodAppRole, "role-123", "secret-456", "some-token")

	require.NoError(t, err)
}

func TestValidateAuthCredentials_TokenIgnoresAppRoleCredentials(t *testing.T) {
	t.Parallel()

	// Token auth should succeed even with AppRole credentials present (they are ignored)
	err := ValidateAuthCredentials(AuthMethodToken, "role-123", "secret-456", "hvs.token")

	require.NoError(t, err)
}
