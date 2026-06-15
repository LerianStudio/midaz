// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate_AppRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		config        Config
		expectError   bool
		errorContains string
	}{
		{
			name: "valid approle config with explicit method",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "secret-456",
			},
			expectError: false,
		},
		{
			name: "valid approle config with default method (empty)",
			config: Config{
				Addr:     "https://vault.example.com:8200",
				RoleID:   "role-123",
				SecretID: "secret-456",
			},
			expectError: false,
		},
		{
			name: "missing addr returns error",
			config: Config{
				Addr:       "",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "secret-456",
			},
			expectError:   true,
			errorContains: "addr",
		},
		{
			name: "missing role_id returns error",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "",
				SecretID:   "secret-456",
			},
			expectError:   true,
			errorContains: "role_id",
		},
		{
			name: "missing secret_id returns error",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "",
			},
			expectError:   true,
			errorContains: "secret_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_Token(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		config        Config
		expectError   bool
		errorContains string
	}{
		{
			name: "valid token config",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodToken,
				Token:      "hvs.test-token-123",
			},
			expectError: false,
		},
		{
			name: "token auth missing token returns error",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodToken,
				Token:      "",
			},
			expectError:   true,
			errorContains: "token",
		},
		{
			name: "token auth with whitespace-only token returns error",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodToken,
				Token:      "   ",
			},
			expectError:   true,
			errorContains: "token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_InvalidAuthMethod(t *testing.T) {
	t.Parallel()

	config := Config{
		Addr:       "https://vault.example.com:8200",
		AuthMethod: "invalid",
		RoleID:     "role-123",
		SecretID:   "secret-456",
	}

	err := config.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid auth method")
}

func TestConfig_EffectiveAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authMethod AuthMethod
		expected   AuthMethod
	}{
		{
			name:       "empty defaults to approle",
			authMethod: "",
			expected:   AuthMethodAppRole,
		},
		{
			name:       "approle returns approle",
			authMethod: AuthMethodAppRole,
			expected:   AuthMethodAppRole,
		},
		{
			name:       "token returns token",
			authMethod: AuthMethodToken,
			expected:   AuthMethodToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := Config{AuthMethod: tt.authMethod}

			assert.Equal(t, tt.expected, config.EffectiveAuthMethod())
		})
	}
}

func TestConfig_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name:     "empty config returns true",
			config:   Config{},
			expected: true,
		},
		{
			name: "config with only whitespace returns true",
			config: Config{
				Addr:     "   ",
				RoleID:   "   ",
				SecretID: "   ",
				Token:    "   ",
			},
			expected: true,
		},
		{
			name: "config with addr returns false",
			config: Config{
				Addr: "https://vault.example.com:8200",
			},
			expected: false,
		},
		{
			name: "config with token returns false",
			config: Config{
				Token: "hvs.token",
			},
			expected: false,
		},
		{
			name: "fully populated approle config returns false",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "secret-456",
			},
			expected: false,
		},
		{
			name: "fully populated token config returns false",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodToken,
				Token:      "hvs.token",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.config.IsEmpty()

			assert.Equal(t, tt.expected, result)
		})
	}
}
