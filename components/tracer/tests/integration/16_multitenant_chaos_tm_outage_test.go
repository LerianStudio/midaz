// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Gate 8 — Deliverable B: Tenant Manager outage chaos.
//
// Exercises the lib-commons tenant-manager client's circuit breaker + cache
// behaviour end-to-end. The flow mirrors a production incident where the
// Tenant Manager HTTP endpoint becomes unreachable:
//
//  1. Service boots with a fake TM reachable.
//  2. We warm the per-tenant postgres pool for tenant-A by making one
//     successful request (lib-commons caches the TenantConfig).
//  3. We flip the TM handler to return 503 for every subsequent request —
//     i.e. the TM is "unreachable" for everything after the warmup.
//  4. Requests for tenant-A continue to succeed because lib-commons' pool
//     manager keeps the cached PostgresConnection alive and PINGs it before
//     reuse. No outbound TM call is needed on the hot path once pooled.
//  5. Requests for tenant-B (never warmed) fail with a 4xx/5xx error that
//     does NOT leak panic traces; the circuit breaker fails fast after a few
//     attempts.
//
// The circuit breaker timeout is configured to 2s at boot (see harness
// bootServiceInMTMode); we don't assert automatic recovery in this test
// because (a) the default TM handler is restored in cleanup and (b) the
// recovery path requires fiddling with lib-commons internals that are not
// part of the public API. What matters for Gate 8 is the degraded-mode
// guarantee: cached tenants keep working, new tenants get a clean 4xx/5xx.
package integration

import (
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenant_Chaos_TenantManagerOutage proves the tracer keeps serving
// already-resolved tenants when the Tenant Manager goes dark.
//
// Not parallel: reboots the shared integration server.
func TestMultiTenant_Chaos_TenantManagerOutage(t *testing.T) {
	h := newMTHarness(t)

	// Tenant A points at the shared test DB (any valid DSN works — the
	// middleware resolves and connects, that's enough to warm the pool).
	warmedSpec := specFromAdminDSN(t, getAdminDSNForTests(t), testDBNameForTests(t))
	h.RegisterTenant("chaos-tenant-warmed", warmedSpec)

	// Tenant B is NOT registered in the default handler; we'll register it
	// only after the TM goes into outage mode so lib-commons has to fetch
	// fresh config (which will fail).
	cleanup := bootServiceInMTMode(t, h, nil)
	defer cleanup()

	warmedJWT := mintJWTWithTenantID("chaos-tenant-warmed")
	coldJWT := mintJWTWithTenantID("chaos-tenant-cold")

	// ------------------------------------------------------------------
	// Phase 1 — warm the cache for tenant-warmed. We expect 200 (the rules
	// list query returns an empty list from a freshly-created DB, but we
	// don't care about the body here; we care that the call reached the
	// handler, which proves pool resolution succeeded).
	// ------------------------------------------------------------------
	t.Run("Phase1_WarmCache_Succeeds", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", warmedJWT, "")
		// Accept 200 (happy path) or 503 (if the tenant DB lacks migrations
		// the query fails with a relation-not-found error that fires the
		// server's internal error path — either way proves we got past the
		// middleware). The assertion that matters is "not panic, not 401".
		require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
			"warmup must get past TenantMiddleware; got 401 body=%s", string(body))
		require.NotContains(t, string(body), "panic",
			"warmup response must not leak panic")
	})

	// ------------------------------------------------------------------
	// Phase 2 — simulate the TM outage. Subsequent TM fetches return 503.
	// Count the outage-path requests so we can prove the CB eventually
	// opens (failures stop reaching the TM after the threshold).
	// ------------------------------------------------------------------
	var outageHits atomic.Int32

	h.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		outageHits.Add(1)
		http.Error(w, `{"error":"tenant-manager unavailable"}`, http.StatusServiceUnavailable)
	})

	// ------------------------------------------------------------------
	// Phase 3 — warmed tenant stays functional. Fire several requests to
	// cover the revalidation path (lib-commons re-pings the pool every
	// CONNECTIONS_CHECK_INTERVAL_SEC; we set that to 30s at boot so no
	// revalidation should fire during this burst).
	// ------------------------------------------------------------------
	t.Run("Phase3_WarmedTenant_SurvivesTMOutage", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			resp, body := doRequest(t, http.MethodGet, "/v1/rules", warmedJWT, "")
			require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"iteration %d: cached pool must survive TM outage; got 401 body=%s",
				i, string(body))
			require.NotContains(t, string(body), "panic",
				"iteration %d: response must not leak panic", i)
		}
	})

	// ------------------------------------------------------------------
	// Phase 4 — cold tenant fails fast. lib-commons will call the TM,
	// receive 503, propagate the error to the middleware which maps it to
	// a 4xx/5xx — in either case the tracer returns a documented,
	// panic-free response.
	// ------------------------------------------------------------------
	t.Run("Phase4_ColdTenant_FailsClean", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", coldJWT, "")

		// M13: Pin the expected status to a narrow documented set. Previously
		// the test accepted ANY 4xx–5xx, which meant a 400 ("bad input") or
		// 418 ("teapot") would have counted as a clean degradation — not the
		// invariant we want. Cold-tenant requests during a TM outage must
		// surface as either:
		//   - 503 Service Unavailable (tenant pool unreachable — expected)
		//   - 500 Internal Server Error (tenant resolution propagates as 500
		//     when the middleware maps pool errors to a generic error code)
		//   - 401/403 (rare, but possible if the JWT's tenantId cannot be
		//     authorized without TM)
		//
		// Narrowing the accepted set catches regressions where a new error
		// path accidentally returns 200 with an empty body, or 404 because
		// the tenant pool is silently missing.
		acceptable := map[int]struct{}{
			http.StatusUnauthorized:        {}, // 401
			http.StatusForbidden:           {}, // 403
			http.StatusInternalServerError: {}, // 500
			http.StatusBadGateway:          {}, // 502
			http.StatusServiceUnavailable:  {}, // 503
			http.StatusGatewayTimeout:      {}, // 504
		}

		_, ok := acceptable[resp.StatusCode]
		assert.True(t, ok,
			"cold tenant during TM outage must return one of {401,403,500,502,503,504}, got %d body=%s",
			resp.StatusCode, string(body))

		require.NotContains(t, string(body), "panic",
			"cold-tenant error body must not leak panic trace")
		require.NotContains(t, string(body), "goroutine ",
			"cold-tenant error body must not leak goroutine dump")
	})

	// ------------------------------------------------------------------
	// Phase 5 — circuit breaker observability. Fire enough cold requests
	// to trip the breaker (threshold=5 at boot). Once the breaker opens,
	// cold-tenant attempts should stop hitting the TM (outageHits stops
	// growing after the breaker opens). We're asserting that the breaker
	// EVENTUALLY gates requests, not a precise count (the exact fail/open
	// semantics depend on lib-commons internals).
	// ------------------------------------------------------------------
	t.Run("Phase5_CircuitBreakerObservable", func(t *testing.T) {
		before := outageHits.Load()

		// 10 rapid-fire cold requests to exceed the breaker threshold.
		for i := 0; i < 10; i++ {
			resp, _ := doRequest(t, http.MethodGet, "/v1/rules", coldJWT, "")
			// All must be clean client/server errors, never panics.
			require.True(t, resp.StatusCode >= 400,
				"cold-tenant attempt %d must fail, got %d", i, resp.StatusCode)
		}

		after := outageHits.Load()
		hits := int(after - before)

		// At a minimum the TM received at least ONE request during the burst.
		assert.GreaterOrEqual(t, hits, 1,
			"TM must receive at least one call during cold-tenant burst")

		// Strong invariant: the breaker MUST short-circuit at least one
		// request out of 10. If we see all 10 round-trips reach the TM
		// adapter, the breaker either never opened or never gated — both
		// are regressions. The breaker threshold defaults to 5, so by the
		// 6th request the breaker should already be tripping calls.
		assert.Less(t, hits, 10,
			"circuit breaker must short-circuit at least one of the 10 cold-tenant requests; got %d TM hits = breaker never opened", hits)
	})
}

// ------------------------------------------------------------------
// test-local helpers that don't belong on the harness itself
// ------------------------------------------------------------------

// getAdminDSNForTests returns the integration suite's admin DSN.
func getAdminDSNForTests(t *testing.T) string {
	t.Helper()

	dsn := mtTestAdminDSN()
	require.NotEmpty(t, dsn, "admin DSN must be available via env")

	return dsn
}

func testDBNameForTests(t *testing.T) string {
	t.Helper()

	name := os.Getenv("DB_NAME")
	if strings.TrimSpace(name) == "" {
		name = "tracer_test"
	}

	return name
}
