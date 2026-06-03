// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// findMetric returns the first Metric whose Name == name across all ScopeMetrics.
// It is a small test helper that lets us assert presence/values without depending
// on metric ordering, which the SDK does not guarantee across exports.
func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}

	return metricdata.Metrics{}, false
}

// newReaderAndMetrics creates a manual reader, meter provider, and Metrics
// instance, returning them so tests can collect emitted points after exercising
// the API.
func newReaderAndMetrics(t *testing.T) (*sdkmetric.ManualReader, *Metrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("readyz-test")

	m, err := NewMetrics(meter)
	require.NoError(t, err)
	require.NotNil(t, m)

	return reader, m
}

// TestNewMetrics_NilMeter_ReturnsNoopMetrics verifies that passing a nil meter
// returns a Metrics that is safe to call (records to a noop meter).
func TestNewMetrics_NilMeter_ReturnsNoopMetrics(t *testing.T) {
	t.Parallel()

	m, err := NewMetrics(nil)
	require.NoError(t, err)
	require.NotNil(t, m)

	// All emit methods MUST be safe with the noop fallback (no panic).
	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.EmitCheckDuration(ctx, "mongo", StatusUp, 10*time.Millisecond)
	})
	assert.NotPanics(t, func() {
		m.EmitCheckStatus(ctx, "mongo", StatusUp)
	})
	assert.NotPanics(t, func() {
		m.EmitSelfProbeResult(ctx, "mongo", true)
	})
}

// TestMetrics_NilReceiver_DoesNotPanic guarantees that nil-receiver Emit calls
// are no-ops. This means handlers/probes that have no Metrics injected (legacy
// call sites or tests) cannot crash by calling the emit helpers.
func TestMetrics_NilReceiver_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var m *Metrics

	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.EmitCheckDuration(ctx, "mongo", StatusUp, time.Millisecond)
	})
	assert.NotPanics(t, func() {
		m.EmitCheckStatus(ctx, "mongo", StatusUp)
	})
	assert.NotPanics(t, func() {
		m.EmitSelfProbeResult(ctx, "mongo", true)
	})
}

// TestNewMetrics_RealMeter_RegistersAllInstruments verifies that the three
// instruments are registered on the provided meter and that their attribute
// keys are dep and status (where applicable). We use a manual reader to
// inspect emitted data points.
func TestNewMetrics_RealMeter_RegistersAllInstruments(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitCheckDuration(ctx, "mongo", StatusUp, 25*time.Millisecond)
	m.EmitCheckStatus(ctx, "mongo", StatusUp)
	m.EmitSelfProbeResult(ctx, "mongo", true)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	// All three metric families must exist after a single emission round.
	_, durOK := findMetric(t, rm, "readyz_check_duration_ms")
	_, statusOK := findMetric(t, rm, "readyz_check_status")
	_, probeOK := findMetric(t, rm, "selfprobe_result")

	assert.True(t, durOK, "readyz_check_duration_ms must be registered")
	assert.True(t, statusOK, "readyz_check_status must be registered")
	assert.True(t, probeOK, "selfprobe_result must be registered")
}

// TestEmitCheckDuration_RecordsObservation verifies that EmitCheckDuration
// records a single observation per call with the correct attribute keys
// (dep, status) and value (in milliseconds).
func TestEmitCheckDuration_RecordsObservation(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitCheckDuration(ctx, "mongo", StatusUp, 42*time.Millisecond)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findMetric(t, rm, "readyz_check_duration_ms")
	require.True(t, ok, "duration metric must be present")

	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "data type must be Histogram[float64], got %T", got.Data)
	require.Len(t, hist.DataPoints, 1, "exactly one data point expected")

	dp := hist.DataPoints[0]
	assert.Equal(t, uint64(1), dp.Count, "one observation expected")
	assert.InDelta(t, 42.0, dp.Sum, 0.001, "sum must be 42 ms")

	dep, depOK := dp.Attributes.Value("dep")
	require.True(t, depOK, "dep attribute must exist")
	assert.Equal(t, "mongo", dep.AsString())

	status, statusOK := dp.Attributes.Value("status")
	require.True(t, statusOK, "status attribute must exist")
	assert.Equal(t, "up", status.AsString())
}

// TestMetrics_HistogramBucketsArePinned verifies that
// readyz_check_duration_ms is registered with the canonical explicit
// bucket boundaries from metrics.go (1, 5, 10, 25, 50, 100, 250, 500,
// 1000, 2000, 5000) and not the OTel SDK default buckets.
//
// This is a regression guard: if a future refactor drops the
// metric.WithExplicitBucketBoundaries call (e.g., switches to default
// buckets), the existing Count/Sum tests would still pass but dashboards
// keying on these specific bucket boundaries would silently return wrong
// p95/p99 results. Pinning the bucket set explicitly makes that drift
// catchable in unit tests.
//
// Test-reviewer H5.
func TestMetrics_HistogramBucketsArePinned(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	// One observation is enough — bucket boundaries are part of the
	// instrument registration metadata, not derived from the data.
	m.EmitCheckDuration(context.Background(), "mongo", StatusUp, 12*time.Millisecond)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	got, ok := findMetric(t, rm, "readyz_check_duration_ms")
	require.True(t, ok, "duration histogram must be registered")

	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "data type must be Histogram[float64], got %T", got.Data)
	require.Len(t, hist.DataPoints, 1)

	expected := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}
	assert.Equal(t, expected, hist.DataPoints[0].Bounds,
		"explicit bucket boundaries must match the canonical readyz contract")
}

// TestEmitCheckStatus_IncrementsCounter verifies that consecutive
// EmitCheckStatus calls accumulate as a counter (additive monotonic), and
// that distinct (dep, status) tuples produce separate data points.
func TestEmitCheckStatus_IncrementsCounter(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitCheckStatus(ctx, "mongo", StatusUp)
	m.EmitCheckStatus(ctx, "mongo", StatusUp)
	m.EmitCheckStatus(ctx, "mongo", StatusDown)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findMetric(t, rm, "readyz_check_status")
	require.True(t, ok)

	sum, ok := got.Data.(metricdata.Sum[int64])
	require.True(t, ok, "data type must be Sum[int64], got %T", got.Data)
	assert.True(t, sum.IsMonotonic, "counter must be monotonic")

	// Two data points: one for (mongo, up)=2 and one for (mongo, down)=1.
	require.Len(t, sum.DataPoints, 2)

	// Build a label-keyed map for assertion clarity.
	totals := make(map[string]int64)

	for _, dp := range sum.DataPoints {
		dep, _ := dp.Attributes.Value("dep")
		status, _ := dp.Attributes.Value("status")
		key := dep.AsString() + "/" + status.AsString()
		totals[key] = dp.Value
	}

	assert.Equal(t, int64(2), totals["mongo/up"])
	assert.Equal(t, int64(1), totals["mongo/down"])
}

// TestEmitSelfProbeResult_GaugeReflectsLatestState verifies that
// EmitSelfProbeResult sets the gauge to 1 when up and 0 when down, replacing
// the previous value (gauge semantics, not counter).
func TestEmitSelfProbeResult_GaugeReflectsLatestState(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()
	// Three transitions for "mongo": up -> down -> up. The gauge must reflect
	// only the latest value (1), not an aggregation.
	m.EmitSelfProbeResult(ctx, "mongo", true)
	m.EmitSelfProbeResult(ctx, "mongo", false)
	m.EmitSelfProbeResult(ctx, "mongo", true)

	// And one stable down for "redis".
	m.EmitSelfProbeResult(ctx, "redis", false)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findMetric(t, rm, "selfprobe_result")
	require.True(t, ok)

	gauge, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "data type must be Gauge[int64], got %T", got.Data)
	require.Len(t, gauge.DataPoints, 2, "expected one data point per dep")

	values := make(map[string]int64)

	for _, dp := range gauge.DataPoints {
		dep, _ := dp.Attributes.Value("dep")
		values[dep.AsString()] = dp.Value
	}

	assert.Equal(t, int64(1), values["mongo"], "mongo's latest state is up=1")
	assert.Equal(t, int64(0), values["redis"], "redis's latest state is down=0")
}

// TestEmitSelfProbeResult_NoStatusAttribute confirms we do NOT add a status
// label to selfprobe_result. The contract is dep-only with the value (1=up,
// 0=down) carrying the meaning. This guards against accidental cardinality
// growth and matches the contract in pkg/readyz/SKILL.md.
func TestEmitSelfProbeResult_NoStatusAttribute(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitSelfProbeResult(ctx, "mongo", true)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findMetric(t, rm, "selfprobe_result")
	require.True(t, ok)

	gauge := got.Data.(metricdata.Gauge[int64])
	require.Len(t, gauge.DataPoints, 1)

	_, statusOK := gauge.DataPoints[0].Attributes.Value("status")
	assert.False(t, statusOK, "selfprobe_result must NOT carry a status label")
}

// TestMetrics_ConcurrentEmissionRaceFree exercises all three emit helpers from
// many goroutines concurrently. The race detector (go test -race) must not
// flag this test, and after the goroutines join the recorded counts MUST be
// deterministic.
func TestMetrics_ConcurrentEmissionRaceFree(t *testing.T) {
	t.Parallel()

	reader, m := newReaderAndMetrics(t)

	ctx := context.Background()

	const (
		workers          = 8
		iterationsPerDep = 25
	)

	var wg sync.WaitGroup

	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterationsPerDep; j++ {
				m.EmitCheckDuration(ctx, "mongo", StatusUp, time.Millisecond)
				m.EmitCheckStatus(ctx, "mongo", StatusUp)
				m.EmitSelfProbeResult(ctx, "mongo", true)
			}
		}()
	}

	wg.Wait()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	statusGot, ok := findMetric(t, rm, "readyz_check_status")
	require.True(t, ok)

	sum := statusGot.Data.(metricdata.Sum[int64])
	require.Len(t, sum.DataPoints, 1)
	assert.Equal(t, int64(workers*iterationsPerDep), sum.DataPoints[0].Value)

	durGot, ok := findMetric(t, rm, "readyz_check_duration_ms")
	require.True(t, ok)

	hist := durGot.Data.(metricdata.Histogram[float64])
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(workers*iterationsPerDep), hist.DataPoints[0].Count)
}
