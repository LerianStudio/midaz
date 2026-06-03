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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// testTokenResponse is the expected response from plugin-auth token endpoint.
type testTokenResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
}

// newTestContext creates a context with tenant ID, logger and tracer for testing.
func newTestContext(tenantID string) context.Context {
	ctx := context.Background()
	ctx = tmcore.ContextWithTenantID(ctx, tenantID)

	return ctx
}

// fakeCredentialFetcher simulates AWS Secrets Manager credential retrieval.
type fakeCredentialFetcher struct {
	mu          sync.Mutex
	credentials map[string]*M2MCredential
	callCount   int
	err         error
}

func (f *fakeCredentialFetcher) FetchCredential(_ context.Context, tenantID, _ string) (*M2MCredential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.callCount++

	if f.err != nil {
		return nil, f.err
	}

	cred, ok := f.credentials[tenantID]
	if !ok {
		return &M2MCredential{
			ClientID:     "client-" + tenantID,
			ClientSecret: "secret-" + tenantID,
		}, nil
	}

	return cred, nil
}

func TestM2MCredentialProvider_GetToken_Success(t *testing.T) {
	t.Parallel()

	expectedToken := "jwt-token-tenant-abc"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/login/oauth/access_token", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody tokenExchangeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.Equal(t, "client_credentials", reqBody.GrantType)
		assert.Equal(t, "client-tenant-abc", reqBody.ClientID)
		assert.Equal(t, "secret-tenant-abc", reqBody.ClientSecret)

		resp := testTokenResponse{
			AccessToken: expectedToken,
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &fakeCredentialFetcher{
		credentials: map[string]*M2MCredential{
			"tenant-abc": {ClientID: "client-tenant-abc", ClientSecret: "secret-tenant-abc"},
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

	ctx := newTestContext("tenant-abc")
	token, err := provider.GetToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, expectedToken, token)
}

func TestM2MCredentialProvider_GetToken_AlwaysFresh(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
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

	ctx := newTestContext("tenant-xyz")

	// First call
	token1, err := provider.GetToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", token1)

	// Second call should also hit auth server (no caching)
	token2, err := provider.GetToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", token2)

	// Both calls should have hit the auth server
	assert.Equal(t, int32(2), callCount.Load())
}

// newMarginTestProvider creates a provider against an httptest server that returns
// a token with the supplied expiresIn (seconds). When expiresIn is 0 the field is
// emitted as JSON zero — exercising the "TTL unknown" edge case (F1.R2).
func newMarginTestProvider(t *testing.T, expiresIn int, margin time.Duration) (*M2MCredentialProvider, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := testTokenResponse{
			AccessToken: "margin-test-token",
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:      server.URL,
			TargetService:    "fetcher",
			CredentialTTL:    5 * time.Minute,
			TokenCacheMargin: margin,
		},
		&fakeCredentialFetcher{},
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		NoopM2MMetrics(),
	)

	return provider, server
}

// TestGetToken_TokenCacheMargin consolidates the F1.2 boundary semantics in a
// single table-driven test. Each case exercises one rule of the safety-margin
// contract:
//
//   - F1.R1 (below margin)     → REJECT, error mentions "below safety margin"
//   - F1.R1 (above margin)     → ACCEPT, original token surfaces
//   - F1.R1 (inclusive bound)  → REJECT, the comparison is `<=`, not `<`
//   - F1.R2 (TTL unknown)      → ACCEPT with WARN, a zero ExpiresIn cannot
//     be meaningfully compared to a positive margin
//
// Driven via newMarginTestProvider (httptest + fakeCredentialFetcher) instead
// of a gomock mock because the rest of pkg/auth uses hand-rolled fakes and
// introducing mockgen for a single test would diverge from package convention.
func TestGetToken_TokenCacheMargin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expiresIn   int
		margin      time.Duration
		tenantID    string
		wantErr     bool
		wantErrText string // substring expected in err.Error() when wantErr is true
		wantToken   string // expected token value when wantErr is false
	}{
		{
			name:        "below margin — rejected",
			expiresIn:   3,
			margin:      5 * time.Second,
			tenantID:    "tenant-low-ttl",
			wantErr:     true,
			wantErrText: "below safety margin",
		},
		{
			name:      "above margin — accepted",
			expiresIn: 3600,
			margin:    60 * time.Second,
			tenantID:  "tenant-high-ttl",
			wantErr:   false,
			wantToken: "margin-test-token",
		},
		{
			name:        "exactly at margin — rejected (boundary is inclusive)",
			expiresIn:   5,
			margin:      5 * time.Second,
			tenantID:    "tenant-exact-margin",
			wantErr:     true,
			wantErrText: "below safety margin",
		},
		{
			name:      "expires_in == 0 — accepted as TTL unknown (F1.R2)",
			expiresIn: 0,
			margin:    60 * time.Second,
			tenantID:  "tenant-unknown-ttl",
			wantErr:   false,
			wantToken: "margin-test-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, server := newMarginTestProvider(t, tt.expiresIn, tt.margin)
			defer server.Close()

			ctx := newTestContext(tt.tenantID)
			token, err := provider.GetToken(ctx)

			if tt.wantErr {
				require.Error(t, err, "expected GetToken to reject token")
				assert.Empty(t, token, "no token should leak when GetToken rejects")
				assert.Contains(t, err.Error(), tt.wantErrText,
					"error must signal the rejection reason")

				return
			}

			require.NoError(t, err, "expected GetToken to succeed")
			assert.Equal(t, tt.wantToken, token)
		})
	}
}
