// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/buildinfo"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// stubChecker is a Checker that returns a pre-canned DependencyCheck. Used
// to drive deterministic handler tests.
type stubChecker struct {
	name   string
	result DependencyCheck
	calls  *atomic.Int64
	delay  time.Duration
}

func (s *stubChecker) Name() string { return s.name }

func (s *stubChecker) Check(ctx context.Context) DependencyCheck {
	if s.calls != nil {
		s.calls.Add(1)
	}

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
		}
	}

	return s.result
}

// ----------------------------------------------------------------------------
// Build provenance fields (Epic 2.2)
// ----------------------------------------------------------------------------

// TestFiberHandler_IncludesBuildInfo asserts the /readyz body carries the
// build provenance fields (commit/buildTime/dirty) sourced from buildinfo.Get()
// at handler construction time. JSON keys must match the VersionHandler wire
// contract exactly.
func TestFiberHandler_IncludesBuildInfo(t *testing.T) {
	t.Parallel()

	info := buildinfo.Get()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(nil, &DrainState{}, "1.2.3", "saas", nil))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var got map[string]any

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, info.Commit, got["commit"])
	assert.Equal(t, info.BuildTime, got["buildTime"])
	assert.Equal(t, info.Dirty, got["dirty"])
}

// TestNetHTTPHandler_IncludesBuildInfo is the net/http counterpart, asserting
// the same provenance fields appear in the Worker's /readyz body.
func TestNetHTTPHandler_IncludesBuildInfo(t *testing.T) {
	t.Parallel()

	info := buildinfo.Get()

	h := NewNetHTTPHandler(nil, &DrainState{}, "1.2.3", "saas", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	var got map[string]any

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	assert.Equal(t, info.Commit, got["commit"])
	assert.Equal(t, info.BuildTime, got["buildTime"])
	assert.Equal(t, info.Dirty, got["dirty"])
}

// ----------------------------------------------------------------------------
// Fiber handler
// ----------------------------------------------------------------------------

func newFiberAppWithReadyz(checkers []Checker, drain *DrainState) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, drain, "1.2.3", "saas", nil))

	return app
}

func TestFiberHandler_AllUp_Returns200Healthy(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&stubChecker{name: "redis", result: DependencyCheck{Status: StatusUp, LatencyMs: 3}},
	}

	app := newFiberAppWithReadyz(checkers, &DrainState{})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "healthy", body.Status)
	assert.Equal(t, "1.2.3", body.Version)
	assert.Equal(t, "saas", body.DeploymentMode)
	assert.Len(t, body.Checks, 2)
	assert.Equal(t, StatusUp, body.Checks["mongo"].Status)
	assert.Equal(t, StatusUp, body.Checks["redis"].Status)
}

func TestFiberHandler_OneDown_Returns503Unhealthy(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}},
		&stubChecker{name: "redis", result: DependencyCheck{Status: StatusDown, Error: "boom"}},
	}

	app := newFiberAppWithReadyz(checkers, &DrainState{})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "unhealthy", body.Status)
	assert.Equal(t, StatusDown, body.Checks["redis"].Status)
}

func TestFiberHandler_Draining_Returns503DrainingNoChecks(t *testing.T) {
	t.Parallel()

	calls := &atomic.Int64{}
	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}, calls: calls},
	}

	drain := &DrainState{}
	drain.StartDraining()

	app := newFiberAppWithReadyz(checkers, drain)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var parsed Response

	require.NoError(t, json.Unmarshal(body, &parsed))

	assert.Equal(t, "draining", parsed.Status)
	assert.NotEmpty(t, parsed.Reason)
	// Dispatch 2 G fix: the Checks field is always present in the JSON
	// response — even during drain — so dashboards keying on `checks`
	// keep working (they just see an empty map). Non-nil but empty is
	// the canonical contract.
	assert.NotNil(t, parsed.Checks, "Checks field must always be present, even during drain")
	assert.Empty(t, parsed.Checks, "Checks must be empty during drain (no probes ran)")
	assert.Equal(t, int64(0), calls.Load(), "checkers must NOT be called when draining")
	assert.Equal(t, "1.2.3", parsed.Version)
	assert.Equal(t, "saas", parsed.DeploymentMode)
}

// TestFiberHandler_Draining_RawJSONHasChecksField verifies that the wire
// format actually contains a "checks" key during drain — not just that
// the decoded Response struct has a non-nil map (which json.Unmarshal
// could fabricate).
func TestFiberHandler_Draining_RawJSONHasChecksField(t *testing.T) {
	t.Parallel()

	drain := &DrainState{}
	drain.StartDraining()

	app := newFiberAppWithReadyz(nil, drain)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Raw substring assertion: the JSON must literally contain "checks":{}.
	assert.Contains(t, string(body), `"checks":{}`,
		"drain response must include an empty checks map: %s", string(body))
}

// TestFiberHandler_ParallelCheckersCompleteWithinSlowestBudget verifies
// runAll's parallel fan-out semantics: three independent checkers each
// sleeping `delay` should complete in roughly one `delay` window, not
// three (which would indicate serial execution). The threshold is set
// to 2*delay to leave generous slack for CI scheduler jitter without
// admitting actually-serial execution (which would take >=3*delay).
//
// Test-reviewer L1: renamed from TestFiberHandler_RunsCheckersInParallel
// because the original name suggested a structural assertion (e.g.
// instrumented Check entry/exit) but the body only measured wall time.
// The new name reflects what is actually being verified.
func TestFiberHandler_ParallelCheckersCompleteWithinSlowestBudget(t *testing.T) {
	t.Parallel()

	// Three checkers each sleeping 50ms. Serial execution would take
	// ~150ms; parallel execution completes in roughly the slowest single
	// checker's duration.
	const delay = 50 * time.Millisecond

	checkers := []Checker{
		&stubChecker{name: "a", result: DependencyCheck{Status: StatusUp}, delay: delay},
		&stubChecker{name: "b", result: DependencyCheck{Status: StatusUp}, delay: delay},
		&stubChecker{name: "c", result: DependencyCheck{Status: StatusUp}, delay: delay},
	}

	app := newFiberAppWithReadyz(checkers, &DrainState{})

	start := time.Now()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	elapsed := time.Since(start)

	// 2*delay is the upper bound: 3*delay would indicate serial. CI
	// scheduling jitter makes a tighter bound flaky; this one stays
	// stable while still falsifying serial execution.
	assert.Less(t, elapsed, 2*delay,
		"3 checkers each sleeping %s must complete in parallel in <%s (vs %s serial), got %s",
		delay, 2*delay, 3*delay, elapsed)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFiberHandler_EmptyCheckers_Returns200Healthy(t *testing.T) {
	t.Parallel()

	app := newFiberAppWithReadyz(nil, &DrainState{})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "healthy", body.Status)
}

func TestFiberHandler_NilDrainState_DoesNotPanic(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, nil, "1.0", "local", nil))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ----------------------------------------------------------------------------
// net/http handler
// ----------------------------------------------------------------------------

func TestNetHTTPHandler_AllUp_Returns200(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
	}

	h := NewNetHTTPHandler(checkers, &DrainState{}, "1.0", "local", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, "healthy", body.Status)
	assert.Equal(t, StatusUp, body.Checks["mongo"].Status)
}

func TestNetHTTPHandler_OneDown_Returns503(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusDown, Error: "ping failed"}},
	}

	h := NewNetHTTPHandler(checkers, &DrainState{}, "1.0", "local", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, "unhealthy", body.Status)
}

func TestNetHTTPHandler_Draining_Returns503DrainingNoChecks(t *testing.T) {
	t.Parallel()

	calls := &atomic.Int64{}
	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}, calls: calls},
	}

	drain := &DrainState{}
	drain.StartDraining()

	h := NewNetHTTPHandler(checkers, drain, "1.0", "local", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, "draining", body.Status)
	// Empty but non-nil — the field is always present in the response.
	assert.NotNil(t, body.Checks, "Checks must be present during drain")
	assert.Empty(t, body.Checks)
	assert.Equal(t, int64(0), calls.Load())
	// Test-reviewer M4: the drain response on the net/http transport must
	// expose the same shape as the Fiber transport, including Version and
	// DeploymentMode. Asymmetry-eliminating regression guard.
	assert.Equal(t, "1.0", body.Version,
		"Version must be present in the drain response on net/http")
	assert.Equal(t, "local", body.DeploymentMode,
		"DeploymentMode must be present in the drain response on net/http")
	assert.NotEmpty(t, body.Reason,
		"drain response must include a Reason explaining why probes were skipped")
}

// TestNetHTTPHandler_Draining_RawJSONHasChecksField mirrors the Fiber
// raw-JSON assertion on the net/http transport.
func TestNetHTTPHandler_Draining_RawJSONHasChecksField(t *testing.T) {
	t.Parallel()

	drain := &DrainState{}
	drain.StartDraining()

	h := NewNetHTTPHandler(nil, drain, "1.0", "local", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"checks":{}`,
		"drain response must include an empty checks map: %s", rec.Body.String())
}

// subMsChecker simulates a checker that runs in well under one millisecond
// (cache hit, in-memory probe). The DependencyCheck it returns reports a
// realistic LatencyMs=0 and an internal Latency in microseconds.
type subMsChecker struct {
	name    string
	latency time.Duration
}

func (s *subMsChecker) Name() string { return s.name }

func (s *subMsChecker) Check(_ context.Context) DependencyCheck {
	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: s.latency.Milliseconds(), // truncates to 0 for sub-ms
		Latency:   s.latency,
	}
}

// TestHandler_LatencyPrecision_SubMsRecordedAsFractionalMs verifies the
// FULL handler→aggregator→metrics path preserves sub-millisecond probe
// latencies in the histogram. Before the precision fix, the handler
// reconstructed Duration via time.Duration(check.LatencyMs)*time.Millisecond,
// which truncated sub-ms probes to 0 and silently bottomed out the
// histogram on every cache hit.
//
// This test runs an end-to-end /readyz request with a 200µs (sub-ms)
// checker and asserts the histogram observation is in (0, 1] — i.e., the
// first explicit bucket (0..1ms) for sub-millisecond probes. The
// canonical bucket set is {1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000,
// 5000} so a 0.2ms observation lands in BucketCounts[0].
//
// The fix: DependencyCheck now carries an internal time.Duration field
// (not marshalled, preserves full resolution) that the handler forwards
// to EmitCheckDuration directly. LatencyMs in the JSON wire format
// continues to be int64 ms (the contract the dashboard schema relies on),
// but the histogram sees the unrounded value.
//
// Test-reviewer H2.
func TestHandler_LatencyPrecision_SubMsRecordedAsFractionalMs(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	checkers := []Checker{
		&subMsChecker{name: "fast", latency: 200 * time.Microsecond},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, &DrainState{}, "1.0", "local", m))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	got, ok := findMetric(t, rm, "readyz_check_duration_ms")
	require.True(t, ok)

	hist := got.Data.(metricdata.Histogram[float64])
	require.Len(t, hist.DataPoints, 1)

	dp := hist.DataPoints[0]
	require.Equal(t, uint64(1), dp.Count, "exactly one observation expected")

	// The observation must reflect the actual sub-ms latency, not 0.
	// 200µs == 0.2ms. We allow a small delta to absorb scheduler jitter
	// (the goroutine handoff in the handler can add a few µs), but the
	// recorded value MUST be > 0 — that's the whole point.
	assert.Greater(t, dp.Sum, 0.0,
		"sub-ms probe must record a fractional ms value, got %v", dp.Sum)
	assert.Less(t, dp.Sum, 1.0,
		"sub-ms probe must record < 1ms (first bucket), got %v", dp.Sum)

	// Bucket-1 (0..1ms) must contain the observation. With the canonical
	// bucket set the first BucketCount slot represents (-Inf, 1.0].
	require.NotEmpty(t, dp.BucketCounts)
	assert.Equal(t, uint64(1), dp.BucketCounts[0],
		"sub-ms observation must land in the first bucket [0..1ms]: %+v",
		dp.BucketCounts)
}

// TestFiberHandler_EmitsMetrics_PerCheck verifies that the Fiber handler
// records readyz_check_duration_ms and readyz_check_status with the expected
// (dep, status) attributes for every checker in a single request.
func TestFiberHandler_EmitsMetrics_PerCheck(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 7}},
		&stubChecker{name: "redis", result: DependencyCheck{Status: StatusDown, Error: "boom", LatencyMs: 12}},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, &DrainState{}, "1.0", "local", m))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	statusGot, ok := findMetric(t, rm, "readyz_check_status")
	require.True(t, ok)

	sum := statusGot.Data.(metricdata.Sum[int64])
	require.Len(t, sum.DataPoints, 2, "one data point per (dep, status) tuple")

	durGot, ok := findMetric(t, rm, "readyz_check_duration_ms")
	require.True(t, ok)

	hist := durGot.Data.(metricdata.Histogram[float64])
	require.Len(t, hist.DataPoints, 2)
}

// TestNetHTTPHandler_EmitsMetrics_PerCheck mirrors the Fiber metrics check on
// the net/http path so both transports are guaranteed to wire metrics
// identically.
func TestNetHTTPHandler_EmitsMetrics_PerCheck(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
	}

	h := NewNetHTTPHandler(checkers, &DrainState{}, "1.0", "local", m)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	statusGot, ok := findMetric(t, rm, "readyz_check_status")
	require.True(t, ok)

	sum := statusGot.Data.(metricdata.Sum[int64])
	require.Len(t, sum.DataPoints, 1)
	assert.Equal(t, int64(1), sum.DataPoints[0].Value)
}

// TestHandler_DrainShortCircuit_NoMetricsEmitted verifies that a draining
// response does NOT emit per-check metrics — there were no checks to emit
// for. This protects dashboards from spurious "down" counts during
// shutdown.
func TestHandler_DrainShortCircuit_NoMetricsEmitted(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	drain := &DrainState{}
	drain.StartDraining()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, drain, "1.0", "local", m))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	_, ok := findMetric(t, rm, "readyz_check_status")
	assert.False(t, ok, "no per-check metrics should be recorded during drain")
}

// slowIgnoringCtxChecker simulates a misbehaving checker that ignores the
// context's cancellation signal and sleeps. The handler must remain
// responsive: production checkers in this package always honor ctx, but a
// future checker (or a refactor) could regress, and runAll's fan-out must
// not hang the request indefinitely.
//
// release lets the test unblock the goroutine before TestMain's goleak
// guard runs, so the leaked goroutine eventually exits cleanly.
type slowIgnoringCtxChecker struct {
	name    string
	release <-chan struct{}
}

func (s *slowIgnoringCtxChecker) Name() string { return s.name }

func (s *slowIgnoringCtxChecker) Check(_ context.Context) DependencyCheck {
	if s.release != nil {
		<-s.release
	}

	return DependencyCheck{Status: StatusUp, LatencyMs: 0}
}

// TestRunAll_HandlesSlowCheckerWithCancelledCtx verifies that runAll
// honors the request-level ctx deadline even when an individual Checker
// ignores ctx. The defensive guarantee: if a checker takes longer than
// the request budget, runAll returns the partial results immediately
// rather than blocking on wg.Wait().
//
// We launch one fast checker and one slow-ignoring-ctx checker. The
// request ctx has a 100ms deadline. The slow checker is held by a
// release channel for the duration of the test, so without the
// defensive timeout in runAll the test would deadlock on wg.Wait().
//
// The leaked goroutine (the slow checker) is released at end-of-test
// so TestMain's goleak.VerifyTestMain succeeds.
//
// Test-reviewer H4.
func TestRunAll_HandlesSlowCheckerWithCancelledCtx(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	defer close(release)

	checkers := []Checker{
		&stubChecker{name: "fast", result: DependencyCheck{Status: StatusUp, LatencyMs: 1}},
		&slowIgnoringCtxChecker{name: "slow", release: release},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	results := runAll(ctx, checkers)
	elapsed := time.Since(start)

	// Must complete within the budget window — generous bound to avoid
	// CI flakes. If runAll blocked on wg.Wait() the test would deadlock
	// at the release channel close instead.
	assert.Less(t, elapsed, 1*time.Second,
		"runAll must honor ctx deadline; took %s", elapsed)

	// The fast checker's result MUST be in the map.
	fast, ok := results["fast"]
	require.True(t, ok, "fast checker result must be present: %+v", results)
	assert.Equal(t, StatusUp, fast.Status)

	// The slow checker MUST also be in the map — runAll synthesizes a
	// timed-out entry for any checker whose real result was not collected
	// before ctx fired. This keeps the response shape stable: every
	// declared dependency always has an entry, even when the checker
	// itself is misbehaving.
	slow, ok := results["slow"]
	require.True(t, ok, "slow checker result must be present (synthesized): %+v", results)
	assert.Equal(t, StatusDown, slow.Status,
		"misbehaving checker must be reported as down, not absent")
	assert.Contains(t, slow.Error, "timed out",
		"synthesized error must indicate the timeout cause: %q", slow.Error)
}

// erroringChecker simulates a checker that returns a structured error in
// the DependencyCheck. The handler must propagate it without panicking.
type erroringChecker struct{}

func (e erroringChecker) Name() string { return "x" }
func (e erroringChecker) Check(_ context.Context) DependencyCheck {
	return DependencyCheck{Status: StatusDown, Error: errors.New("nope").Error()}
}

func TestNetHTTPHandler_StructuredErrorPropagated(t *testing.T) {
	t.Parallel()

	h := NewNetHTTPHandler([]Checker{erroringChecker{}}, &DrainState{}, "1.0", "local", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, "nope", body.Checks["x"].Error)
}

// TestFiberHandler_NilMetrics_DoesNotPanic guarantees that the documented
// nil-metrics tolerance in NewHandler holds end-to-end. Every per-dep emit
// path in emitCheckResults runs through the Metrics nil-receiver guards;
// this test exercises that path with multiple checkers (each producing
// their own duration + status emission) so a regression that re-introduces
// a direct receiver dereference would surface as a runtime panic on the
// /readyz hot path.
func TestFiberHandler_NilMetrics_DoesNotPanic(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&stubChecker{name: "redis", result: DependencyCheck{Status: StatusDown, LatencyMs: 12, Error: "boom"}},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler(checkers, &DrainState{}, "1.2.3", "saas", nil))

	var resp *http.Response

	require.NotPanics(t, func() {
		var err error

		resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.NoError(t, err)
	})

	defer resp.Body.Close()

	// Aggregate is unhealthy because one dep is down.
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "unhealthy", body.Status)
	assert.Len(t, body.Checks, 2)
	assert.Equal(t, StatusUp, body.Checks["mongo"].Status)
	assert.Equal(t, StatusDown, body.Checks["redis"].Status)
}

// TestNetHTTPHandler_NilMetrics_DoesNotPanic mirrors the Fiber handler
// guarantee for the bare-stdlib transport. Worker bootstraps that wire
// /readyz before metrics are plumbed (or test scaffolding that omits the
// emitter entirely) MUST NOT see a runtime panic on probe execution.
func TestNetHTTPHandler_NilMetrics_DoesNotPanic(t *testing.T) {
	t.Parallel()

	checkers := []Checker{
		&stubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&stubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusUp, LatencyMs: 7}},
		&stubChecker{name: "storage", result: DependencyCheck{Status: StatusDown, LatencyMs: 250, Error: "io timeout"}},
	}

	h := NewNetHTTPHandler(checkers, &DrainState{}, "1.2.3", "saas", nil)
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unhealthy", body.Status)
	assert.Len(t, body.Checks, 3)
	assert.Equal(t, StatusUp, body.Checks["mongo"].Status)
	assert.Equal(t, StatusUp, body.Checks["rabbitmq"].Status)
	assert.Equal(t, StatusDown, body.Checks["storage"].Status)
}
