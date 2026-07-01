// Copyright (c) 2026 Lerian Studio.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package observability

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedHistogramBuckets is the canonical bucket boundary set for
// readyz_check_duration_ms. The contract pins these values — any drift breaks
// downstream dashboards / alerts that hard-code these thresholds.
var expectedHistogramBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}

// newRecorderWithRegistry builds a Recorder backed by a fresh per-test
// Prometheus registry. Returning the registry lets the test scrape the
// bridged metrics directly without leaking series into the package-global
// default registry. This pattern keeps every test independent — there is no
// shared mutable state between cases.
func newRecorderWithRegistry(t *testing.T) (*Recorder, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()

	factory, shutdown, err := NewPrometheusBackedFactory(reg, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = shutdown()
	})

	return NewRecorder(factory, nil), reg
}

// gather runs reg.Gather() and returns metric families keyed by name.
// Wrapping the gather call in a helper keeps the assertion-side of every
// test focused on the metric semantics rather than the registry mechanics.
func gather(t *testing.T, reg *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	out := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		out[f.GetName()] = f
	}

	return out
}

// findHistogramSampleCount returns the cumulative observation count for the
// (dep, status) pair in mf. Returns 0 when the series is not present —
// matches the "deltas not absolutes" pattern used elsewhere in the suite.
func findHistogramSampleCount(mf *dto.MetricFamily, dep, status string) uint64 {
	for _, m := range mf.GetMetric() {
		if !labelsMatch(m.GetLabel(), dep, status) {
			continue
		}

		if h := m.GetHistogram(); h != nil {
			return h.GetSampleCount()
		}
	}

	return 0
}

// findCounterValue returns the value of the counter series matching
// (dep, status). Returns 0 when the series is not present.
func findCounterValue(mf *dto.MetricFamily, dep, status string) float64 {
	for _, m := range mf.GetMetric() {
		if !labelsMatch(m.GetLabel(), dep, status) {
			continue
		}

		if c := m.GetCounter(); c != nil {
			return c.GetValue()
		}
	}

	return 0
}

// findGaugeValue returns the gauge value for the dep series. Returns NaN
// equivalent (0) when the series has not been emitted yet — callers that
// want to distinguish "never emitted" from "emitted as 0" should assert on
// presence first via gather().
func findGaugeValue(mf *dto.MetricFamily, dep string) float64 {
	for _, m := range mf.GetMetric() {
		if !labelsMatch(m.GetLabel(), dep, "") {
			continue
		}

		if g := m.GetGauge(); g != nil {
			return g.GetValue()
		}
	}

	return 0
}

// labelsMatch returns true when the metric's labels contain dep=<dep> and
// (when status is non-empty) status=<status>. Passing status="" makes the
// status component optional — useful for the gauge series which has no
// status label.
func labelsMatch(labels []*dto.LabelPair, dep, status string) bool {
	var gotDep, gotStatus string

	for _, l := range labels {
		switch l.GetName() {
		case "dep":
			gotDep = l.GetValue()
		case "status":
			gotStatus = l.GetValue()
		}
	}

	if gotDep != dep {
		return false
	}

	if status == "" {
		return true
	}

	return gotStatus == status
}

// TestEmitCheckDuration_RecordsObservation verifies that every call to
// EmitCheckDuration produces an observation in the histogram for the supplied
// (dep, status) label pair.
func TestEmitCheckDuration_RecordsObservation(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)

	const dep = "postgres"

	const status = "degraded"

	ctx := context.Background()

	r.EmitCheckDuration(ctx, dep, status, 12*time.Millisecond)
	r.EmitCheckDuration(ctx, dep, status, 27*time.Millisecond)

	families := gather(t, reg)

	mf, ok := families["readyz_check_duration_ms"]
	require.True(t, ok, "histogram MetricFamily must be present after emission")

	count := findHistogramSampleCount(mf, dep, status)
	assert.EqualValues(t, 2, count,
		"expected exactly 2 observations for (%s,%s)", dep, status)
}

// TestEmitCheckDuration_RejectsInvalidLabels verifies the bounded-cardinality
// contract: arbitrary dep/status values are dropped at the emission point so
// typos can never create new label series.
func TestEmitCheckDuration_RejectsInvalidLabels(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)
	ctx := context.Background()

	// Invalid dep — dropped.
	r.EmitCheckDuration(ctx, "not_a_real_dep", "up", 1*time.Millisecond)
	// Invalid status — dropped.
	r.EmitCheckDuration(ctx, "postgres", "weird_status", 1*time.Millisecond)

	families := gather(t, reg)

	if mf, ok := families["readyz_check_duration_ms"]; ok {
		assert.Empty(t, mf.GetMetric(),
			"invalid (dep,status) tuples must not create new series")
	}
}

// TestEmitCheckStatus_Increments verifies counter behaviour: each call adds 1
// to the (dep, status) child series, and different status values produce
// distinct child series.
func TestEmitCheckStatus_Increments(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)
	ctx := context.Background()

	const dep = "rule_cache"

	for i := 0; i < 3; i++ {
		r.EmitCheckStatus(ctx, dep, "up")
	}

	r.EmitCheckStatus(ctx, dep, "down")

	families := gather(t, reg)

	mf, ok := families["readyz_check_status"]
	require.True(t, ok, "counter MetricFamily must be present after emission")

	upVal := findCounterValue(mf, dep, "up")
	downVal := findCounterValue(mf, dep, "down")

	assert.EqualValues(t, 3, upVal, "expected up counter to be 3")
	assert.EqualValues(t, 1, downVal, "expected down counter to be 1")
}

// TestEmitSelfProbeResult_SetsGauge verifies that the selfprobe gauge
// reflects exactly the last reported value (1 for up, 0 for down).
func TestEmitSelfProbeResult_SetsGauge(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)
	ctx := context.Background()

	const dep = "postgres"

	r.EmitSelfProbeResult(ctx, dep, true)

	families := gather(t, reg)

	mf, ok := families["selfprobe_result"]
	require.True(t, ok, "gauge MetricFamily must be present after emission")
	assert.EqualValues(t, 1, findGaugeValue(mf, dep), "expected gauge to be 1 after up")

	r.EmitSelfProbeResult(ctx, dep, false)

	families = gather(t, reg)
	mf = families["selfprobe_result"]
	assert.EqualValues(t, 0, findGaugeValue(mf, dep), "expected gauge to be 0 after down")
}

// TestMetrics_ScrapeExposesAllThreeNames verifies the scrape contract:
// after at least one observation per metric, the bridged Prometheus
// registry surfaces exactly the canonical metric names (no `_total` /
// `_milliseconds` suffix appended by the OTel exporter).
func TestMetrics_ScrapeExposesAllThreeNames(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)
	ctx := context.Background()

	r.EmitCheckDuration(ctx, "postgres", "up", 1*time.Millisecond)
	r.EmitCheckStatus(ctx, "postgres", "up")
	r.EmitSelfProbeResult(ctx, "postgres", true)

	families := gather(t, reg)

	want := []string{
		"readyz_check_duration_ms",
		"readyz_check_status",
		"selfprobe_result",
	}

	for _, name := range want {
		_, ok := families[name]
		assert.True(t, ok, "metric %q must be present in the bridged registry", name)
	}
}

// TestMetrics_HistogramBuckets locks the bucket boundaries of
// readyz_check_duration_ms. Bucket drift breaks dashboards and SLO alerts that
// hard-code these thresholds — the boundaries are part of the metric's public
// contract.
func TestMetrics_HistogramBuckets(t *testing.T) {
	r, reg := newRecorderWithRegistry(t)
	ctx := context.Background()

	// Force at least one observation so the histogram surfaces in Gather().
	r.EmitCheckDuration(ctx, "postgres", "up", 1*time.Millisecond)

	families := gather(t, reg)

	mf, ok := families["readyz_check_duration_ms"]
	require.True(t, ok, "histogram MetricFamily must be present after emission")

	require.GreaterOrEqual(t, len(mf.GetMetric()), 1,
		"histogram MetricFamily must contain at least one metric")

	h := mf.GetMetric()[0].GetHistogram()
	require.NotNil(t, h, "first metric must be a Histogram")

	// The OTel exporter emits a synthetic +Inf bucket on top of the explicit
	// bucket boundaries — strip it before comparing against the canonical set.
	got := make([]float64, 0, len(h.GetBucket()))
	for _, b := range h.GetBucket() {
		ub := b.GetUpperBound()

		// math.IsInf without importing math: +Inf > any finite float.
		if ub > expectedHistogramBuckets[len(expectedHistogramBuckets)-1] {
			continue
		}

		got = append(got, ub)
	}

	assert.Equal(t, expectedHistogramBuckets, got,
		"histogram bucket boundaries must match the canonical contract")
}

// TestNilRecorder_NoOps verifies that a nil-receiver Recorder silently
// drops every emission. This is the contract the call sites depend on:
// HealthChecker / RunSelfProbe can be exercised in unit tests without
// wiring a recorder, and production keeps a guaranteed-non-nil recorder
// from observability.NewNopRecorder when factory construction fails.
func TestNilRecorder_NoOps(t *testing.T) {
	var r *Recorder

	// Should not panic and should not allocate any metrics.
	r.EmitCheckDuration(context.Background(), "postgres", "up", 1*time.Millisecond)
	r.EmitCheckStatus(context.Background(), "postgres", "up")
	r.EmitSelfProbeResult(context.Background(), "postgres", true)
}

// TestNopRecorder_AllowsCallsButProducesNoSeries verifies that
// NewNopRecorder is safe in tests that don't wire a Prometheus bridge.
// The smoke-test on a separate per-test registry asserts that the no-op
// path doesn't accidentally register collectors with a side-channel.
func TestNopRecorder_AllowsCallsButProducesNoSeries(t *testing.T) {
	r := NewNopRecorder()
	require.NotNil(t, r)

	ctx := context.Background()

	r.EmitCheckDuration(ctx, "postgres", "up", 1*time.Millisecond)
	r.EmitCheckStatus(ctx, "postgres", "up")
	r.EmitSelfProbeResult(ctx, "postgres", true)

	// No-op recorder is built on libMetrics.NewNopFactory — the underlying
	// metrics never reach a registry. Confirm by asserting the package-default
	// registry's gather output mentions none of the three metric names.
	gathered, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	names := make([]string, 0, len(gathered))
	for _, mf := range gathered {
		names = append(names, mf.GetName())
	}

	joined := strings.Join(names, ",")

	assert.NotContains(t, joined, "readyz_check_duration_ms")
	assert.NotContains(t, joined, "readyz_check_status")
	assert.NotContains(t, joined, "selfprobe_result")
}
