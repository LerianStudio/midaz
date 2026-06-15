// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid approle config creates client",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "secret-456",
			},
			expectError: false,
		},
		{
			name: "valid token config creates client",
			config: Config{
				Addr:       "https://vault.example.com:8200",
				AuthMethod: AuthMethodToken,
				Token:      "hvs.test-token",
			},
			expectError: false,
		},
		{
			name: "invalid config returns error",
			config: Config{
				Addr:       "",
				AuthMethod: AuthMethodAppRole,
				RoleID:     "role-123",
				SecretID:   "secret-456",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestClient_Login_AppRole(t *testing.T) {
	t.Parallel()

	t.Run("successful approle login sets token", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/auth/approle/login" {
				resp := map[string]any{
					"auth": map[string]any{
						"client_token":   "test-token-123",
						"lease_duration": 3600,
						"renewable":      true,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)

				return
			}

			http.NotFound(w, r)
		}))
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())

		require.NoError(t, err)
		assert.True(t, client.isLoggedIn)
	})

	t.Run("failed approle login returns error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{
				"errors": []string{"permission denied"},
			})
		}))
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			AuthMethod: AuthMethodAppRole,
			RoleID:     "invalid-role",
			SecretID:   "invalid-secret",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())

		require.Error(t, err)
		assert.False(t, client.isLoggedIn)
	})

	t.Run("empty auth response returns error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{})
		}))
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty auth response")
	})
}

func TestClient_Login_Token(t *testing.T) {
	t.Parallel()

	t.Run("token login sets token directly", func(t *testing.T) {
		t.Parallel()

		// Token auth doesn't make any HTTP calls during login
		client, err := NewClient(Config{
			Addr:       "https://vault.example.com:8200",
			AuthMethod: AuthMethodToken,
			Token:      "hvs.test-token-123",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())

		require.NoError(t, err)
		assert.True(t, client.isLoggedIn)
	})
}

func TestClient_AuthMethod(t *testing.T) {
	t.Parallel()

	t.Run("returns approle for approle config", func(t *testing.T) {
		t.Parallel()

		client, err := NewClient(Config{
			Addr:       "https://vault.example.com:8200",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		assert.Equal(t, AuthMethodAppRole, client.AuthMethod())
	})

	t.Run("returns token for token config", func(t *testing.T) {
		t.Parallel()

		client, err := NewClient(Config{
			Addr:       "https://vault.example.com:8200",
			AuthMethod: AuthMethodToken,
			Token:      "hvs.test-token",
		})
		require.NoError(t, err)

		assert.Equal(t, AuthMethodToken, client.AuthMethod())
	})

	t.Run("returns approle for default (empty) auth method", func(t *testing.T) {
		t.Parallel()

		client, err := NewClient(Config{
			Addr:     "https://vault.example.com:8200",
			RoleID:   "role-123",
			SecretID: "secret-456",
		})
		require.NoError(t, err)

		assert.Equal(t, AuthMethodAppRole, client.AuthMethod())
	})
}

func TestClient_Close(t *testing.T) {
	t.Parallel()

	t.Run("close clears login state for approle", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"auth": map[string]any{
					"client_token":   "test-token-123",
					"lease_duration": 3600,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())
		require.NoError(t, err)
		assert.True(t, client.isLoggedIn)

		err = client.Close()

		require.NoError(t, err)
		assert.False(t, client.isLoggedIn)
	})

	t.Run("close clears login state for token", func(t *testing.T) {
		t.Parallel()

		client, err := NewClient(Config{
			Addr:       "https://vault.example.com:8200",
			AuthMethod: AuthMethodToken,
			Token:      "hvs.test-token",
		})
		require.NoError(t, err)

		err = client.Login(context.Background())
		require.NoError(t, err)
		assert.True(t, client.isLoggedIn)

		err = client.Close()

		require.NoError(t, err)
		assert.False(t, client.isLoggedIn)
	})
}

func TestIsPermissionDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-response error returns false",
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isPermissionDenied(tt.err)

			assert.Equal(t, tt.expected, result)
		})
	}
}
