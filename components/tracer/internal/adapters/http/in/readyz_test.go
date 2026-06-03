// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/api"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// createReadyzTestApp registers /readyz against the supplied HealthChecker
// and threads a tracer into the request context, mirroring production
// middleware order. Tests never go through the full NewRoutes() pipeline so
// the rest of the middleware (auth, CORS, telemetry) is intentionally absent.
func createReadyzTestApp(hc *HealthChecker) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx = libObservability.ContextWithTracer(ctx, otel.Tracer("tracer-readyz-test"))
		c.SetUserContext(ctx)

		return c.Next()
	})
	app.Get("/readyz", hc.ReadyzHandler())

	return app
}

// newReadyzCheckerWithDB builds a HealthChecker pre-wired with a sqlmock DB
// that pings successfully. The returned cleanup must be deferred by the test.
func newReadyzCheckerWithDB(t *testing.T, version, deploymentMode string) (*HealthChecker, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	provider := NewMockPostgresDBProvider(ctrl)
	provider.EXPECT().IsConnected().Return(true).AnyTimes()
	provider.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	hc := NewTestableHealthCheckerWithMeta(provider, version, deploymentMode)

	cleanup := func() {
		// Register the close expectation AFTER the test has run so the
		// queued ExpectPing/etc. set up by the test were already consumed in
		// order. Then close, then verify all expectations were met.
		// Without the ExpectationsWereMet() check, a test could stop hitting
		// PingContext (or any other expected DB call) and still pass — a
		// silent contract regression.
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}

	return hc, mock, cleanup
}

// TestReadyzHandler_AllUp_Returns200WithHealthyShape asserts the canonical
// healthy-shape contract: 200, top-level "healthy", every check has a status
// from the closed vocabulary, and version + deployment_mode are echoed back.
//
// The single-tenant /readyz cycle returns exactly two checks: postgres +
// rule_cache. tenant_manager/tenant_pubsub were stripped at Gate-strip.
func TestReadyzHandler_AllUp_Returns200WithHealthyShape(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.2.3", "saas")
	defer cleanup()

	mock.ExpectPing()

	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: 5 * time.Second, size: 7})
	// Wire postgres TLS detection: sslmode=require ⇒ tls=true.
	hc.SetPostgresDSN("host=localhost user=tracer password=secret dbname=tracer port=5432 sslmode=require")
	hc.SetPostgresTLSDetector(stubPostgresTLSDetector(true, nil))

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "1.2.3", response.Version)
	assert.Equal(t, "saas", response.DeploymentMode)
	assert.False(t, response.Draining, "draining must be false (and omitted from JSON) in normal operation")

	// Exactly two checks — postgres + rule_cache. No tenant_* checks.
	assert.Len(t, response.Checks, 2, "single-tenant /readyz cycle returns exactly two checks")

	// Postgres must be present and "up", with tls=true populated by the detector.
	pg, ok := response.Checks["postgres"]
	require.True(t, ok, "postgres check missing")
	assert.Equal(t, StatusUp, pg.Status)
	require.NotNil(t, pg.TLS, "postgres tls field must be populated when DSN + detector are wired")
	assert.True(t, *pg.TLS, "sslmode=require must surface as tls=true")

	// rule_cache: ready ⇒ "up". TLS is omitted (in-process).
	rc, ok := response.Checks["rule_cache"]
	require.True(t, ok, "rule_cache check missing")
	assert.Equal(t, StatusUp, rc.Status)
	assert.Nil(t, rc.TLS, "rule_cache has no TLS concept — field must be omitted")

	// tenant_manager + tenant_pubsub must NOT appear in the response.
	_, hasTM := response.Checks["tenant_manager"]
	assert.False(t, hasTM, "tenant_manager check must not appear in single-tenant /readyz response")

	_, hasPS := response.Checks["tenant_pubsub"]
	assert.False(t, hasPS, "tenant_pubsub check must not appear in single-tenant /readyz response")
}

// stubPostgresTLSDetector returns a function-form detector that ignores its
// DSN argument and returns the supplied (bool, error) pair. Lets readyz
// tests drive the TLS branches without crafting full DSNs each time.
func stubPostgresTLSDetector(tls bool, err error) func(string) (bool, error) {
	return func(string) (bool, error) {
		return tls, err
	}
}

// TestReadyzHandler_PostgresDown_Returns503WithDownStatus asserts the
// aggregation rule: a single "down" check forces 503 and top-level
// "unhealthy", regardless of any other check.
func TestReadyzHandler_PostgresDown_Returns503WithDownStatus(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	provider := NewMockPostgresDBProvider(ctrl)
	provider.EXPECT().IsConnected().Return(false)

	hc := NewTestableHealthCheckerWithMeta(provider, "1.2.3", "local")
	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: time.Second, size: 1})

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "unhealthy", response.Status)

	pg := response.Checks["postgres"]
	assert.Equal(t, StatusDown, pg.Status)
	assert.NotEmpty(t, pg.Error, "down checks must surface an operator-facing error")
}

// TestReadyzHandler_RuleCacheStale_Returns503WithDegradedStatus
// is the documented behavior change: stale rule_cache returns 503 with
// "degraded" (was 200 + "DEGRADED" before).
func TestReadyzHandler_RuleCacheStale_Returns503WithDegradedStatus(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "local")
	defer cleanup()

	mock.ExpectPing()

	// Cache ready but staleness exceeds threshold ⇒ "degraded"
	hc.SetCacheHealthProvider(&mockCacheHealth{
		ready:     true,
		staleness: 10 * time.Minute,
		size:      3,
	})

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"degraded must aggregate to 503 per canonical contract")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "unhealthy", response.Status)

	rc := response.Checks["rule_cache"]
	assert.Equal(t, StatusDegraded, rc.Status)
}

// TestReadyzHandler_Draining_Returns503EvenIfAllDepsUp asserts the SIGTERM
// drain branch: once MarkDraining() is called, /readyz returns 503 with a
// draining-flagged response, regardless of dep health. The canonical
// contract requires the per-dep `checks` map to STILL be populated while
// draining — operators must see what looked healthy at the moment drain
// started; the `draining: true` flag is the signal to withhold traffic.
func TestReadyzHandler_Draining_Returns503EvenIfAllDepsUp(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "local")
	defer cleanup()

	// Probes still run during drain — per-dep timeouts bound the work and
	// the canonical contract requires populated `checks`.
	mock.ExpectPing()

	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: time.Second, size: 1})
	hc.MarkDraining()

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "unhealthy", response.Status)
	assert.True(t, response.Draining, "draining flag must be true once MarkDraining() is called")

	// Canonical-shape preservation: the per-dep checks map must be populated
	// during drain, NOT empty. Operators rely on this to see snapshot health
	// at drain time.
	assert.Len(t, response.Checks, 2,
		"drain must preserve canonical checks shape (postgres + rule_cache)")

	pg, ok := response.Checks["postgres"]
	require.True(t, ok, "postgres check must be present during drain")
	assert.Equal(t, StatusUp, pg.Status, "postgres probe still ran and reported up")

	rc, ok := response.Checks["rule_cache"]
	require.True(t, ok, "rule_cache check must be present during drain")
	assert.Equal(t, StatusUp, rc.Status, "rule_cache probe still ran and reported up")

	assert.Equal(t, "1.0.0", response.Version, "version must be echoed during drain")
	assert.Equal(t, "local", response.DeploymentMode, "deployment_mode must be echoed during drain")
}

// TestReadyzHandler_VersionAndDeploymentMode_PresentInResponse asserts that
// the version + deployment_mode fields are sourced from the HealthChecker's
// configuration (i.e. cfg.OtelServiceVersion + cfg.DeploymentMode at
// bootstrap time), not hardcoded.
func TestReadyzHandler_VersionAndDeploymentMode_PresentInResponse(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "9.9.9-rc1", "byoc")
	defer cleanup()

	mock.ExpectPing()
	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "9.9.9-rc1", response.Version)
	assert.Equal(t, "byoc", response.DeploymentMode)
}

// TestReadyzHandler_AggregationRule_AnyDownOrDegradedReturns503 is a
// table-driven enforcement of the aggregation rule: every combination
// containing at least one "down" or "degraded" check must return 503.
func TestReadyzHandler_AggregationRule_AnyDownOrDegradedReturns503(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		setup     func(hc *HealthChecker, mock sqlmock.Sqlmock)
		expectErr bool
	}{
		{
			name: "rule_cache not ready ⇒ 503",
			setup: func(hc *HealthChecker, mock sqlmock.Sqlmock) {
				mock.ExpectPing()
				hc.SetCacheHealthProvider(&mockCacheHealth{ready: false})
			},
			expectErr: true,
		},
		{
			name: "rule_cache stale ⇒ 503 (degraded)",
			setup: func(hc *HealthChecker, mock sqlmock.Sqlmock) {
				mock.ExpectPing()
				hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: 1 * time.Hour})
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "saas")
			defer cleanup()

			tt.setup(hc, mock)

			app := createReadyzTestApp(hc)
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
				"any down/degraded must aggregate to 503")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response api.ReadyzResponse
			require.NoError(t, json.Unmarshal(body, &response))
			assert.Equal(t, "unhealthy", response.Status)
		})
	}
}

// TestReadyzHandler_AggregationRule_AllUpIsHealthy asserts the inverse rule:
// when all checks are "up", aggregation is "healthy" (200).
func TestReadyzHandler_AggregationRule_AllUpIsHealthy(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "saas")
	defer cleanup()

	mock.ExpectPing()
	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: time.Second, size: 1})

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "all up must aggregate to 200/healthy")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusUp, response.Checks["postgres"].Status)
	assert.Equal(t, StatusUp, response.Checks["rule_cache"].Status)
}

// TestReadyzHandler_TLSField_OmittedForRuleCache asserts the Gate 3 contract
// for the `tls` field across both probes:
//
//   - postgres with sslmode=disable ⇒ tls=false (NOT nil — operator
//     explicitly configured "no TLS")
//   - rule_cache (in-process) ⇒ tls omitted (nil pointer in struct)
func TestReadyzHandler_TLSField_OmittedForRuleCache(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "saas")
	defer cleanup()

	mock.ExpectPing()

	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})
	hc.SetPostgresDSN("host=localhost user=tracer password=secret dbname=tracer port=5432 sslmode=disable")
	hc.SetPostgresTLSDetector(stubPostgresTLSDetector(false, nil))

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	pg := response.Checks["postgres"]
	require.NotNil(t, pg.TLS, "postgres tls=false must be explicit, not omitted")
	assert.False(t, *pg.TLS, "sslmode=disable must surface as tls=false")

	rc := response.Checks["rule_cache"]
	assert.Nil(t, rc.TLS, "rule_cache has no TLS concept — field must be omitted (nil)")
}

// TestReadyzHandler_TLSDetectorParseError_OmitsTLSField asserts that when the
// postgres TLS detector returns a non-nil error (e.g. malformed DSN), the
// probe still reports up/down based on the actual liveness check — only the
// `tls` field is omitted. Misconfigurations in posture detection must NOT
// take down a healthy service.
func TestReadyzHandler_TLSDetectorParseError_OmitsTLSField(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.0.0", "saas")
	defer cleanup()

	mock.ExpectPing()

	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})
	hc.SetPostgresDSN("postgres://invalid%ZZ@host/db")
	hc.SetPostgresTLSDetector(stubPostgresTLSDetector(false, errors.New("parse url: invalid URL escape")))

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"TLS posture parse failure must NOT flip the probe to down")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Nil(t, response.Checks["postgres"].TLS, "postgres tls must be omitted on parse error")
}

// TestReadyzHandler_RuleCacheNotReady_ReportsDown is a regression guard for
// the canonical single-tenant cache mapping: cacheHealth.IsReady=false ⇒
// rule_cache "down". Stripped MT branch must never re-emerge.
func TestReadyzHandler_RuleCacheNotReady_ReportsDown(t *testing.T) {
	t.Parallel()

	testutil.SetupTestTracing(t)

	hc := NewTestableHealthChecker(nil)
	hc.SetCacheHealthProvider(&mockCacheHealth{
		ready:     false,
		staleness: time.Duration(math.MaxInt64),
	})

	ctx := libObservability.ContextWithTracer(context.Background(), otel.Tracer("tracer-test"))
	check := hc.probeReadyzRuleCache(ctx)

	assert.Equal(t, StatusDown, check.Status,
		"single-tenant /readyz cycle reports rule_cache 'down' when IsReady=false")
}

// TestReadyzHandler_RuleCacheLatencyMs_AlwaysPopulated pins the contract that
// every rule_cache probe response carries a measured LatencyMs (computed from
// time.Since(start).Milliseconds()), regardless of the up/down/degraded
// outcome. For an in-process cache the value almost always rounds to 0; the
// assertion is `>= 0`, not `== 0`, because slow CI machines and the GC can
// push the call into the next millisecond bucket.
func TestReadyzHandler_RuleCacheLatencyMs_AlwaysPopulated(t *testing.T) {
	t.Parallel()

	testutil.SetupTestTracing(t)

	tests := []struct {
		name     string
		provider RuleCacheHealthProvider
	}{
		{
			name:     "up_path_records_latency",
			provider: &mockCacheHealth{ready: true, staleness: time.Second, size: 1},
		},
		{
			name:     "down_path_records_latency",
			provider: &mockCacheHealth{ready: false, staleness: time.Duration(math.MaxInt64)},
		},
		{
			name:     "degraded_path_records_latency",
			provider: &mockCacheHealth{ready: true, staleness: time.Hour, size: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hc := NewTestableHealthChecker(nil)
			hc.SetCacheHealthProvider(tc.provider)

			ctx := libObservability.ContextWithTracer(context.Background(), otel.Tracer("tracer-test"))
			check := hc.probeReadyzRuleCache(ctx)

			assert.GreaterOrEqual(t, check.LatencyMs, int64(0),
				"rule_cache probe must populate LatencyMs on every path (got %d)",
				check.LatencyMs)
		})
	}
}

// TestReadyzHandler_RuleCacheLatencyMs_DownProviderNotConfigured covers the
// "no provider wired" branch: SetCacheHealthProvider was never called, so the
// probe short-circuits. The contract is identical to the other branches —
// LatencyMs must be present and >= 0.
func TestReadyzHandler_RuleCacheLatencyMs_DownProviderNotConfigured(t *testing.T) {
	t.Parallel()

	testutil.SetupTestTracing(t)

	hc := NewTestableHealthChecker(nil)
	// Deliberately do NOT call SetCacheHealthProvider.

	ctx := libObservability.ContextWithTracer(context.Background(), otel.Tracer("tracer-test"))
	check := hc.probeReadyzRuleCache(ctx)

	assert.Equal(t, StatusDown, check.Status)
	assert.GreaterOrEqual(t, check.LatencyMs, int64(0),
		"rule_cache probe must populate LatencyMs even on the not-configured branch")
}

// TestAggregateStatus_UnknownStatusFailsClosed pins the fail-CLOSED contract
// for the canonical aggregation rule. A probe that returns a Status outside
// the closed 5-value vocabulary ("up", "down", "degraded", "skipped", "n/a")
// — the empty string, an uppercase typo, anything else — must aggregate to
// "unhealthy" so a buggy probe cannot accidentally let an unready service
// take traffic.
func TestAggregateStatus_UnknownStatusFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		checks  map[string]api.ReadyzCheck
		wantTop string
	}{
		{
			name: "empty_status_fails_closed",
			checks: map[string]api.ReadyzCheck{
				"postgres":   {Status: StatusUp},
				"rule_cache": {Status: ""},
			},
			wantTop: StatusUnhealthy,
		},
		{
			name: "uppercase_typo_fails_closed",
			checks: map[string]api.ReadyzCheck{
				"postgres":   {Status: "OK"},
				"rule_cache": {Status: StatusUp},
			},
			wantTop: StatusUnhealthy,
		},
		{
			name: "ready_legacy_token_fails_closed",
			checks: map[string]api.ReadyzCheck{
				"postgres":   {Status: StatusUp},
				"rule_cache": {Status: "READY"},
			},
			wantTop: StatusUnhealthy,
		},
		{
			name: "all_canonical_up_is_healthy",
			checks: map[string]api.ReadyzCheck{
				"postgres":   {Status: StatusUp},
				"rule_cache": {Status: StatusUp},
			},
			wantTop: StatusHealthy,
		},
		{
			name: "skipped_and_na_are_healthy_contributions",
			checks: map[string]api.ReadyzCheck{
				"postgres":   {Status: StatusUp},
				"rule_cache": {Status: StatusSkipped},
				"other":      {Status: StatusNA},
			},
			wantTop: StatusHealthy,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := aggregateStatus(tc.checks)
			assert.Equal(t, tc.wantTop, got,
				"aggregation must fail closed for unknown statuses; %v", tc.checks)
		})
	}
}

// TestReadyzHandler_ProbesRunInParallel asserts the two /readyz probes
// execute concurrently rather than serially. The assertion is channel-based
// rather than wall-clock based so it is deterministic on loaded CI runners:
// each probe signals on entry and then blocks (postgres on sqlmock's
// WillDelayFor, rule_cache on the release channel). The test asserts BOTH
// probes signalled entry before EITHER was released.
//
// A serial handler would invoke the first probe and block on it indefinitely;
// the second probe's entry signal would never arrive and the test would fail
// at the 2s entry-signal timeout. Parallel execution makes both signals
// arrive within microseconds.
//
// Implementation note: gomock's DoAndReturn drives the entry signals — no
// hand-rolled test doubles required. Each EXPECT.DoAndReturn runs inside the
// probe goroutine, emitting on `entered` (best-effort, buffered) and then
// returning the underlying value (sqlmock-backed *sql.DB for postgres, true
// for rule_cache).
func TestReadyzHandler_ProbesRunInParallel(t *testing.T) {
	testutil.SetupTestTracing(t)

	entered := make(chan string, 2)
	release := make(chan struct{})

	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	defer func() {
		// Register close AFTER the request has been processed (and the
		// ExpectPing consumed) so the in-order expectation queue stays
		// correct. Verify all expectations were met to catch silent contract
		// regressions where a probe stops calling PingContext.
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}()

	dbProvider := NewMockPostgresDBProvider(ctrl)
	dbProvider.EXPECT().IsConnected().Return(true).AnyTimes()
	// GetDB is the observation point for the postgres probe — it runs inside
	// the probe goroutine just before db.PingContext. DoAndReturn lets us
	// emit the entry signal without polluting production code with a hook.
	dbProvider.EXPECT().GetDB(gomock.Any()).
		DoAndReturn(func(_ context.Context) (*sql.DB, error) {
			select {
			case entered <- "postgres":
			default:
			}

			return db, nil
		}).AnyTimes()

	// Wedge postgres inside the probe by making the ping block until the
	// release channel fires (or the 2s sqlmock delay elapses).
	mock.ExpectPing().WillDelayFor(2 * time.Second)

	cacheHealth := NewMockRuleCacheHealthProvider(ctrl)
	// IsReady is the rule_cache probe's first call — emit entry signal and
	// block on `release` to mirror how the postgres probe is wedged on
	// PingContext. Best-effort signal via select+default keeps it safe even
	// if the test has already moved on.
	cacheHealth.EXPECT().IsReady(gomock.Any()).
		DoAndReturn(func(ctx context.Context) bool {
			select {
			case entered <- "rule_cache":
			default:
			}

			select {
			case <-release:
			case <-ctx.Done():
			}

			return true
		}).AnyTimes()
	cacheHealth.EXPECT().Staleness(gomock.Any()).Return(time.Second).AnyTimes()

	hc := NewTestableHealthCheckerWithMeta(dbProvider, "1.0.0", "saas")
	hc.SetCacheHealthProvider(cacheHealth)

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	// Fire the request on a goroutine; it blocks until both probes complete.
	type result struct {
		statusCode int
		err        error
	}

	resultCh := make(chan result, 1)

	go func() {
		resp, testErr := app.Test(req, -1)
		if testErr != nil {
			resultCh <- result{err: testErr}
			return
		}

		defer resp.Body.Close()
		resultCh <- result{statusCode: resp.StatusCode}
	}()

	// Both probes MUST enter before EITHER is released — that is the
	// definition of parallel execution. A serial handler would never deliver
	// the second entry signal because the first probe would still be blocked.
	seen := map[string]bool{}

	for range 2 {
		select {
		case name := <-entered:
			seen[name] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("probe parallelism: only %d/2 probes entered within 2s; seen=%v", len(seen), seen)
		}
	}

	require.True(t, seen["postgres"], "postgres probe must enter concurrently")
	require.True(t, seen["rule_cache"], "rule_cache probe must enter concurrently")

	// Release the rule_cache probe. Postgres is wedged on sqlmock's
	// WillDelayFor (2s) and will exit on its own when the delay elapses.
	close(release)

	select {
	case res := <-resultCh:
		require.NoError(t, res.err)
		// Status code is not asserted here — parallelism is the only
		// invariant under test. Aggregation is covered by other tests.
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return within 5s after release")
	}
}

// TestReadyzHandler_MultiTenant_SkipsRuleCacheProbe asserts H1: in
// multi-tenant mode the /readyz handler MUST NOT probe the global rule cache
// because the empty-tenant bucket is intentionally evicted at boot
// (conditionalWarmUpCache). Probing it would always report "down" against the
// K8s probe context (no tenantID), aggregate to "unhealthy", and 503 every
// pod fleet-wide.
//
// Expected behaviour: rule_cache check reports Status=n/a (skipped), aggregate
// stays healthy, response code is 200. Postgres still probes normally.
func TestReadyzHandler_MultiTenant_SkipsRuleCacheProbe(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.2.3", "saas")
	defer cleanup()

	mock.ExpectPing()

	// Cache provider deliberately returns "not ready" — if the MT gate were
	// missing we would observe Status=down. The test asserts the gate
	// short-circuits BEFORE touching the provider.
	hc.SetCacheHealthProvider(&mockCacheHealth{ready: false})
	hc.SetMultiTenantEnabled(true)

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"MT mode must NOT 503 — the rule_cache lane is gated by design")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse

	require.NoError(t, json.Unmarshal(body, &response))

	rc, ok := response.Checks["rule_cache"]
	require.True(t, ok, "rule_cache check must still appear in the response shape")
	assert.Equal(t, StatusNA, rc.Status,
		"MT mode must report rule_cache as n/a, not down")
	assert.Empty(t, rc.Error, "skipped probe must not surface an error")
	assert.Equal(t, StatusHealthy, response.Status,
		"aggregate must remain healthy when rule_cache is n/a")
}

// TestReadyzHandler_MultiTenant_PostgresStillProbed verifies that the MT
// gate only short-circuits the rule_cache probe — postgres failures still
// propagate to the aggregate so /readyz remains a meaningful infra signal.
func TestReadyzHandler_MultiTenant_PostgresStillProbed(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.2.3", "saas")
	defer cleanup()

	// Postgres ping fails: the postgres lane MUST still flip the aggregate
	// to unhealthy even with the rule_cache gate active.
	mock.ExpectPing().WillReturnError(errors.New("simulated db outage"))

	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})
	hc.SetMultiTenantEnabled(true)

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"postgres failure must still 503 in MT mode")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse

	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
	assert.Equal(t, StatusNA, response.Checks["rule_cache"].Status,
		"rule_cache must remain n/a regardless of postgres outcome")
}

// TestHealthChecker_SetCacheStalenessThreshold_Override verifies H3: bootstrap
// can override the default 5-min threshold via
// READYZ_CACHE_STALENESS_THRESHOLD_SECONDS, and the new value is respected by
// the rule_cache probe's degraded branch.
func TestHealthChecker_SetCacheStalenessThreshold_Override(t *testing.T) {
	testutil.SetupTestTracing(t)

	hc, mock, cleanup := newReadyzCheckerWithDB(t, "1.2.3", "local")
	defer cleanup()

	hc.SetCacheStalenessThreshold(1 * time.Second)
	require.Equal(t, 1*time.Second, hc.CacheStalenessThreshold(),
		"setter must update the live threshold")

	mock.ExpectPing()
	hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: 30 * time.Second})

	app := createReadyzTestApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"override must take effect — staleness > threshold ⇒ 503")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse

	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, StatusDegraded, response.Checks["rule_cache"].Status,
		"shorter threshold must flip the rule_cache lane to degraded")
}

// TestHealthChecker_SetCacheStalenessThreshold_NonPositiveIgnored verifies
// the setter contract: a 0 or negative value MUST NOT clobber the default,
// because a zero threshold would force every probe into "degraded" the
// moment any staleness is reported.
func TestHealthChecker_SetCacheStalenessThreshold_NonPositiveIgnored(t *testing.T) {
	hc := NewTestableHealthChecker(nil)

	defaultThreshold := hc.CacheStalenessThreshold()
	require.Greater(t, defaultThreshold, time.Duration(0),
		"sanity: default threshold must be positive")

	hc.SetCacheStalenessThreshold(0)
	assert.Equal(t, defaultThreshold, hc.CacheStalenessThreshold(),
		"zero must be rejected — default preserved")

	hc.SetCacheStalenessThreshold(-1 * time.Second)
	assert.Equal(t, defaultThreshold, hc.CacheStalenessThreshold(),
		"negative must be rejected — default preserved")

	hc.SetCacheStalenessThreshold(2 * time.Minute)
	assert.Equal(t, 2*time.Minute, hc.CacheStalenessThreshold(),
		"positive override must land")
}
