// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// probeStubChecker is a deterministic Checker used to drive RunSelfProbe in
// tests. It returns the configured DependencyCheck verbatim, optionally after
// a small sleep so latency-related assertions have non-zero values.
//
// Named distinctly from handler_test.go's stubChecker (which lives in the
// same _test package) to avoid redeclaration.
type probeStubChecker struct {
	name   string
	result DependencyCheck
	sleep  time.Duration
}

func (s *probeStubChecker) Name() string { return s.name }

func (s *probeStubChecker) Check(_ context.Context) DependencyCheck {
	if s.sleep > 0 {
		time.Sleep(s.sleep)
	}

	return s.result
}

// recordingLogger captures every Log() call so tests can assert on the
// structured events RunSelfProbe is required to emit. It implements log.Logger
// from lib-commons/v5 so it can be passed to RunSelfProbe verbatim.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedEntry
}

type recordedEntry struct {
	level  log.Level
	msg    string
	fields []log.Field
}

func (r *recordingLogger) Log(_ context.Context, level log.Level, msg string, fields ...log.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]log.Field, len(fields))
	copy(cp, fields)
	r.entries = append(r.entries, recordedEntry{level: level, msg: msg, fields: cp})
}

func (r *recordingLogger) With(_ ...log.Field) log.Logger { return r }
func (r *recordingLogger) WithGroup(_ string) log.Logger  { return r }
func (r *recordingLogger) Enabled(_ log.Level) bool       { return true }
func (r *recordingLogger) Sync(_ context.Context) error   { return nil }

func (r *recordingLogger) snapshot() []recordedEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]recordedEntry, len(r.entries))
	copy(out, r.entries)

	return out
}

// hasMessage reports whether any captured entry matches msg with the given level.
func (r *recordingLogger) hasMessage(level log.Level, msg string) bool {
	for _, e := range r.snapshot() {
		if e.level == level && e.msg == msg {
			return true
		}
	}

	return false
}

// countMessage returns how many captured entries match msg (any level).
func (r *recordingLogger) countMessage(msg string) int {
	n := 0

	for _, e := range r.snapshot() {
		if e.msg == msg {
			n++
		}
	}

	return n
}

// newProbeMetricsReader builds an OTel ManualReader-backed Metrics so tests can
// inspect emitted selfprobe_result data points after RunSelfProbe completes.
func newProbeMetricsReader(t *testing.T) (*sdkmetric.ManualReader, *Metrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("readyz-selfprobe-test")

	m, err := NewMetrics(meter)
	require.NoError(t, err)

	return reader, m
}

// collectSelfProbeGauge returns the latest selfprobe_result data points keyed
// by dep name. Empty map means no points were emitted for that metric.
func collectSelfProbeGauge(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	got, ok := findMetric(t, rm, "selfprobe_result")
	if !ok {
		return map[string]int64{}
	}

	gauge, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "selfprobe_result must be Gauge[int64], got %T", got.Data)

	values := make(map[string]int64, len(gauge.DataPoints))

	for _, dp := range gauge.DataPoints {
		dep, _ := dp.Attributes.Value("dep")
		values[dep.AsString()] = dp.Value
	}

	return values
}

// TestSelfProbeState_DefaultsToUnhealthy proves that a freshly constructed
// SelfProbeState reports IsHealthy()=false. /health gating relies on this
// invariant: traffic must not be accepted until RunSelfProbe explicitly flips
// the flag.
func TestSelfProbeState_DefaultsToUnhealthy(t *testing.T) {
	t.Parallel()

	var state SelfProbeState

	assert.False(t, state.IsHealthy(), "new SelfProbeState must default to unhealthy")
}

// TestSelfProbeState_MarkHealthy_FlipsFlag verifies the one-way transition from
// unhealthy to healthy. There is no reset by design.
func TestSelfProbeState_MarkHealthy_FlipsFlag(t *testing.T) {
	t.Parallel()

	state := &SelfProbeState{}
	state.MarkHealthy()

	assert.True(t, state.IsHealthy(), "after MarkHealthy, IsHealthy must be true")
}

// TestSelfProbeState_MarkHealthy_Idempotent verifies that MarkHealthy can be
// called multiple times without changing the observed state. This matters
// because the bootstrap path may retry the self-probe and we don't want the
// second success to no-op or to flip the flag back.
func TestSelfProbeState_MarkHealthy_Idempotent(t *testing.T) {
	t.Parallel()

	state := &SelfProbeState{}
	state.MarkHealthy()
	state.MarkHealthy()
	state.MarkHealthy()

	assert.True(t, state.IsHealthy())
}

// TestSelfProbeState_ConcurrentReadsAreRaceFree exercises IsHealthy() and
// MarkHealthy() from many goroutines under -race. The atomic.Bool inside
// SelfProbeState must serialize all access.
func TestSelfProbeState_ConcurrentReadsAreRaceFree(t *testing.T) {
	t.Parallel()

	state := &SelfProbeState{}

	var wg sync.WaitGroup

	const workers = 16

	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				_ = state.IsHealthy()
			}
		}()
		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				state.MarkHealthy()
			}
		}()
	}

	wg.Wait()

	assert.True(t, state.IsHealthy())
}

// TestRunSelfProbe_AllUp_ReturnsNil verifies the happy path: every checker
// returns up, RunSelfProbe returns nil error, and the operator log shows
// passed.
func TestRunSelfProbe_AllUp_ReturnsNil(t *testing.T) {
	t.Parallel()

	reader, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&probeStubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusUp, LatencyMs: 7}},
		&probeStubChecker{name: "redis", result: DependencyCheck{Status: StatusUp, LatencyMs: 1}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.NoError(t, err, "all-up probe must return nil")

	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_started"),
		"must log startup_self_probe_started")
	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_passed"),
		"must log startup_self_probe_passed when all deps are up")
	assert.False(t, rec.hasMessage(log.LevelError, "startup_self_probe_failed"),
		"must NOT log failed when all deps are up")
	assert.Equal(t, len(checkers), rec.countMessage("self_probe_check"),
		"per-dep self_probe_check log must fire once per checker")

	gauge := collectSelfProbeGauge(t, reader)
	assert.Equal(t, int64(1), gauge["mongo"])
	assert.Equal(t, int64(1), gauge["rabbitmq"])
	assert.Equal(t, int64(1), gauge["redis"])
}

// TestRunSelfProbe_OneDown_ReturnsError verifies that any down dep fails the
// probe, and the returned error references the failing dep name.
func TestRunSelfProbe_OneDown_ReturnsError(t *testing.T) {
	t.Parallel()

	reader, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&probeStubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusDown, Error: "amqp dial failed: timeout"}},
		&probeStubChecker{name: "redis", result: DependencyCheck{Status: StatusUp, LatencyMs: 1}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.Error(t, err, "one down dep must fail the probe")
	assert.Contains(t, err.Error(), "rabbitmq", "error must name the failing dep")

	assert.True(t, rec.hasMessage(log.LevelError, "startup_self_probe_failed"),
		"must log startup_self_probe_failed on any down dep")
	assert.False(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_passed"),
		"must NOT log passed when any dep is down")

	gauge := collectSelfProbeGauge(t, reader)
	assert.Equal(t, int64(1), gauge["mongo"])
	assert.Equal(t, int64(0), gauge["rabbitmq"])
	assert.Equal(t, int64(1), gauge["redis"])
}

// TestRunSelfProbe_Degraded_ReturnsError verifies that StatusDegraded is
// treated as unhealthy by the self-probe (matches the /readyz aggregation
// rule in aggregation.go).
func TestRunSelfProbe_Degraded_ReturnsError(t *testing.T) {
	t.Parallel()

	_, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusDegraded, Error: "circuit half-open"}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongo")
}

// TestRunSelfProbe_AllSkipped_ReturnsNil verifies that skipped deps count as
// healthy. A service whose only deps are intentionally disabled (e.g.,
// FETCHER_ENABLED=false) must be allowed to start.
func TestRunSelfProbe_AllSkipped_ReturnsNil(t *testing.T) {
	t.Parallel()

	reader, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "fetcher", result: DependencyCheck{Status: StatusSkipped, Reason: "FETCHER_ENABLED=false"}},
		&probeStubChecker{name: "tenant_manager", result: DependencyCheck{Status: StatusSkipped, Reason: "MULTI_TENANT_ENABLED=false"}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.NoError(t, err, "skipped deps must count as healthy")
	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_passed"))

	gauge := collectSelfProbeGauge(t, reader)
	// Skipped deps are reported as up=1 in the gauge — they are intentionally
	// healthy, not absent.
	assert.Equal(t, int64(1), gauge["fetcher"])
	assert.Equal(t, int64(1), gauge["tenant_manager"])
}

// TestRunSelfProbe_AllNA_ReturnsNil mirrors the skipped case but for
// StatusNA, which is the contract value used by Mongo/RabbitMQ in
// multi-tenant mode where per-tenant probing is deferred.
func TestRunSelfProbe_AllNA_ReturnsNil(t *testing.T) {
	t.Parallel()

	_, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusNA, Reason: "multi-tenant deferred"}},
		&probeStubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusNA, Reason: "multi-tenant deferred"}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.NoError(t, err)
	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_passed"))
}

// TestRunSelfProbe_MixedStatuses_DownFails verifies that a single down dep
// among many otherwise healthy deps still fails the probe. This is the
// canonical "any failure is total failure" rule.
func TestRunSelfProbe_MixedStatuses_DownFails(t *testing.T) {
	t.Parallel()

	reader, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp, LatencyMs: 3}},
		&probeStubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusSkipped, Reason: "disabled"}},
		&probeStubChecker{name: "redis", result: DependencyCheck{Status: StatusDown, Error: "redis unreachable"}},
		&probeStubChecker{name: "fetcher", result: DependencyCheck{Status: StatusNA, Reason: "n/a"}},
	}

	err := RunSelfProbe(context.Background(), checkers, metrics, rec)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis")

	gauge := collectSelfProbeGauge(t, reader)
	assert.Equal(t, int64(1), gauge["mongo"])
	assert.Equal(t, int64(1), gauge["rabbitmq"])
	assert.Equal(t, int64(0), gauge["redis"])
	assert.Equal(t, int64(1), gauge["fetcher"])
}

// TestRunSelfProbe_EmptyCheckers_ReturnsNil verifies that a service with no
// configured deps does not block on startup. It still emits the started/passed
// log pair so dashboards see the probe ran.
func TestRunSelfProbe_EmptyCheckers_ReturnsNil(t *testing.T) {
	t.Parallel()

	_, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	err := RunSelfProbe(context.Background(), nil, metrics, rec)

	require.NoError(t, err)
	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_started"))
	assert.True(t, rec.hasMessage(log.LevelInfo, "startup_self_probe_passed"))
}

// TestRunSelfProbe_NilMetrics_DoesNotPanic verifies that passing a nil
// *Metrics is tolerated. EmitSelfProbeResult is already nil-safe (see
// metrics.go); this test guards the call site against accidental
// dereferences (e.g., a future caller forgetting the nil check before
// invoking Emit*).
func TestRunSelfProbe_NilMetrics_DoesNotPanic(t *testing.T) {
	t.Parallel()

	rec := &recordingLogger{}
	checkers := []Checker{
		&probeStubChecker{name: "mongo", result: DependencyCheck{Status: StatusUp}},
	}

	assert.NotPanics(t, func() {
		_ = RunSelfProbe(context.Background(), checkers, nil, rec)
	})
}

// TestRunSelfProbe_PerDepLogContainsDepNameAndStatus verifies the structured
// per-dep log carries enough information for an operator to diagnose which
// dep failed without parsing the aggregated error. Specifically, every
// per-dep "self_probe_check" entry must carry both a "dep" field naming the
// checker and a "status" field carrying the closed-vocabulary status.
//
// Test-reviewer M6: the previous version only confirmed the message was
// emitted, not that the structured fields actually carried the dep name.
// A refactor that drops the log.String("dep", …) call would have gone
// undetected. This version inspects the captured Field slice and asserts
// the dep field is present and matches the checker's Name() — applied
// across the canonical six-dep checker set so the assertion stays
// representative of production wiring.
func TestRunSelfProbe_PerDepLogContainsDepNameAndStatus(t *testing.T) {
	t.Parallel()

	_, metrics := newProbeMetricsReader(t)
	rec := &recordingLogger{}

	// Mirror the canonical six-dep set so we exercise the assertion across
	// the full surface (mongodb, rabbitmq, redis, storage, fetcher,
	// tenant_manager). A mix of statuses ensures both the success and
	// failure log paths are covered.
	checkers := []Checker{
		&probeStubChecker{name: "mongodb", result: DependencyCheck{Status: StatusUp, LatencyMs: 5}},
		&probeStubChecker{name: "rabbitmq", result: DependencyCheck{Status: StatusUp, LatencyMs: 7}},
		&probeStubChecker{name: "redis", result: DependencyCheck{Status: StatusDown, Error: "boom"}},
		&probeStubChecker{name: "storage", result: DependencyCheck{Status: StatusUp, LatencyMs: 3}},
		&probeStubChecker{name: "fetcher", result: DependencyCheck{Status: StatusSkipped, Reason: "FETCHER_ENABLED=false"}},
		&probeStubChecker{name: "tenant_manager", result: DependencyCheck{Status: StatusNA, Reason: "MULTI_TENANT_ENABLED=false"}},
	}

	_ = RunSelfProbe(context.Background(), checkers, metrics, rec)

	// Build a name → fields map from every "self_probe_check" entry. We
	// use the first matching entry per dep; the production code emits
	// exactly one self_probe_check per checker.
	perDep := make(map[string][]log.Field)

	for _, e := range rec.snapshot() {
		if e.msg != "self_probe_check" {
			continue
		}

		for _, f := range e.fields {
			if f.Key == "dep" {
				if depName, ok := f.Value.(string); ok {
					perDep[depName] = e.fields
				}

				break
			}
		}
	}

	// fieldByKey extracts the first field whose Key matches.
	fieldByKey := func(fields []log.Field, key string) (log.Field, bool) {
		for _, f := range fields {
			if f.Key == key {
				return f, true
			}
		}

		return log.Field{}, false
	}

	for _, ck := range checkers {
		fields, ok := perDep[ck.Name()]
		require.True(t, ok, "self_probe_check entry must exist for dep=%q (saw=%v)",
			ck.Name(), perDep)

		depField, ok := fieldByKey(fields, "dep")
		require.True(t, ok, "self_probe_check log MUST carry a 'dep' field for %q", ck.Name())
		assert.Equal(t, ck.Name(), depField.Value,
			"dep field Value must equal the checker name for %q", ck.Name())

		statusField, ok := fieldByKey(fields, "status")
		require.True(t, ok, "self_probe_check log MUST carry a 'status' field for %q", ck.Name())
		assert.NotEmpty(t, statusField.Value,
			"status field must be populated from the closed status vocabulary for %q",
			ck.Name())
		// Sanity: status must be a string (the closed vocabulary), not an
		// arbitrary value. Catches accidental log.Int / log.Any drift.
		_, isString := statusField.Value.(string)
		assert.True(t, isString,
			"status field Value must be a string, got %T", statusField.Value)
	}
}
