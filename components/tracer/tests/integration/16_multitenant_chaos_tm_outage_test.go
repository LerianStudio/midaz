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
//  5. Requests for tenant-B (never warmed) fail with the outage's own
//     answer — 500 while the Tenant Manager error propagates, 503 once the
//     circuit breaker opens — without leaking panic traces; the breaker fails
//     fast after a few attempts.
//
// The circuit breaker timeout is configured to 2s at boot (see harness
// bootServiceInMTMode); we don't assert automatic recovery in this test
// because (a) the default TM handler is restored in cleanup and (b) the
// recovery path requires fiddling with lib-commons internals that are not
// part of the public API. What matters for Gate 8 is the degraded-mode
// guarantee: cached tenants keep working, new tenants get a clean 500/503.
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
	// Phase 1 — warm the pool for tenant-warmed. 200 proves the tenant
	// resolved through the Tenant Manager and its pool served the query;
	// without it Phase 3 would prove nothing about surviving the outage.
	// ------------------------------------------------------------------
	t.Run("Phase1_WarmCache_Succeeds", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", warmedJWT, "")
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"warmup must resolve the tenant and serve the list; got %d body=%s",
			resp.StatusCode, string(body))
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
			require.Equal(t, http.StatusOK, resp.StatusCode,
				"iteration %d: cached pool must survive TM outage; got %d body=%s",
				i, resp.StatusCode, string(body))
		}
	})

	// ------------------------------------------------------------------
	// Phase 4 — cold tenant fails fast. lib-commons calls the TM, receives
	// 503 and, with the breaker still closed, answers 500 TENANT_DB_ERROR. A
	// reachable TM answers 404 for this never-registered tenant, so the
	// assertion fails unless the outage is what refused the request.
	// ------------------------------------------------------------------
	t.Run("Phase4_ColdTenant_FailsClean", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", coldJWT, "")

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode,
			"cold tenant during TM outage must return 500 before the breaker opens; body=%s", string(body))
		assert.Contains(t, string(body), "TENANT_DB_ERROR")
	})

	// ------------------------------------------------------------------
	// Phase 5 — circuit breaker. The breaker opens on the 5th consecutive
	// TM 5xx (threshold=5 at boot), which still answers 500; from then on
	// cold requests get 503 SERVICE_UNAVAILABLE without reaching the TM.
	// The open point is cold request 4 or 5 (the warmed tenant's first
	// cache hit may spend one failure on a background revalidation), so
	// only the burst's last answer is pinned.
	// ------------------------------------------------------------------
	t.Run("Phase5_CircuitBreakerObservable", func(t *testing.T) {
		before := outageHits.Load()

		var (
			resp *httpResponse
			body []byte
		)

		for i := 0; i < 10; i++ {
			resp, body = doRequest(t, http.MethodGet, "/v1/rules", coldJWT, "")
			require.Contains(t, []int{http.StatusInternalServerError, http.StatusServiceUnavailable}, resp.StatusCode,
				"cold-tenant attempt %d must fail with the outage's 500/503; body=%s", i, string(body))
		}

		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
			"the open breaker must answer the last cold request with 503; body=%s", string(body))
		assert.Contains(t, string(body), "SERVICE_UNAVAILABLE")

		hits := int(outageHits.Load() - before)

		assert.GreaterOrEqual(t, hits, 1,
			"TM must receive at least one call during cold-tenant burst")
		// 5 failures open the breaker and Phase 4 spent one: at most 4 reach the TM here.
		assert.LessOrEqual(t, hits, 4,
			"circuit breaker must stop TM calls after the threshold; got %d TM hits", hits)
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
