// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newRecordingTracerContext returns a context carrying a real SDK tracer whose
// spans are captured by the returned SpanRecorder. Service code extracts the
// tracer via libCommons.NewTrackingFromContext, so injecting it through
// libCommons.ContextWithTracer exercises the production span path. Telemetry is
// fully in-process.
func newRecordingTracerContext(t *testing.T, ctx context.Context) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := provider.Tracer("encryption-test")

	return libCommons.ContextWithTracer(ctx, tracer), recorder
}

// spanByName returns the first ended span with the given name, or nil.
func spanByName(recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return s
		}
	}

	return nil
}

// spanStringAttr returns the string value of the named span attribute and
// whether it was present.
func spanStringAttr(s sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString(), true
		}
	}

	return "", false
}

// spanHasError reports whether the span recorded an error status.
func spanHasError(s sdktrace.ReadOnlySpan) bool {
	return s.Status().Code == codes.Error
}

// recordingMetrics is a test seam that captures real metric emissions so tests
// can assert counter name + attribute values. The protectionMetrics factory is
// a concrete *metrics.MetricsFactory backed by an OTel meter (NewNopFactory
// cannot capture emissions), so the spy wires a real SDK MeterProvider with a
// ManualReader and builds the factory over its meter. Collecting the
// ManualReader yields the exact counters and attributes emitted through the
// production path.
type recordingMetrics struct {
	reader  *metric.ManualReader
	factory *metrics.MetricsFactory
}

// newRecordingMetrics builds a protectionMetrics-compatible factory whose
// emissions are captured by an in-memory ManualReader. Telemetry is fully
// in-process; no exporter or network is involved.
func newRecordingMetrics(t *testing.T) (*protectionMetrics, *recordingMetrics) {
	t.Helper()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	factory, err := metrics.NewMetricsFactory(provider.Meter("encryption-test"), nil)
	if err != nil {
		t.Fatalf("NewMetricsFactory() error = %v", err)
	}

	return NewProtectionMetrics(factory), &recordingMetrics{reader: reader, factory: factory}
}

// counterSample is a single emitted (counter, attributes) data point.
type counterSample struct {
	name  string
	value int64
	attrs map[string]string
}

// collect drains the ManualReader and flattens every counter data point into
// counterSample values for assertion.
func (r *recordingMetrics) collect(t *testing.T) []counterSample {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := r.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("ManualReader.Collect() error = %v", err)
	}

	var samples []counterSample

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}

			for _, dp := range sum.DataPoints {
				attrs := make(map[string]string)

				for _, kv := range dp.Attributes.ToSlice() {
					attrs[string(kv.Key)] = kv.Value.Emit()
				}

				samples = append(samples, counterSample{name: m.Name, value: dp.Value, attrs: attrs})
			}
		}
	}

	return samples
}

// counterValue returns the summed value of the named counter whose attributes
// contain every wantAttr key/value pair, and the count of matching data points.
func (r *recordingMetrics) counterValue(t *testing.T, name string, wantAttr map[string]string) (int64, int) {
	t.Helper()

	var (
		total   int64
		matches int
	)

	for _, s := range r.collect(t) {
		if s.name != name {
			continue
		}

		if !attrsContain(s.attrs, wantAttr) {
			continue
		}

		total += s.value
		matches++
	}

	return total, matches
}

func attrsContain(got, want map[string]string) bool {
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}

	return true
}

// counterCount returns the number of data points emitted for the named counter,
// regardless of attributes.
func (r *recordingMetrics) counterCount(t *testing.T, name string) int {
	t.Helper()

	var n int

	for _, s := range r.collect(t) {
		if s.name == name {
			n++
		}
	}

	return n
}

// histogramCount returns the total number of recorded observations for the named
// int64 histogram whose data-point attributes contain every wantAttr key/value
// pair. Each Record() call increments a data point's Count, so summing Count over
// matching data points yields the number of timing samples emitted.
func (r *recordingMetrics) histogramCount(t *testing.T, name string, wantAttr map[string]string) int {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := r.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("ManualReader.Collect() error = %v", err)
	}

	var total int

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			hist, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				continue
			}

			for _, dp := range hist.DataPoints {
				attrs := make(map[string]string)
				for _, kv := range dp.Attributes.ToSlice() {
					attrs[string(kv.Key)] = kv.Value.Emit()
				}

				if attrsContain(attrs, wantAttr) {
					total += int(dp.Count)
				}
			}
		}
	}

	return total
}

// histogramSum returns the summed recorded value (metricdata Sum field) across
// every data point of the named int64 histogram whose attributes contain every
// wantAttr key/value pair. It proves the recorded magnitude (e.g. milliseconds),
// not just the observation count, so a value > 0 confirms sub-second durations
// are no longer truncated to zero.
func (r *recordingMetrics) histogramSum(t *testing.T, name string, wantAttr map[string]string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := r.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("ManualReader.Collect() error = %v", err)
	}

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			hist, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				continue
			}

			for _, dp := range hist.DataPoints {
				attrs := make(map[string]string)
				for _, kv := range dp.Attributes.ToSlice() {
					attrs[string(kv.Key)] = kv.Value.Emit()
				}

				if attrsContain(attrs, wantAttr) {
					total += dp.Sum
				}
			}
		}
	}

	return total
}

// assertNoCounter fails if any data point for the named counter was emitted.
func (r *recordingMetrics) assertNoCounter(t *testing.T, name string) {
	t.Helper()

	for _, s := range r.collect(t) {
		if s.name == name {
			t.Fatalf("expected no emission for counter %q, got value=%d attrs=%v", name, s.value, s.attrs)
		}
	}
}
