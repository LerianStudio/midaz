// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestM2MCredentialProvider_GetToken_AlwaysCallsAuthServer(t *testing.T) {
	t.Parallel()

	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		resp := testTokenResponse{
			AccessToken: "fresh-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &fakeCredentialFetcher{}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	ctx := newTestContext("tenant-refresh")

	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	// Both calls should hit auth server (no token caching)
	assert.Equal(t, 2, callCount)
}

func TestM2MCredentialProvider_GetToken_NoTenantID_ReturnsError(t *testing.T) {
	t.Parallel()

	fetcher := &fakeCredentialFetcher{}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   "http://unused",
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	// Context without tenant ID
	ctx := context.Background()
	_, err := provider.GetToken(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant ID not found in context")
}

func TestM2MCredentialProvider_GetToken_CredentialFetchError(t *testing.T) {
	t.Parallel()

	fetcher := &fakeCredentialFetcher{
		err: assert.AnError,
	}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   "http://unused",
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	ctx := newTestContext("tenant-fail")
	_, err := provider.GetToken(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch M2M credential")
}

func TestM2MCredentialProvider_GetToken_AuthServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer server.Close()

	fetcher := &fakeCredentialFetcher{}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	ctx := newTestContext("tenant-auth-fail")
	_, err := provider.GetToken(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token exchange failed")
}

func TestM2MCredentialProvider_GetToken_CachesCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := testTokenResponse{
			AccessToken: "token-cached-cred",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &fakeCredentialFetcher{}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	ctx := newTestContext("tenant-cred-cache")

	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	// Credential fetcher should only be called once (credential cached)
	fetcher.mu.Lock()
	assert.Equal(t, 1, fetcher.callCount)
	fetcher.mu.Unlock()
}

func TestM2MCredentialProvider_GetToken_PerTenantIsolation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody tokenExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		resp := testTokenResponse{
			AccessToken: "token-for-" + reqBody.ClientID,
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &fakeCredentialFetcher{
		credentials: map[string]*M2MCredential{
			"tenant-a": {ClientID: "client-a", ClientSecret: "secret-a"},
			"tenant-b": {ClientID: "client-b", ClientSecret: "secret-b"},
		},
	}

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	ctxA := newTestContext("tenant-a")
	ctxB := newTestContext("tenant-b")

	tokenA, err := provider.GetToken(ctxA)
	require.NoError(t, err)

	tokenB, err := provider.GetToken(ctxB)
	require.NoError(t, err)

	assert.Equal(t, "token-for-client-a", tokenA)
	assert.Equal(t, "token-for-client-b", tokenB)
	assert.NotEqual(t, tokenA, tokenB, "different tenants must receive different tokens")
}
