// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServer creates a test server that handles AppRole login and Transit operations.
func newTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default AppRole login handler
		if r.URL.Path == "/v1/auth/approle/login" {
			resp := map[string]any{
				"auth": map[string]any{
					"client_token":   "test-token",
					"lease_duration": 3600,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

			return
		}

		// Check for custom handlers
		if handler, ok := handlers[r.URL.Path]; ok {
			handler(w, r)

			return
		}

		http.NotFound(w, r)
	}))
}

func TestClient_Encrypt(t *testing.T) {
	t.Parallel()

	t.Run("successful encryption", func(t *testing.T) {
		t.Parallel()

		plaintext := []byte("sensitive data")
		expectedCiphertext := "vault:v1:encrypted-data"

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/encrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				json.NewDecoder(r.Body).Decode(&req)

				// Verify plaintext is base64 encoded
				receivedB64, ok := req["plaintext"].(string)
				require.True(t, ok)

				decoded, err := base64.StdEncoding.DecodeString(receivedB64)
				require.NoError(t, err)
				assert.Equal(t, plaintext, decoded)

				resp := map[string]any{
					"data": map[string]any{
						"ciphertext": expectedCiphertext,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		ciphertext, err := client.Encrypt(context.Background(), "org/test-org-id", plaintext)

		require.NoError(t, err)
		assert.Equal(t, expectedCiphertext, ciphertext)
	})

	t.Run("re-authenticates on 403 and retries", func(t *testing.T) {
		t.Parallel()

		var callCount atomic.Int32

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/encrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				count := callCount.Add(1)

				if count == 1 {
					// First call: return 403 to trigger re-auth
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(map[string]any{
						"errors": []string{"permission denied"},
					})

					return
				}

				// Second call: return success
				resp := map[string]any{
					"data": map[string]any{
						"ciphertext": "vault:v1:retry-success",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		ciphertext, err := client.Encrypt(context.Background(), "org/test-org-id", []byte("data"))

		require.NoError(t, err)
		assert.Equal(t, "vault:v1:retry-success", ciphertext)
		assert.Equal(t, int32(2), callCount.Load())
	})

	t.Run("empty response returns error", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/encrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{})
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		_, err = client.Encrypt(context.Background(), "org/test-org-id", []byte("data"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("missing ciphertext in response returns error", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/encrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]any{
					"data": map[string]any{
						"other_field": "value",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		_, err = client.Encrypt(context.Background(), "org/test-org-id", []byte("data"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing ciphertext")
	})
}

func TestClient_Decrypt(t *testing.T) {
	t.Parallel()

	t.Run("successful decryption", func(t *testing.T) {
		t.Parallel()

		expectedPlaintext := []byte("sensitive data")
		ciphertext := "vault:v1:encrypted-data"

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/decrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				json.NewDecoder(r.Body).Decode(&req)

				// Verify ciphertext is passed correctly
				receivedCiphertext, ok := req["ciphertext"].(string)
				require.True(t, ok)
				assert.Equal(t, ciphertext, receivedCiphertext)

				resp := map[string]any{
					"data": map[string]any{
						"plaintext": base64.StdEncoding.EncodeToString(expectedPlaintext),
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		plaintext, err := client.Decrypt(context.Background(), "org/test-org-id", ciphertext)

		require.NoError(t, err)
		assert.Equal(t, expectedPlaintext, plaintext)
	})

	t.Run("re-authenticates on 403 and retries", func(t *testing.T) {
		t.Parallel()

		var callCount atomic.Int32
		expectedPlaintext := []byte("retry data")

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/decrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				count := callCount.Add(1)

				if count == 1 {
					// First call: return 403 to trigger re-auth
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(map[string]any{
						"errors": []string{"permission denied"},
					})

					return
				}

				// Second call: return success
				resp := map[string]any{
					"data": map[string]any{
						"plaintext": base64.StdEncoding.EncodeToString(expectedPlaintext),
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		plaintext, err := client.Decrypt(context.Background(), "org/test-org-id", "vault:v1:data")

		require.NoError(t, err)
		assert.Equal(t, expectedPlaintext, plaintext)
		assert.Equal(t, int32(2), callCount.Load())
	})

	t.Run("empty response returns error", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/decrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{})
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		_, err = client.Decrypt(context.Background(), "org/test-org-id", "vault:v1:data")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("invalid base64 plaintext returns error", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/decrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]any{
					"data": map[string]any{
						"plaintext": "not-valid-base64!!!",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		_, err = client.Decrypt(context.Background(), "org/test-org-id", "vault:v1:data")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode plaintext")
	})
}

func TestClient_EncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("encrypt then decrypt returns original data", func(t *testing.T) {
		t.Parallel()

		originalData := []byte("sensitive information that needs protection")
		var storedCiphertext string

		server := newTestServer(t, map[string]http.HandlerFunc{
			"/v1/transit/encrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				json.NewDecoder(r.Body).Decode(&req)

				// Store the "encrypted" data (just add prefix for simulation)
				plaintextB64 := req["plaintext"].(string)
				storedCiphertext = "vault:v1:" + plaintextB64

				resp := map[string]any{
					"data": map[string]any{
						"ciphertext": storedCiphertext,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			"/v1/transit/decrypt/org/test-org-id": func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				json.NewDecoder(r.Body).Decode(&req)

				// Extract the "plaintext" from our simulated ciphertext
				ciphertext := req["ciphertext"].(string)
				plaintextB64 := ciphertext[len("vault:v1:"):]

				resp := map[string]any{
					"data": map[string]any{
						"plaintext": plaintextB64,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
		})
		defer server.Close()

		client, err := NewClient(Config{
			Addr:       server.URL,
			MountPath:  "transit",
			AuthMethod: AuthMethodAppRole,
			RoleID:     "role-123",
			SecretID:   "secret-456",
		})
		require.NoError(t, err)

		// Encrypt
		ciphertext, err := client.Encrypt(context.Background(), "org/test-org-id", originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, ciphertext)
		assert.NotEqual(t, string(originalData), ciphertext)

		// Decrypt
		decrypted, err := client.Decrypt(context.Background(), "org/test-org-id", ciphertext)
		require.NoError(t, err)

		// Verify round-trip
		assert.Equal(t, originalData, decrypted)
	})
}
