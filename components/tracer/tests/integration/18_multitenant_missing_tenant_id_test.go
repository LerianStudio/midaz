// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Gate 8 — Deliverable D: missing tenantId claim in JWT.
//
// When multi-tenant mode is enabled, the /v1 route group is protected by the
// lib-commons TenantMiddleware. The middleware extracts tenantId from the
// Authorization Bearer JWT; if the header is missing OR the claim is
// absent/empty, the request MUST be rejected cleanly — no 500, no panic, no
// stack-trace leak.
//
// This test reboots the integration server in multi-tenant mode and exercises
// the three negative cases: no JWT, JWT without tenantId claim, JWT with
// empty-string tenantId claim.
package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenant_MissingTenantIDRejected proves the tenant middleware rejects
// requests that lack a usable tenantId in a clean, documented way.
//
// Ring standard "Testing Multi-Tenant Code" requires error-path coverage for
// missing tenant claims; this test is that coverage at the integration layer.
//
// Not parallel: reboots the shared integration server.
func TestMultiTenant_MissingTenantIDRejected(t *testing.T) {
	h := newMTHarness(t)

	// Register one tenant so the active-tenants sync at boot has something to
	// return — not strictly required, but a realistic starting state.
	h.RegisterTenant("t-tenant-a", tenantPGSpec{
		Host: "127.0.0.1", Port: 65535, Database: "unreachable",
		Username: "u", Password: "p", SSLMode: "disable",
	})

	cleanup := bootServiceInMTMode(t, h, nil)
	defer cleanup()

	// ------------------------------------------------------------------
	// Case 1: no Authorization header at all.
	//
	// The AuthGuard itself allows an API key-only request for /v1 routes, so
	// the call reaches the TenantMiddleware, which then demands a JWT. Expect
	// 401 (MISSING_TOKEN).
	// ------------------------------------------------------------------
	t.Run("NoAuthorizationHeader_401", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", "", "")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"request without bearer JWT must return 401, got %d: %s",
			resp.StatusCode, string(body))

		// Clean error — no stack trace, no 5xx.
		require.NotContains(t, string(body), "panic",
			"response body must not leak a panic trace: %s", string(body))
		require.NotContains(t, string(body), "goroutine ",
			"response body must not leak a goroutine dump: %s", string(body))
	})

	// ------------------------------------------------------------------
	// Case 2: JWT present but carries no tenantId claim.
	//
	// lib-commons ParseUnverified succeeds, claims cast succeeds, but the
	// tenantId lookup returns "". The middleware path is
	// ErrMissingTenantIDClaim → MISSING_TENANT → 401.
	// ------------------------------------------------------------------
	t.Run("JWT_WithoutTenantIDClaim_401", func(t *testing.T) {
		jwt := mintJWTWithoutTenantID()

		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwt, "")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"JWT without tenantId must return 401, got %d: %s",
			resp.StatusCode, string(body))

		require.NotContains(t, string(body), "panic",
			"response must not leak panic info")

		// The body should carry a recognisable client-error shape, not a 5xx
		// fallback. We don't pin the exact code string because lib-commons
		// owns the wire format — the guarantee is "4xx, not 5xx".
		assert.Less(t, resp.StatusCode, 500,
			"missing tenantId must be a client error, not 5xx")
	})

	// ------------------------------------------------------------------
	// Case 3: JWT with explicit empty tenantId claim.
	//
	// This guards against a different code path inside lib-commons — the
	// claims map DOES contain "tenantId", but its value is "". The middleware
	// must still reject, not proceed with an empty tenant key into the pg
	// manager (which would otherwise explode with "tenant ID is required").
	// ------------------------------------------------------------------
	t.Run("JWT_WithEmptyTenantIDClaim_401", func(t *testing.T) {
		jwt := mintJWTWithEmptyTenantID()

		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwt, "")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"JWT with empty tenantId must return 401, got %d: %s",
			resp.StatusCode, string(body))

		require.NotContains(t, string(body), "panic",
			"response must not leak panic info")

		assert.Less(t, resp.StatusCode, 500,
			"empty tenantId must be a client error, not 5xx")
	})

	// ------------------------------------------------------------------
	// Case 4: malformed Authorization header (non-JWT).
	//
	// Covers the parse-failure path; lib-commons returns INVALID_TOKEN, still
	// 401. Primarily a regression guard — we've seen services crash on
	// malformed bearer headers in the past.
	// ------------------------------------------------------------------
	t.Run("MalformedBearerToken_401", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", "not.a.jwt", "")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"malformed bearer must return 401, got %d: %s",
			resp.StatusCode, string(body))

		require.NotContains(t, string(body), "panic")
		assert.Less(t, resp.StatusCode, 500)
	})
}
