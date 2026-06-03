// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Auth dual-mode tests ---

func TestFetcherClient_ListConnections_NoAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/management/connections", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"), "single-tenant mode should have no auth header")
		assert.Empty(t, r.Header.Get("X-Organization-Id"), "must NOT send X-Organization-Id (D3)")
		assert.Empty(t, r.Header.Get("X-API-Key"), "must NOT send X-API-Key")
		assert.Empty(t, r.Header.Get("X-Tenant-ID"), "must NOT send X-Tenant-ID")

		resp := ConnectionListResponse{
			Connections: []ConnectionResponse{
				{ID: "ds-1", ConfigName: "onboarding", Type: "postgresql"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL) // no m2mProvider = single-tenant

	connections, err := client.ListConnections(context.Background())
	require.NoError(t, err)
	require.Len(t, connections, 1)
	assert.Equal(t, "ds-1", connections[0].ID)
	assert.Equal(t, "onboarding", connections[0].ConfigName)
}

func TestFetcherClient_ListConnections_WithAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer m2m-jwt-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Organization-Id"), "must NOT send X-Organization-Id (D3)")

		resp := ConnectionListResponse{
			Connections: []ConnectionResponse{
				{ID: "ds-2", ConfigName: "transactions", Type: "mongodb"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(&stubM2MProvider{token: "m2m-jwt-token"}),
	)

	connections, err := client.ListConnections(context.Background())
	require.NoError(t, err)
	require.Len(t, connections, 1)
	assert.Equal(t, "ds-2", connections[0].ID)
}

func TestFetcherClient_ListConnections_AuthTokenError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when token retrieval fails")
	}))
	defer server.Close()

	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(&stubM2MProvider{err: assert.AnError}),
	)

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get M2M token")
}

// --- HTTP error handling tests ---

func TestFetcherClient_ListConnections_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"INTERNAL","message":"internal error"}`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// --- Timeout handling tests ---

func TestFetcherClient_ListConnections_ContextTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.ListConnections(ctx)
	require.Error(t, err)
}

// --- Network error tests ---

func TestFetcherClient_ListConnections_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // Close immediately so connections are refused

	client := NewFetcherClient(server.URL)

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

func TestFetcherClient_GetConnectionSchema_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.GetConnectionSchema(context.Background(), "ds-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

func TestFetcherClient_ValidateSchema_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"ds-1": {"users": {"id"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

// --- Invalid URL tests ---

func TestFetcherClient_ListConnections_InvalidURL(t *testing.T) {
	t.Parallel()

	client := NewFetcherClient("http://\x00invalid")

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

// --- Invalid JSON response tests ---

func TestFetcherClient_ListConnections_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

// --- F2: 401 → InvalidateCredentials trigger tests ---

// invalidatingM2MProvider is a stub provider that records InvalidateCredentials
// calls. Used to assert F2 behavior: every 401 from a downstream call must
// invoke InvalidateCredentials(ctx, tenantID, "unauthorized").
type invalidatingM2MProvider struct {
	mu                  sync.Mutex
	token               string
	tokenErr            error
	invalidations       int
	lastTenantID        string
	lastTrigger         string
	invalidateReturnErr error
}

func (p *invalidatingM2MProvider) GetToken(_ context.Context) (string, error) {
	return p.token, p.tokenErr
}

func (p *invalidatingM2MProvider) InvalidateCredentials(_ context.Context, tenantID, trigger string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.invalidations++
	p.lastTenantID = tenantID
	p.lastTrigger = trigger

	return p.invalidateReturnErr
}

func (p *invalidatingM2MProvider) snapshot() (int, string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.invalidations, p.lastTenantID, p.lastTrigger
}

// ctxWithTenant returns a context carrying the given tenant ID so that the
// fetcher client's invalidateAuthOnUnauthorized helper can read it.
func ctxWithTenant(tenantID string) context.Context {
	return tmcore.ContextWithTenantID(context.Background(), tenantID)
}

// TestFetcherClient_InvalidatesCredentialsOn401 verifies that any downstream
// 401 response triggers M2M credential invalidation with trigger="unauthorized"
// for the tenant in context. The original 401 error MUST still propagate.
func TestFetcherClient_InvalidatesCredentialsOn401(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":"AUT-1003","message":"unauthorized"}`))
	}))
	defer server.Close()

	provider := &invalidatingM2MProvider{token: "expired-token"}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	_, err := client.ListConnections(ctxWithTenant("tenant-x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401", "original 401 error must propagate")

	invalidations, tenantID, trigger := provider.snapshot()
	assert.Equal(t, 1, invalidations, "InvalidateCredentials should be called exactly once on 401")
	assert.Equal(t, "tenant-x", tenantID, "InvalidateCredentials must receive the tenant from context")
	assert.Equal(t, "unauthorized", trigger, `trigger must be "unauthorized" for 401-driven invalidations`)
}

// TestFetcherClient_DoesNotInvalidateOn500 verifies that non-401 responses
// (e.g., 5xx, 4xx other than 401) do NOT trigger credential invalidation.
func TestFetcherClient_DoesNotInvalidateOn500(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"INTERNAL","message":"boom"}`))
	}))
	defer server.Close()

	provider := &invalidatingM2MProvider{token: "valid-token"}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	_, err := client.ListConnections(ctxWithTenant("tenant-y"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")

	invalidations, _, _ := provider.snapshot()
	assert.Equal(t, 0, invalidations, "500 must NOT trigger credential invalidation")
}

// TestFetcherClient_NoOpInSingleTenant verifies that with m2mProvider == nil
// (single-tenant mode), a 401 surfaces as an error WITHOUT panicking and
// WITHOUT attempting to invalidate (there is nothing to invalidate).
func TestFetcherClient_NoOpInSingleTenant(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":"AUT-1003"}`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL) // no m2mProvider = single-tenant

	// Must not panic even though there is no provider to invalidate against.
	require.NotPanics(t, func() {
		_, err := client.ListConnections(ctxWithTenant("tenant-irrelevant"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}

// TestFetcherClient_NoOpWhenTenantIDMissing verifies that when no tenant ID
// is present in context, the helper is a silent no-op — no panic, no
// invalidation call attempted (we have no key to invalidate against).
func TestFetcherClient_NoOpWhenTenantIDMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":"AUT-1003"}`))
	}))
	defer server.Close()

	provider := &invalidatingM2MProvider{token: "any-token"}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	// Plain context — no tenant ID.
	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")

	invalidations, _, _ := provider.snapshot()
	assert.Equal(t, 0, invalidations, "missing tenantID must skip invalidation silently")
}

// rotatingM2MProvider is a stub provider that returns a different token after
// each InvalidateCredentials call. Used by F3 tests to assert that the retry
// attempt carries a fresh token (not the stale one rejected by the server).
type rotatingM2MProvider struct {
	mu            sync.Mutex
	tokens        []string // tokens to return in order
	idx           int
	invalidations int
}

func (p *rotatingM2MProvider) GetToken(_ context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1], nil
	}

	tok := p.tokens[p.idx]
	p.idx++

	return tok, nil
}

func (p *rotatingM2MProvider) InvalidateCredentials(_ context.Context, _ /*tenantID*/, _ /*trigger*/ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.invalidations++

	return nil
}

func (p *rotatingM2MProvider) invalidationCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.invalidations
}

// TestFetcherClient_InvalidateErrorIsSwallowed verifies that an error returned
// by InvalidateCredentials (e.g., Redis down) does NOT mask the original 401
// from the caller. The 401 must still propagate; the invalidation failure is
// logged at WARN by the helper.
func TestFetcherClient_InvalidateErrorIsSwallowed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":"AUT-1003"}`))
	}))
	defer server.Close()

	provider := &invalidatingM2MProvider{
		token:               "expired",
		invalidateReturnErr: assert.AnError,
	}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	_, err := client.ListConnections(ctxWithTenant("tenant-redis-down"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401", "original 401 must surface even when invalidation fails")

	invalidations, _, _ := provider.snapshot()
	assert.Equal(t, 1, invalidations, "InvalidateCredentials should still be attempted")
}

// --- F3: defensive auth retry on 401 ---

// TestFetcherClient_RetriesOn401_Succeeds verifies the happy retry path:
// the first attempt returns 401, the helper invalidates credentials, mints
// a fresh token, and the second attempt succeeds with the new token.
func TestFetcherClient_RetriesOn401_Succeeds(t *testing.T) {
	t.Parallel()

	var (
		callCount    int32
		seenTokens   []string
		seenTokensMu sync.Mutex
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)

		seenTokensMu.Lock()
		seenTokens = append(seenTokens, r.Header.Get("Authorization"))
		seenTokensMu.Unlock()

		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"AUT-1003","message":"unauthorized"}`))

			return
		}

		resp := ConnectionListResponse{
			Connections: []ConnectionResponse{{ID: "ds-1", ConfigName: "onboarding", Type: "postgresql"}},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &rotatingM2MProvider{tokens: []string{"tokenA", "tokenB"}}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	connections, err := client.ListConnections(ctxWithTenant("tenant-r1"))
	require.NoError(t, err, "retry with fresh token should succeed")
	require.Len(t, connections, 1)
	assert.Equal(t, "ds-1", connections[0].ID)

	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "exactly two HTTP attempts must occur")
	assert.Equal(t, 1, provider.invalidationCount(), "InvalidateCredentials must be called exactly once")

	seenTokensMu.Lock()
	defer seenTokensMu.Unlock()
	require.Len(t, seenTokens, 2)
	assert.Equal(t, "Bearer tokenA", seenTokens[0], "first attempt uses the stale token")
	assert.Equal(t, "Bearer tokenB", seenTokens[1], "second attempt uses the fresh token")
}

// TestFetcherClient_RetriesOn401_FailsAgain verifies the bounded-retry contract:
// when the second attempt also returns 401 (e.g., Casdoor still down or token
// truly invalid), the helper does NOT retry a third time. Exactly two HTTP
// attempts are made and the second 401 propagates as the surfaced error.
func TestFetcherClient_RetriesOn401_FailsAgain(t *testing.T) {
	t.Parallel()

	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"AUT-1003","message":"unauthorized"}`))
	}))
	defer server.Close()

	provider := &rotatingM2MProvider{tokens: []string{"tokenA", "tokenB"}}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	_, err := client.ListConnections(ctxWithTenant("tenant-r2"))
	require.Error(t, err, "second 401 must surface as an error")
	assert.Contains(t, err.Error(), "401")

	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "retry loop must be bounded to a single re-attempt (2 total)")
	assert.Equal(t, 1, provider.invalidationCount(), "InvalidateCredentials must still be invoked once on the first 401")
}

// TestFetcherClient_NoRetryOn500 verifies that non-401 errors (5xx, 4xx other
// than 401) do NOT trigger a retry. The original status surfaces directly.
func TestFetcherClient_NoRetryOn500(t *testing.T) {
	t.Parallel()

	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL","message":"boom"}`))
	}))
	defer server.Close()

	provider := &rotatingM2MProvider{tokens: []string{"tokenA"}}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	_, err := client.ListConnections(ctxWithTenant("tenant-r3"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "5xx must not trigger a retry")
	assert.Equal(t, 0, provider.invalidationCount(), "5xx must not invoke InvalidateCredentials")
}

// TestFetcherClient_NoRetryInSingleTenant verifies that with m2mProvider == nil
// (single-tenant mode), a 401 propagates verbatim without any retry attempt.
// The legacy single-tenant path must remain unaffected by F3.
func TestFetcherClient_NoRetryInSingleTenant(t *testing.T) {
	t.Parallel()

	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"AUT-1003"}`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL) // no m2mProvider = single-tenant

	_, err := client.ListConnections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "single-tenant mode must not retry on 401")
}

// TestFetcherClient_RetryPreservesPOSTBody verifies that a POST request body is
// preserved across the retry boundary. The server returns 401 on the first
// attempt and 200 on the second; we assert that both attempts received the
// IDENTICAL body. This guards against the regression where http.Request.Body
// is consumed on the first read and the second attempt sends an empty body.
func TestFetcherClient_RetryPreservesPOSTBody(t *testing.T) {
	t.Parallel()

	var (
		callCount    int32
		seenBodies   []string
		seenBodiesMu sync.Mutex
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		seenBodiesMu.Lock()
		seenBodies = append(seenBodies, string(bodyBytes))
		seenBodiesMu.Unlock()

		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"AUT-1003"}`))

			return
		}

		// Successful validation response on second attempt.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","message":"ok","errors":[]}`))
	}))
	defer server.Close()

	provider := &rotatingM2MProvider{tokens: []string{"tokenA", "tokenB"}}
	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(provider),
	)

	mappedFields := map[string]map[string][]string{
		"ds-1": {"users": {"id", "name", "email"}},
	}

	result, err := client.ValidateSchema(ctxWithTenant("tenant-r5"), mappedFields)
	require.NoError(t, err, "retry path must succeed with the body intact")
	require.NotNil(t, result)
	assert.Equal(t, "success", result.Status)

	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))

	seenBodiesMu.Lock()
	defer seenBodiesMu.Unlock()
	require.Len(t, seenBodies, 2)
	assert.NotEmpty(t, seenBodies[0], "first attempt must carry the payload")
	assert.Equal(t, seenBodies[0], seenBodies[1], "second attempt must receive the SAME body as the first")
	assert.Contains(t, seenBodies[1], "users", "retry body must contain the original mapped-field payload")
}
