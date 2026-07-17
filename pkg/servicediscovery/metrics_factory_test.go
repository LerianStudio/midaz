// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestMetricsFactory builds a real *metrics.MetricsFactory backed by an
// in-memory OTel meter provider (no exporter), mirroring the repo's bootstrap
// tests. It lets the factory-backed recorder exercise the real Counter/Histogram
// paths without a live telemetry pipeline.
func newTestMetricsFactory(t *testing.T) *metrics.MetricsFactory {
	t.Helper()

	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("servicediscovery_test")

	factory, err := metrics.NewMetricsFactory(meter, nil)
	if err != nil {
		t.Fatalf("NewMetricsFactory returned error: %v", err)
	}

	return factory
}

// newReaderBackedFactory builds a *metrics.MetricsFactory whose meter is wired to
// a ManualReader, so tests can Collect and assert on the emitted metricdata.
// Mirrors the repo pattern of driving a factory over an in-memory meter provider.
func newReaderBackedFactory(t *testing.T) (*metrics.MetricsFactory, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("servicediscovery_observe_test")

	factory, err := metrics.NewMetricsFactory(meter, nil)
	if err != nil {
		t.Fatalf("NewMetricsFactory returned error: %v", err)
	}

	return factory, reader
}

// collect gathers the current metrics from the reader and returns them keyed by
// metric name for assertion.
func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("reader.Collect returned error: %v", err)
	}

	out := make(map[string]metricdata.Metrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}

	return out
}

// attrValue returns the string value of key in the datapoint's attribute set,
// and whether it was present.
func attrValue(set attribute.Set, key string) (string, bool) {
	v, ok := set.Value(attribute.Key(key))
	return v.AsString(), ok
}

// requireSumDataPoint returns the single int64 Sum datapoint for the named metric,
// failing if the metric is absent, not a Sum, or does not have exactly one point.
func requireSumDataPoint(t *testing.T, metricsByName map[string]metricdata.Metrics, name string) metricdata.DataPoint[int64] {
	t.Helper()

	m, ok := metricsByName[name]
	if !ok {
		t.Fatalf("metric %q not emitted; got %v", name, metricNames(metricsByName))
	}

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q has data type %T; want metricdata.Sum[int64]", name, m.Data)
	}

	if len(sum.DataPoints) != 1 {
		t.Fatalf("metric %q has %d datapoints; want exactly 1", name, len(sum.DataPoints))
	}

	return sum.DataPoints[0]
}

// requireHistogramDataPoint returns the single int64 Histogram datapoint for the
// named metric, failing if the metric is absent, not a Histogram, or does not
// have exactly one point.
func requireHistogramDataPoint(t *testing.T, metricsByName map[string]metricdata.Metrics, name string) metricdata.HistogramDataPoint[int64] {
	t.Helper()

	m, ok := metricsByName[name]
	if !ok {
		t.Fatalf("metric %q not emitted; got %v", name, metricNames(metricsByName))
	}

	hist, ok := m.Data.(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("metric %q has data type %T; want metricdata.Histogram[int64]", name, m.Data)
	}

	if len(hist.DataPoints) != 1 {
		t.Fatalf("metric %q has %d datapoints; want exactly 1", name, len(hist.DataPoints))
	}

	return hist.DataPoints[0]
}

func metricNames(metricsByName map[string]metricdata.Metrics) []string {
	names := make([]string, 0, len(metricsByName))
	for n := range metricsByName {
		names = append(names, n)
	}

	return names
}

func TestNewMetricsFactoryRecorder_NilFactoryReturnsNop(t *testing.T) {
	r := NewMetricsFactoryRecorder(nil, libLog.NewNop())
	if r == nil {
		t.Fatal("NewMetricsFactoryRecorder(nil, ...) returned nil; want non-nil no-op recorder")
	}

	if _, ok := r.(NopMetricsRecorder); !ok {
		t.Fatalf("NewMetricsFactoryRecorder(nil, ...) returned %T; want NopMetricsRecorder", r)
	}

	// The returned no-op recorder must be callable without panic.
	ctx := context.Background()
	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultError)
	r.ResolveResult(ctx, "plugin-auth", ResultFallback, 3)
}

func TestNewMetricsFactoryRecorder_NonNilFactoryImplementsRecorder(t *testing.T) {
	factory := newTestMetricsFactory(t)

	r := NewMetricsFactoryRecorder(factory, libLog.NewNop())
	if r == nil {
		t.Fatal("NewMetricsFactoryRecorder(factory, ...) returned nil; want a recorder")
	}

	if _, ok := r.(NopMetricsRecorder); ok {
		t.Fatal("NewMetricsFactoryRecorder(factory, ...) returned NopMetricsRecorder; want the factory-backed recorder")
	}
}

func TestMetricsFactoryRecorder_RegisterInitiated_EmitsCounter(t *testing.T) {
	factory, reader := newReaderBackedFactory(t)
	r := NewMetricsFactoryRecorder(factory, libLog.NewNop())
	ctx := context.Background()

	r.RegisterInitiated(ctx)

	dp := requireSumDataPoint(t, collect(t, reader), "sd_register_total")

	if dp.Value != 1 {
		t.Errorf("sd_register_total value = %d; want 1", dp.Value)
	}

	if dp.Attributes.Len() != 0 {
		t.Errorf("sd_register_total has %d attributes; want 0 (register is unlabeled)", dp.Attributes.Len())
	}
}

func TestMetricsFactoryRecorder_DeregisterResult_EmitsLabeledCounter(t *testing.T) {
	tests := []struct {
		name   string
		result string
	}{
		{name: "ok", result: ResultOK},
		{name: "error", result: ResultError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, reader := newReaderBackedFactory(t)
			r := NewMetricsFactoryRecorder(factory, libLog.NewNop())

			r.DeregisterResult(context.Background(), tt.result)

			dp := requireSumDataPoint(t, collect(t, reader), "sd_deregister_total")

			if dp.Value != 1 {
				t.Errorf("sd_deregister_total value = %d; want 1", dp.Value)
			}

			got, ok := attrValue(dp.Attributes, "result")
			if !ok {
				t.Fatalf("sd_deregister_total datapoint missing %q attribute", "result")
			}

			if got != tt.result {
				t.Errorf("sd_deregister_total result = %q; want %q", got, tt.result)
			}
		})
	}
}

func TestMetricsFactoryRecorder_ResolveResult_EmitsCounterAndHistogram(t *testing.T) {
	const (
		service    = "plugin-auth"
		durationMs = int64(12)
	)

	factory, reader := newReaderBackedFactory(t)
	r := NewMetricsFactoryRecorder(factory, libLog.NewNop())

	r.ResolveResult(context.Background(), service, ResultResolved, durationMs)

	metricsByName := collect(t, reader)

	// Counter: sd_resolve_total{service,result} == 1
	countDP := requireSumDataPoint(t, metricsByName, "sd_resolve_total")

	if countDP.Value != 1 {
		t.Errorf("sd_resolve_total value = %d; want 1", countDP.Value)
	}

	if got, ok := attrValue(countDP.Attributes, "service"); !ok || got != service {
		t.Errorf("sd_resolve_total service = %q (present=%t); want %q", got, ok, service)
	}

	if got, ok := attrValue(countDP.Attributes, "result"); !ok || got != ResultResolved {
		t.Errorf("sd_resolve_total result = %q (present=%t); want %q", got, ok, ResultResolved)
	}

	// Histogram: sd_resolve_duration_milliseconds{service} count==1, sum==durationMs
	histDP := requireHistogramDataPoint(t, metricsByName, "sd_resolve_duration_milliseconds")

	if histDP.Count != 1 {
		t.Errorf("sd_resolve_duration_milliseconds count = %d; want 1", histDP.Count)
	}

	if histDP.Sum != durationMs {
		t.Errorf("sd_resolve_duration_milliseconds sum = %d; want %d", histDP.Sum, durationMs)
	}

	if got, ok := attrValue(histDP.Attributes, "service"); !ok || got != service {
		t.Errorf("sd_resolve_duration_milliseconds service = %q (present=%t); want %q", got, ok, service)
	}

	if _, ok := attrValue(histDP.Attributes, "result"); ok {
		t.Errorf("sd_resolve_duration_milliseconds must not carry a %q attribute", "result")
	}

	// Lock the ms/seconds contract: emitted bucket boundaries must match the
	// explicit ms buckets on the descriptor (not the seconds-scaled defaults).
	if !reflect.DeepEqual(histDP.Bounds, sdResolveDurationMsBuckets) {
		t.Errorf("sd_resolve_duration_milliseconds bounds = %v; want ms buckets %v", histDP.Bounds, sdResolveDurationMsBuckets)
	}
}

func TestMetricsFactoryRecorder_ResolveResult_LabelVariants(t *testing.T) {
	tests := []struct {
		name   string
		result string
	}{
		{name: "resolved", result: ResultResolved},
		{name: "fallback", result: ResultFallback},
		{name: "error", result: ResultError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, reader := newReaderBackedFactory(t)
			r := NewMetricsFactoryRecorder(factory, libLog.NewNop())

			r.ResolveResult(context.Background(), "midaz-ledger", tt.result, 5)

			dp := requireSumDataPoint(t, collect(t, reader), "sd_resolve_total")

			if got, ok := attrValue(dp.Attributes, "result"); !ok || got != tt.result {
				t.Errorf("sd_resolve_total result = %q (present=%t); want %q", got, ok, tt.result)
			}
		})
	}
}

func TestMetricsFactoryRecorder_NilLoggerDoesNotPanic(t *testing.T) {
	factory := newTestMetricsFactory(t)

	// A nil logger must not cause a panic on the happy path (the factory paths
	// succeed, so the logger is never invoked). This guards the construction
	// contract without depending on unreachable error branches.
	r := NewMetricsFactoryRecorder(factory, nil)
	ctx := context.Background()

	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultOK)
	r.ResolveResult(ctx, "plugin-auth", ResultResolved, 5)
}

func TestMetricsFactoryRecorder_SatisfiesInterface(t *testing.T) {
	var _ MetricsRecorder = (*metricsFactoryRecorder)(nil)
}

// TestMetricsFactoryRecorder_Warn exercises the Warn logging helper directly. The
// counter/histogram builder error branches are unreachable with a valid factory
// (factory.Counter/Histogram only error on meter-creation failure), so warn is
// tested via its own reachable branches: nil logger (no-op) and real logger.
func TestMetricsFactoryRecorder_Warn(t *testing.T) {
	factory := newTestMetricsFactory(t)
	ctx := context.Background()
	err := errors.New("boom")

	// Nil logger must short-circuit without panic.
	nilLoggerRec := &metricsFactoryRecorder{factory: factory, logger: nil}
	nilLoggerRec.warn(ctx, "should be dropped", err)

	// Real logger must accept the Warn call without panic.
	realLoggerRec := &metricsFactoryRecorder{factory: factory, logger: libLog.NewNop()}
	realLoggerRec.warn(ctx, "recorded at warn", err)
}

func TestSDMetricDescriptors(t *testing.T) {
	tests := []struct {
		name     string
		desc     metrics.Metric
		wantName string
		wantUnit string
	}{
		{
			name:     "register_total",
			desc:     sdRegisterTotal,
			wantName: "sd_register_total",
			wantUnit: "1",
		},
		{
			name:     "deregister_total",
			desc:     sdDeregisterTotal,
			wantName: "sd_deregister_total",
			wantUnit: "1",
		},
		{
			name:     "resolve_total",
			desc:     sdResolveTotal,
			wantName: "sd_resolve_total",
			wantUnit: "1",
		},
		{
			name:     "resolve_duration_milliseconds",
			desc:     sdResolveDurationMs,
			wantName: "sd_resolve_duration_milliseconds",
			wantUnit: "ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.desc.Name != tt.wantName {
				t.Errorf("Name = %q; want %q", tt.desc.Name, tt.wantName)
			}

			if tt.desc.Unit != tt.wantUnit {
				t.Errorf("Unit = %q; want %q", tt.desc.Unit, tt.wantUnit)
			}

			if tt.desc.Description == "" {
				t.Errorf("Description is empty; want a non-empty descriptor comment")
			}
		})
	}
}

// TestSDResolveDurationBuckets locks the resolve-duration histogram to explicit
// millisecond boundaries. Without them, MetricsFactory.selectDefaultBuckets picks
// seconds-scaled DefaultLatencyBuckets for any "duration" metric, so ms values
// collapse into +Inf and the distribution becomes unobservable.
func TestSDResolveDurationBuckets(t *testing.T) {
	wantBuckets := []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

	if !reflect.DeepEqual(sdResolveDurationMs.Buckets, wantBuckets) {
		t.Errorf("sdResolveDurationMs.Buckets = %v; want ms buckets %v", sdResolveDurationMs.Buckets, wantBuckets)
	}

	// The other descriptors are counters and must not carry histogram buckets.
	for _, m := range []metrics.Metric{sdRegisterTotal, sdDeregisterTotal, sdResolveTotal} {
		if m.Buckets != nil {
			t.Errorf("counter descriptor %q has Buckets %v; want nil", m.Name, m.Buckets)
		}
	}
}
