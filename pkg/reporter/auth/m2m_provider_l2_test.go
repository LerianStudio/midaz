// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// fakeL2Cache implements L2CredentialCache for testing.
type fakeL2Cache struct {
	mu   sync.Mutex
	data map[string]string
	gets int
	sets int
}

func newFakeL2Cache() *fakeL2Cache {
	return &fakeL2Cache{data: make(map[string]string)}
}

func (f *fakeL2Cache) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gets++

	v, ok := f.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}

	return v, nil
}

func (f *fakeL2Cache) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sets++

	if value == "" {
		delete(f.data, key)
		return nil
	}

	f.data[key] = value

	return nil
}

func newTokenServer(t *testing.T, token string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := testTokenResponse{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestM2MCredentialProvider_L1CacheHit_NoL2Access(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, "l1-hit-token")
	defer server.Close()

	l2 := newFakeL2Cache()
	fetcher := &fakeCredentialFetcher{}
	metrics := NoopM2MMetrics()

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
			L2Cache:       l2,
			L2TTL:         10 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		metrics,
	)

	ctx := newTestContext("tenant-l1")

	// First call: miss L1, miss L2, fetch → stores both
	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	// Second call: hit L1 → should not access L2
	l2.mu.Lock()
	getsAfterFirst := l2.gets
	l2.mu.Unlock()

	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	l2.mu.Lock()
	getsAfterSecond := l2.gets
	l2.mu.Unlock()

	assert.Equal(t, getsAfterFirst, getsAfterSecond, "L2 should not be accessed on L1 hit")
}

func TestM2MCredentialProvider_L2CacheHit_PopulatesL1(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, "l2-hit-token")
	defer server.Close()

	l2 := newFakeL2Cache()
	fetcher := &fakeCredentialFetcher{}
	metrics := NoopM2MMetrics()

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
			L2Cache:       l2,
			L2TTL:         10 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		metrics,
	)

	// Pre-populate L2 with a credential
	cred := &M2MCredential{ClientID: "client-l2", ClientSecret: "secret-l2"}
	credJSON, _ := json.Marshal(cred)
	l2Key := "m2m:cred:tenant-l2:fetcher"
	l2.data[l2Key] = string(credJSON)

	ctx := newTestContext("tenant-l2")

	// Call: miss L1, hit L2 → should use L2 credential
	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	// Verify fetcher was NOT called (L2 cache hit)
	fetcher.mu.Lock()
	assert.Equal(t, 0, fetcher.callCount, "Fetcher should not be called on L2 hit")
	fetcher.mu.Unlock()

	// Second call should hit L1 (populated from L2)
	l2.mu.Lock()
	getsAfterFirst := l2.gets
	l2.mu.Unlock()

	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	l2.mu.Lock()
	getsAfterSecond := l2.gets
	l2.mu.Unlock()

	assert.Equal(t, getsAfterFirst, getsAfterSecond, "L2 should not be accessed on second call (L1 populated)")
}

func TestM2MCredentialProvider_L2CacheMiss_FallsToFetcher(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, "fetched-token")
	defer server.Close()

	l2 := newFakeL2Cache()
	fetcher := &fakeCredentialFetcher{}
	metrics := NoopM2MMetrics()

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
			L2Cache:       l2,
			L2TTL:         10 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		metrics,
	)

	ctx := newTestContext("tenant-miss")

	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	// Fetcher should be called (L1 miss, L2 miss)
	fetcher.mu.Lock()
	assert.Equal(t, 1, fetcher.callCount, "Fetcher should be called on L1+L2 miss")
	fetcher.mu.Unlock()

	// L2 should have been populated
	l2.mu.Lock()
	assert.Equal(t, 1, l2.sets, "L2 should receive one Set after fetch")
	l2.mu.Unlock()
}

func TestM2MCredentialProvider_NilL2Cache_L1OnlyMode(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, "l1-only-token")
	defer server.Close()

	fetcher := &fakeCredentialFetcher{}
	metrics := NoopM2MMetrics()

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
			// L2Cache is nil — L1-only mode
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		metrics,
	)

	ctx := newTestContext("tenant-l1only")

	// Should work without panic or error
	token, err := provider.GetToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "l1-only-token", token)

	// Second call: L1 hit, fetcher called only once
	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	fetcher.mu.Lock()
	assert.Equal(t, 1, fetcher.callCount, "Fetcher should only be called once (L1 cached)")
	fetcher.mu.Unlock()
}

func TestM2MCredentialProvider_InvalidateCredentials(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, "invalidate-token")
	defer server.Close()

	l2 := newFakeL2Cache()
	fetcher := &fakeCredentialFetcher{}
	metrics := NoopM2MMetrics()

	provider := NewM2MCredentialProvider(
		M2MProviderConfig{
			AuthAddress:   server.URL,
			TargetService: "fetcher",
			CredentialTTL: 5 * time.Minute,
			L2Cache:       l2,
			L2TTL:         10 * time.Minute,
		},
		fetcher,
		log.NewNop(),
		noop.NewTracerProvider().Tracer("test"),
		metrics,
	)

	ctx := newTestContext("tenant-inv")

	// Populate caches
	_, err := provider.GetToken(ctx)
	require.NoError(t, err)

	fetcher.mu.Lock()
	assert.Equal(t, 1, fetcher.callCount)
	fetcher.mu.Unlock()

	// Invalidate
	require.NoError(t, provider.InvalidateCredentials(ctx, "tenant-inv", "explicit"))

	// Next call should fetch again (cache cleared)
	_, err = provider.GetToken(ctx)
	require.NoError(t, err)

	fetcher.mu.Lock()
	assert.Equal(t, 2, fetcher.callCount, "Fetcher should be called again after invalidation")
	fetcher.mu.Unlock()
}
