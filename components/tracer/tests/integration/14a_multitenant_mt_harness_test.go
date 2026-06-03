// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Package integration hosts the end-to-end multi-tenant tests for Gate 8
// (Deliverables A-D). This file contains shared helpers used by
// 15/16/17/18_multitenant_*_test.go; the tests themselves live in the numbered
// files next to it.
//
// Harness at a glance
// -------------------
//  1. We spin up a fake Tenant Manager via httptest that answers exactly the
//     two endpoints lib-commons v4 calls: GET /v1/tenants/active?service=tracer
//     and GET /v1/tenants/{tenantID}/associations/{service}/connections.
//  2. We spin up miniredis to back the lib-commons Redis Pub/Sub listener.
//  3. We reboot the tracer service through RestartServerWithConfig with
//     MULTI_TENANT_ENABLED=true and the fake endpoints pointed at us.
//  4. We mint unsigned JWTs (lib-commons parses with ParseUnverified) that
//     carry a tenantId claim, and exercise /v1/rules to stress the
//     TenantMiddleware path.
//
// The read-only scope of the MT isolation test in
// 15_multitenant_isolation_test.go is a deliberate scoping choice (read paths
// already flow through the per-tenant pgManager via pgdb.TxBeginnerAdapter,
// which is context-aware). The tests in this file focus on tenant resolution,
// JWT claim validation, and the per-tenant pool selection that drives both
// reads and writes.
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	testutil_integration "tracer/internal/testutil_integration"
)

// mtHarness bundles the fake infrastructure a multi-tenant test needs. All
// resources are cleaned up via t.Cleanup so callers never have to unwind
// manually.
type mtHarness struct {
	tmServer       *httptest.Server
	pluginAuthSrv  *httptest.Server // fake plugin-auth endpoint, always approves
	miniRedis      *miniredis.Miniredis
	tenantsMu      sync.RWMutex
	tenants        map[string]tenantPGSpec // tenantID -> where its PostgreSQL lives
	activeService  string                  // echoed back under /v1/tenants/active
	tmHandler      http.HandlerFunc        // override knob for chaos tests
	requestCount   int                     // observability for tests
	requestCountMu sync.Mutex
}

// tenantPGSpec describes the PostgreSQL connection details the fake Tenant
// Manager should hand back for a given tenantID. In tests we always point
// every tenant at the shared test container — isolation is simulated via
// per-tenant database names created up-front.
type tenantPGSpec struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	Schema   string
	SSLMode  string
}

// newMTHarness creates a fake TM + miniredis pair. Callers can register
// tenants via registerTenant before calling apply().
func newMTHarness(t *testing.T) *mtHarness {
	t.Helper()

	mr := miniredis.RunT(t)

	h := &mtHarness{
		miniRedis:     mr,
		tenants:       make(map[string]tenantPGSpec),
		activeService: "tracer",
	}

	// Default handler — unit tests can override via SetHandler for chaos.
	h.tmHandler = h.defaultHandler

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.requestCountMu.Lock()
		h.requestCount++
		h.requestCountMu.Unlock()

		h.tmHandler(w, r)
	}))
	h.tmServer = srv

	t.Cleanup(func() {
		srv.Close()
	})

	// Plugin-auth approver: lib-auth's middleware posts to {addr}/v1/authorize
	// with {sub, resource, action} and expects back {authorized, timestamp}.
	// MT tests are security tests for TENANT isolation, not AUTH — so we
	// approve unconditionally. The fail-fast added in
	// ValidateMultiTenantConfig still prevents production from booting
	// without a real plugin-auth endpoint because bootstrap also enforces
	// PLUGIN_AUTH_ADDRESS != "".
	pluginAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/authorize" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorized": true,
			"timestamp":  time.Now().UTC(),
		})
	}))
	h.pluginAuthSrv = pluginAuth

	t.Cleanup(func() {
		pluginAuth.Close()
	})

	return h
}

// defaultHandler implements the tiny slice of the TM HTTP surface lib-commons
// actually calls:
//
//   - GET /v1/tenants/active?service=<name>          → list of active tenants
//   - GET /v1/tenants/{id}/associations/{svc}/connections → one tenant config
func (h *mtHarness) defaultHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/v1/tenants/active":
		h.writeActiveList(w, r)
		return
	case strings.HasPrefix(path, "/v1/tenants/") && strings.HasSuffix(path, "/connections"):
		h.writeTenantConfig(w, r)
		return
	default:
		http.NotFound(w, r)
	}
}

func (h *mtHarness) writeActiveList(w http.ResponseWriter, _ *http.Request) {
	h.tenantsMu.RLock()
	defer h.tenantsMu.RUnlock()

	type summary struct {
		TenantID   string `json:"id"`
		TenantSlug string `json:"tenantSlug"`
		Status     string `json:"status"`
	}

	out := make([]summary, 0, len(h.tenants))
	for id := range h.tenants {
		out = append(out, summary{TenantID: id, TenantSlug: id, Status: "active"})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// writeTenantConfig returns a flat TenantConfig with a single module named
// after the service; the module name is irrelevant to the single-manager
// path tracer wires in routes.go (which is what we exercise).
func (h *mtHarness) writeTenantConfig(w http.ResponseWriter, r *http.Request) {
	// URL shape: /v1/tenants/{tenantID}/associations/{service}/connections
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/tenants/"), "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}

	tenantID := parts[0]

	h.tenantsMu.RLock()
	spec, ok := h.tenants[tenantID]
	h.tenantsMu.RUnlock()

	if !ok {
		// 404 is the domain-defined "tenant not found" signal; lib-commons
		// maps it to a business error, not a CB failure.
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	// The flat format lib-commons v4 expects: databases keyed by module.
	resp := map[string]any{
		"id":            tenantID,
		"tenantSlug":    tenantID,
		"service":       "tracer",
		"status":        "active",
		"isolationMode": "isolated",
		"databases": map[string]any{
			"tracer": map[string]any{
				"postgresql": map[string]any{
					"host":     spec.Host,
					"port":     spec.Port,
					"database": spec.Database,
					"username": spec.Username,
					"password": spec.Password,
					"schema":   spec.Schema,
					"sslmode":  spec.SSLMode,
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// RegisterTenant tells the fake TM where this tenant's database lives. When
// DSN is "", the tenant is "known" but not yet provisioned — the TM returns
// the registered spec and lib-commons attempts to connect.
func (h *mtHarness) RegisterTenant(tenantID string, spec tenantPGSpec) {
	h.tenantsMu.Lock()
	defer h.tenantsMu.Unlock()
	h.tenants[tenantID] = spec
}

// SetHandler swaps the TM HTTP handler at runtime (used by chaos tests that
// want to force 5xx/timeouts without closing the server).
func (h *mtHarness) SetHandler(fn http.HandlerFunc) {
	if fn == nil {
		h.tmHandler = h.defaultHandler
		return
	}

	h.tmHandler = fn
}

// RedisAddr returns host, port for miniredis.
func (h *mtHarness) RedisAddr(t *testing.T) (string, string) {
	t.Helper()

	parts := strings.Split(h.miniRedis.Addr(), ":")
	require.Len(t, parts, 2, "miniredis addr must be host:port")

	return parts[0], parts[1]
}

// URL returns the fake TM base URL (no trailing slash).
func (h *mtHarness) URL() string {
	return h.tmServer.URL
}

// CloseTM simulates a TM outage. Tests that need to reopen the server
// later must stand up a new harness (httptest servers cannot be reopened).
func (h *mtHarness) CloseTM() {
	h.tmServer.Close()
}

// ShutdownRedis stops miniredis. Callers can later call StartRedis to bring
// it back on the same port.
func (h *mtHarness) ShutdownRedis() {
	h.miniRedis.Close()
}

// ------------------------------------------------------------------
// JWT helpers
// ------------------------------------------------------------------

// mintJWTWithTenantID returns a JWT that encodes the given tenantId claim.
// The token is unsigned (alg=none) because lib-commons v4 TenantMiddleware
// uses jwt.ParseUnverified — it trusts the upstream auth layer to validate
// signatures. Tests therefore do not need any shared secret.
//
// `sub` is REQUIRED by the strict-sub policy in auth_guard.extractPrincipalFromBearer
// (introduced as part of the Taura audit fix): any JWT that parses but lacks `sub`
// is rejected with 401 TRC-0350 before reaching lib-auth. Tests mint a synthetic
// sub derived from tenantID so each tenant produces a distinct (and attributable)
// principal in the audit trail.
func mintJWTWithTenantID(tenantID string) string {
	base := testutil.FixedTime()
	claims := jwt.MapClaims{
		"sub":      "test-user-" + tenantID,
		"tenantId": tenantID,
		"iat":      base.Unix(),
		"exp":      base.Add(1 * time.Hour).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)

	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		panic(fmt.Sprintf("mint JWT: %v", err))
	}

	return signed
}

// mintJWTWithoutTenantID returns a JWT that has no tenantId claim. Used by
// Deliverable D to prove missing-claim rejection.
func mintJWTWithoutTenantID() string {
	base := testutil.FixedTime()
	claims := jwt.MapClaims{
		"sub": "test-user",
		"iat": base.Unix(),
		"exp": base.Add(1 * time.Hour).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)

	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		panic(fmt.Sprintf("mint JWT: %v", err))
	}

	return signed
}

// mintJWTWithEmptyTenantID returns a JWT with tenantId="" — distinct from the
// absent-claim case because lib-commons treats "" as a missing claim by
// different code paths.
func mintJWTWithEmptyTenantID() string {
	base := testutil.FixedTime()
	claims := jwt.MapClaims{
		"tenantId": "",
		"sub":      "test-user",
		"iat":      base.Unix(),
		"exp":      base.Add(1 * time.Hour).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)

	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		panic(fmt.Sprintf("mint JWT: %v", err))
	}

	return signed
}

// ------------------------------------------------------------------
// Server reboot with MT enabled
// ------------------------------------------------------------------

// bootServiceInMTMode reboots the integration suite's server with
// MULTI_TENANT_ENABLED=true and points it at the given harness. Returns a
// cleanup function the caller MUST defer. The reboot is sequential with other
// MT tests in this package because it mutates the process-global env.
func bootServiceInMTMode(t *testing.T, h *mtHarness, extra map[string]string) func() {
	t.Helper()

	redisHost, redisPort := h.RedisAddr(t)

	env := map[string]string{
		"MULTI_TENANT_ENABLED":         "true",
		"MULTI_TENANT_URL":             h.URL(),
		"MULTI_TENANT_SERVICE_API_KEY": "test-svc-api-key",
		"MULTI_TENANT_REDIS_HOST":      redisHost,
		"MULTI_TENANT_REDIS_PORT":      redisPort,
		// Integration Redis (miniredis / test container) is plaintext.
		// H14 defaults MULTI_TENANT_REDIS_TLS=true when the env var is unset;
		// flip it back to false explicitly so the bootstrap reaches the
		// non-TLS miniredis address. Production keeps this true.
		"MULTI_TENANT_REDIS_TLS":                   "false",
		"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD":   "5",
		"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC": "2", // short for chaos tests
		"MULTI_TENANT_TIMEOUT":                     "5",
		// MAX_TENANT_POOLS shrunk from production-style 100 → 4 for tests.
		// The integration suite registers at most 2-3 tenants per test, and
		// each pool opens primary+replica × MaxOpenConns. Larger caps let
		// repeated RestartServerWithConfig reboots accumulate hundreds of
		// connections against the testcontainer's max_connections=100.
		"MULTI_TENANT_MAX_TENANT_POOLS": "4",
		// Per-tenant pool sizing: 3 max-open × 2 (primary+replica) × 4 tenants
		// = 24 connections worst case per service instance, leaving generous
		// headroom under max_connections=100 even with stale conns from a
		// previous reboot still draining.
		"MULTI_TENANT_MAX_OPEN_CONNS_PER_TENANT":      "3",
		"MULTI_TENANT_MAX_IDLE_CONNS_PER_TENANT":      "1",
		"MULTI_TENANT_IDLE_TIMEOUT_SEC":               "300",
		"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC": "30",
		"MULTI_TENANT_CACHE_TTL_SEC":                  "120",
		// MT requires plugin auth for JWT signature verification. The
		// default integration suite runs with PLUGIN_AUTH_ENABLED=false
		// (single-tenant canary), but the MT reboots MUST flip this on or
		// ValidateMultiTenantConfig rejects the boot with a security error.
		// PLUGIN_AUTH_ADDRESS is intentionally left unset — lib-auth's
		// client treats that as "no upstream", which is compatible with
		// the httptest-based TenantMiddleware exercised here.
		"PLUGIN_AUTH_ENABLED":             "true",
		"PLUGIN_AUTH_ADDRESS":             h.pluginAuthSrv.URL,
		"API_KEY_ENABLED_ONLY_VALIDATION": "false",
		// The harness exposes http:// URLs (local httptest servers); opt in to
		// the cleartext-HTTP downgrade explicitly per H13. Production must keep
		// this false so cleartext MULTI_TENANT_URL is rejected.
		"MULTI_TENANT_ALLOW_INSECURE_HTTP": "true",
		// MT integration tests exercise both sync and cleanup workers under
		// chaos — keep cleanup enabled so the supervisor spawns the per-tenant
		// cleanup worker alongside the sync worker (H8 respects the flag).
		"CLEANUP_WORKER_ENABLED": "true",
		"CLEANUP_INTERVAL_HOURS": "24",
	}

	for k, v := range extra {
		env[k] = v
	}

	cleanup, err := testutil_integration.RestartServerWithConfig(env)
	require.NoError(t, err, "reboot service with MT=true")

	return func() {
		if err := cleanup(); err != nil {
			t.Logf("MT cleanup: %v (best-effort)", err)
		}
	}
}

// ------------------------------------------------------------------
// DB helpers for the two-tenant isolation test
// ------------------------------------------------------------------

// ensureTenantDatabase creates a CREATE DATABASE (if not exists) against the
// postgres container managed by the integration suite. Returns a tenantPGSpec
// pointing at that database so the fake TM can hand it back to lib-commons.
//
// IMPORTANT: we do NOT run tracer's migrations against these additional DBs.
// The test suite for Deliverable A only asserts tenant resolution — it does
// not round-trip CRUD through the tenant-specific DB. See the comments in
// 18_multitenant_isolation_test.go for the scope reduction rationale.
func ensureTenantDatabase(t *testing.T, dbName string) tenantPGSpec {
	t.Helper()

	adminDSN := testutil.GetTestDSN()

	db, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err, "open admin connection")
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Postgres does not support "CREATE DATABASE IF NOT EXISTS"; check first.
	var exists bool
	err = db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, dbName,
	).Scan(&exists)
	require.NoError(t, err, "check database existence")

	if !exists {
		// pg_ident does not allow parameterised identifier, so quote manually.
		// dbName comes from the test code (never external input), so the
		// gosec warning is safe to suppress.
		//nolint:gosec // G201: dbName is test-controlled, not user input
		_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName))
		require.NoError(t, err, "create tenant database %s", dbName)
	}

	return specFromAdminDSN(t, adminDSN, dbName)
}

// specFromAdminDSN parses the admin DSN to extract host/port/credentials and
// returns a tenantPGSpec targeting dbName.
func specFromAdminDSN(t *testing.T, dsn, dbName string) tenantPGSpec {
	t.Helper()

	u, err := url.Parse(dsn)
	require.NoError(t, err, "parse DSN: %s", dsn)

	host := u.Hostname()
	portStr := u.Port()
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err, "parse port %q", portStr)

	user := u.User.Username()
	pass, _ := u.User.Password()

	q := u.Query()
	sslmode := q.Get("sslmode")

	if sslmode == "" {
		sslmode = "disable"
	}

	return tenantPGSpec{
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: user,
		Password: pass,
		Schema:   "public",
		SSLMode:  sslmode,
	}
}

// ------------------------------------------------------------------
// HTTP helpers
// ------------------------------------------------------------------

// mtTestAdminDSN reconstructs the admin DSN from the integration harness env
// vars. testutil.GetTestDSN would also work but lives behind a helper that
// hard-codes sslmode=disable; replicating inline keeps the test files close to
// the actual boot-time values the suite uses.
func mtTestAdminDSN() string {
	host := envWithFallback("DB_HOST", "127.0.0.1")
	port := envWithFallback("DB_PORT", "5432")
	user := envWithFallback("DB_USER", "tracer")
	pass := envWithFallback("DB_PASSWORD", "tracer")
	db := envWithFallback("DB_NAME", "tracer_test")

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, db)
}

func envWithFallback(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	return v
}

// doRequest issues an HTTP call against the currently-running integration
// server. It attaches X-API-Key (so AuthGuard passes) and optionally a bearer
// JWT (so TenantMiddleware can parse the tenantId).
//
// Returns (statusCode, body). The previous signature returned *http.Response
// + []byte, which led to confusion because the body was already drained and
// closed — callers could not re-read it (M8). (int, []byte) is the minimal
// surface the tests actually use.
type httpResponse struct {
	StatusCode int
	Header     http.Header
}

func doRequest(t *testing.T, method, path, jwt, body string) (*httpResponse, []byte) {
	t.Helper()

	baseURL := testutil.GetBaseURL()

	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	var req *http.Request
	var err error

	if reqBody == nil {
		req, err = http.NewRequest(method, baseURL+path, nil)
	} else {
		req, err = http.NewRequest(method, baseURL+path, reqBody)
	}
	require.NoError(t, err, "build %s %s", method, path)

	req.Header.Set("X-API-Key", testutil.GetAPIKey())
	req.Header.Set("Content-Type", "application/json")

	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err, "execute %s %s", method, path)

	// Read the whole body instead of capping at 4KB. The previous impl used
	// a fixed 4KB buffer with a single Read() whose error was discarded;
	// that made any NotContains assertion in the isolation tests unreliable
	// because a data leak past byte 4096 would be invisible. io.ReadAll is
	// the right primitive for test bodies (bounded by the integration
	// server's own response-size limits).
	respBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, readErr, "doRequest: read body for %s %s", method, path)

	return &httpResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
	}, respBody
}
