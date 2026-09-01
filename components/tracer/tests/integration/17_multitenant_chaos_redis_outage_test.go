// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Gate 8 — Deliverable C: Redis outage chaos.
//
// The multi-tenant stack uses Redis ONLY for the tenant lifecycle Pub/Sub
// channel (OnTenantAdded / OnTenantRemoved). Redis is NOT on the request hot
// path: per-request tenant resolution goes straight to the tenant-manager
// HTTP API (which has its own cache) and the per-tenant postgres pool.
//
// This test encodes that invariant:
//
//  1. Boot with miniredis running.
//  2. Verify a request for tenant-A succeeds (TM + PG path, Redis untouched).
//  3. Shut down miniredis (mr.Close()).
//  4. Request for tenant-A continues to succeed — the listener's detached
//     goroutine errors internally but does not propagate to the request
//     handler.
//  5. A fresh tenant-B that was registered after the Redis outage also
//     resolves, proving the middleware's EnsureWorkers lazy-spawn covers the
//     missed OnTenantAdded Pub/Sub event.
//
// What we DO NOT assert here: reconnection. The lib-commons
// TenantEventListener is a black box — its reconnect strategy is internal and
// not observable via a public API. Rather than poke at unexported fields or
// sleep-and-pray, we document that recovery is "best-effort" and scope the
// test to the degraded-mode guarantee that matters for a live tracer cluster:
// Redis going down does not break in-flight requests.
package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenant_Chaos_RedisOutage proves Redis is not on the request hot
// path; an outage must not break tenant resolution or worker lazy-spawn.
//
// Not parallel: reboots the shared integration server.
func TestMultiTenant_Chaos_RedisOutage(t *testing.T) {
	h := newMTHarness(t)

	spec := specFromAdminDSN(t, getAdminDSNForTests(t), testDBNameForTests(t))
	h.RegisterTenant("redis-chaos-A", spec)

	cleanup := bootServiceInMTMode(t, h, nil)
	defer cleanup()

	jwtA := mintJWTWithTenantID("redis-chaos-A")

	// ------------------------------------------------------------------
	// Phase 1 — baseline. Confirm tenant-A resolves with Redis healthy.
	// ------------------------------------------------------------------
	t.Run("Phase1_Baseline_TenantAResolves", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtA, "")
		require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
			"baseline tenant-A resolution failed with 401 body=%s", string(body))
		require.NotContains(t, string(body), "panic",
			"baseline response must not leak panic")
	})

	// ------------------------------------------------------------------
	// Phase 2 — kill miniredis. The listener goroutine will see connection
	// errors; the tracer HTTP surface must remain functional.
	// ------------------------------------------------------------------
	h.ShutdownRedis()

	t.Run("Phase2_TenantAStillResolves_DuringRedisOutage", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtA, "")
			require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"iter %d: tenant-A must keep resolving during Redis outage; got 401 body=%s",
				i, string(body))
			require.NotContains(t, string(body), "panic",
				"iter %d: response must not leak panic", i)

			// The request must also not be a 5xx from the middleware — that
			// would mean Redis got onto the hot path somehow.
			assert.Less(t, resp.StatusCode, 500,
				"iter %d: tenant resolution must not 5xx when Redis is down; got %d body=%s",
				i, resp.StatusCode, string(body))
		}
	})

	// ------------------------------------------------------------------
	// Phase 3 — a NEW tenant registered with the TM during the Redis
	// outage must still resolve. This proves the middleware's
	// EnsureWorkers lazy-spawn covers the missed OnTenantAdded Pub/Sub
	// event that Redis would normally have delivered. The OnTenantAdded
	// event never fires (Redis is down), but the middleware short-circuits
	// through supervisor.EnsureWorkers on the first request.
	// ------------------------------------------------------------------
	h.RegisterTenant("redis-chaos-B", spec)

	jwtB := mintJWTWithTenantID("redis-chaos-B")

	t.Run("Phase3_LazySpawn_RecoversMissedPubSub", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtB, "")

		require.NotContains(t, string(body), "panic",
			"tenant-B lazy-spawn response must not leak panic")

		// Strong invariant: lazy-spawn must complete the round-trip with a
		// success status — anything else is a regression of the lazy-spawn
		// recovery path during Redis outage.
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"tenant-B must resolve via lazy-spawn path with HTTP 200; got %d body=%s",
			resp.StatusCode, string(body))
	})

	// ------------------------------------------------------------------
	// Phase 4 — a previously-unknown tenant that the TM does NOT know
	// about receives a clean error (404 / 4xx), not a panic. This guards
	// the "unknown tenant during Redis outage" matrix cell — the path
	// should be identical to the healthy-Redis case.
	// ------------------------------------------------------------------
	t.Run("Phase4_UnknownTenant_CleanError", func(t *testing.T) {
		jwtUnknown := mintJWTWithTenantID("redis-chaos-never-registered")

		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtUnknown, "")

		require.NotContains(t, string(body), "panic",
			"unknown-tenant error must not leak panic")

		// Strong invariant: an unknown tenant must produce a clean 4xx (the
		// auth/tenant-resolution layer's job). 5xx would mean a panic-adjacent
		// failure inside the resolution machinery — a regression even during
		// Redis outage, since this path does not depend on Redis.
		require.GreaterOrEqual(t, resp.StatusCode, 400,
			"unknown tenant must produce a client error; got %d", resp.StatusCode)
		require.Less(t, resp.StatusCode, 500,
			"unknown tenant must NOT 5xx (would indicate handler crash); got %d body=%s",
			resp.StatusCode, string(body))
	})

	// Note: we do NOT attempt to restart miniredis and assert the listener
	// reconnects. The reconnect path is internal to lib-commons' listener
	// and has no public observability hook — poking at unexported fields
	// would couple the test to library internals that legitimately change.
	// This test's value is proving the degradation contract: Redis is not
	// on the hot path, so an outage does not break the service. A follow-up
	// can add reconnect assertions if lib-commons exposes a status API.
}
