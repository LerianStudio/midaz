// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// findDsMetric is a tiny test helper that locates a Metric by name within a
// ResourceMetrics struct. Defined here so we keep the datasource metric tests
// in their own file (test names start with TestDatasourceMetrics_*).
func findDsMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
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

func newDsReaderAndMetrics(t *testing.T) (*sdkmetric.ManualReader, *DatasourceMetrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("ds-metrics-test")

	m, err := NewDatasourceMetrics(meter)
	require.NoError(t, err)
	require.NotNil(t, m)

	return reader, m
}

// TestDatasourceMetrics_NilMeter_ReturnsNoopBacked verifies that a nil meter
// produces a Metrics that is safe to call (records to noop instruments).
func TestDatasourceMetrics_NilMeter_ReturnsNoopBacked(t *testing.T) {
	t.Parallel()

	m, err := NewDatasourceMetrics(nil)
	require.NoError(t, err)
	require.NotNil(t, m)

	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.EmitDatasourceHealthy(ctx, "pg_main", true)
	})
	assert.NotPanics(t, func() {
		m.EmitDatasourceCheckDuration(ctx, "pg_main", 5*time.Millisecond)
	})
}

// TestDatasourceMetrics_NilReceiver_DoesNotPanic guarantees that emitter
// methods on a nil *DatasourceMetrics are no-ops, mirroring the readyz
// Metrics safety contract. This lets callers wire metrics conditionally
// without scattering nil-check boilerplate.
func TestDatasourceMetrics_NilReceiver_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var m *DatasourceMetrics

	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.EmitDatasourceHealthy(ctx, "pg_main", true)
	})
	assert.NotPanics(t, func() {
		m.EmitDatasourceCheckDuration(ctx, "pg_main", time.Millisecond)
	})
}

// TestDatasourceMetrics_RealMeter_RegistersBothInstruments verifies that
// both metric families appear after a single emission round.
func TestDatasourceMetrics_RealMeter_RegistersBothInstruments(t *testing.T) {
	t.Parallel()

	reader, m := newDsReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitDatasourceHealthy(ctx, "pg_main", true)
	m.EmitDatasourceCheckDuration(ctx, "pg_main", 12*time.Millisecond)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	_, healthyOK := findDsMetric(t, rm, "datasource_healthy")
	_, durOK := findDsMetric(t, rm, "datasource_check_duration_ms")

	assert.True(t, healthyOK, "datasource_healthy must be registered")
	assert.True(t, durOK, "datasource_check_duration_ms must be registered")
}

// TestDatasourceMetrics_HealthyGauge_ReflectsLatestState verifies gauge
// semantics for datasource_healthy: subsequent records overwrite the previous
// value (per-datasource_id).
func TestDatasourceMetrics_HealthyGauge_ReflectsLatestState(t *testing.T) {
	t.Parallel()

	reader, m := newDsReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitDatasourceHealthy(ctx, "pg_main", true)
	m.EmitDatasourceHealthy(ctx, "pg_main", false)
	m.EmitDatasourceHealthy(ctx, "pg_main", true)
	m.EmitDatasourceHealthy(ctx, "mongo_orders", false)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findDsMetric(t, rm, "datasource_healthy")
	require.True(t, ok)

	gauge, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "data type must be Gauge[int64], got %T", got.Data)
	require.Len(t, gauge.DataPoints, 2)

	values := make(map[string]int64)

	for _, dp := range gauge.DataPoints {
		ds, _ := dp.Attributes.Value("datasource_id")
		values[ds.AsString()] = dp.Value
	}

	assert.Equal(t, int64(1), values["pg_main"])
	assert.Equal(t, int64(0), values["mongo_orders"])
}

// TestDatasourceMetrics_HistogramBucketsArePinned verifies that
// datasource_check_duration_ms is registered with the explicit bucket
// boundaries declared in datasource_metrics.go (which intentionally match
// the readyz boundaries). Regression guard against a refactor silently
// dropping the metric.WithExplicitBucketBoundaries call.
//
// Test-reviewer H5 (companion assertion to the readyz one).
func TestDatasourceMetrics_HistogramBucketsArePinned(t *testing.T) {
	t.Parallel()

	reader, m := newDsReaderAndMetrics(t)

	m.EmitDatasourceCheckDuration(context.Background(), "pg_main", 12*time.Millisecond)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	got, ok := findDsMetric(t, rm, "datasource_check_duration_ms")
	require.True(t, ok, "duration histogram must be registered")

	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "data type must be Histogram[float64], got %T", got.Data)
	require.Len(t, hist.DataPoints, 1)

	expected := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}
	assert.Equal(t, expected, hist.DataPoints[0].Bounds,
		"explicit bucket boundaries must match the canonical contract (mirrors readyz)")
}

// TestDatasourceMetrics_DurationHistogram_RecordsObservations verifies that
// EmitDatasourceCheckDuration records into the histogram with the
// datasource_id label set, and that the observed sum is in milliseconds.
func TestDatasourceMetrics_DurationHistogram_RecordsObservations(t *testing.T) {
	t.Parallel()

	reader, m := newDsReaderAndMetrics(t)

	ctx := context.Background()
	m.EmitDatasourceCheckDuration(ctx, "pg_main", 7*time.Millisecond)
	m.EmitDatasourceCheckDuration(ctx, "pg_main", 13*time.Millisecond)

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(ctx, &rm))

	got, ok := findDsMetric(t, rm, "datasource_check_duration_ms")
	require.True(t, ok)

	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "data type must be Histogram[float64], got %T", got.Data)
	require.Len(t, hist.DataPoints, 1)

	dp := hist.DataPoints[0]
	assert.Equal(t, uint64(2), dp.Count)
	assert.InDelta(t, 20.0, dp.Sum, 0.001)

	dsID, ok := dp.Attributes.Value("datasource_id")
	require.True(t, ok)
	assert.Equal(t, "pg_main", dsID.AsString())
}
