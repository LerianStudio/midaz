// Copyright (c) 2026 Lerian Studio.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package observability owns the OpenTelemetry-backed metrics surface for
// the /readyz dependency probe and the startup self-probe.
//
// Three NON-NEGOTIABLE metrics per the ring:dev-readyz canonical contract:
//   - readyz_check_duration_ms (Histogram, labels: dep, status) — per-probe latency
//   - readyz_check_status      (Counter,   labels: dep, status) — per-outcome count
//   - selfprobe_result         (Gauge,     labels: dep)         — startup probe (1=up,0=down)
//
// Cardinality is bounded: dep ∈ {postgres, rule_cache},
// status ∈ {up, down, degraded, skipped, n/a}. No per-tenant labels.
//
// The metrics are exposed on /metrics via an OpenTelemetry Prometheus exporter
// bridge wired in bootstrap (see prometheus_factory.go) so the operator
// scrape contract is preserved while the SDK migrates to OTel-native
// instrumentation. Metric names + label set are pinned by NoTranslation +
// "without suffixes" exporter options — drift here breaks dashboards and
// SLO alerts that hard-code the canonical names.
package observability

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
)

// allowedDeps and allowedStatuses bound the cardinality of the readyz
// label space. Any value outside these sets is silently dropped at the
// emission point — emitting metrics with arbitrary strings would let a
// typo create unbounded series and break the cardinality contract
// documented at the top of this file.
var (
	allowedDeps = map[string]struct{}{
		"postgres":   {},
		"rule_cache": {},
	}

	allowedStatuses = map[string]struct{}{
		"up":       {},
		"down":     {},
		"degraded": {},
		"skipped":  {},
		"n/a":      {},
	}
)

// isValidDep reports whether dep is in the canonical bounded set.
func isValidDep(dep string) bool {
	_, ok := allowedDeps[dep]

	return ok
}

// isValidStatus reports whether status is in the canonical bounded set.
func isValidStatus(status string) bool {
	_, ok := allowedStatuses[status]

	return ok
}

// Canonical metric definitions. Names and units are pinned — exporter is
// configured with UnderscoreEscapingWithoutSuffixes so no `_total`/`_milliseconds`
// suffix is appended. The bucket boundaries on metricCheckDuration are part
// of the metric's public contract: dashboards and SLO alerts hard-code these
// thresholds.
var (
	metricCheckDuration = libMetrics.Metric{
		Name:        "readyz_check_duration_ms",
		Unit:        "ms",
		Description: "Duration of /readyz dependency checks in milliseconds.",
		Buckets:     []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
	}

	metricCheckStatus = libMetrics.Metric{
		Name:        "readyz_check_status",
		Unit:        "1",
		Description: "Count of /readyz check outcomes by dependency and status.",
	}

	metricSelfProbeResult = libMetrics.Metric{
		Name:        "selfprobe_result",
		Unit:        "1",
		Description: "Last startup self-probe result per dependency (1=up, 0=down).",
	}
)

// Recorder is the call-site dependency for /readyz + selfprobe metric
// emission. It wraps a libMetrics.MetricsFactory with the canonical metric
// definitions and the bounded-cardinality validation pass.
//
// A nil-receiver Recorder is safe — every method is a no-op. This means
// callers (HealthChecker, RunSelfProbe) can be constructed without a
// recorder for unit tests that don't care about metric emission, and
// production code passes a real recorder built via NewRecorder.
type Recorder struct {
	factory *libMetrics.MetricsFactory
	logger  libLog.Logger
}

// NewRecorder returns a Recorder backed by the supplied MetricsFactory.
// A nil factory yields a no-op recorder so the call sites can keep emitting
// without nil-checks — the lib-commons builders already short-circuit on
// nil receivers.
//
// The logger is used only to surface metric-emission failures at Warn level
// — a missing emission is non-fatal, so we never propagate the error.
func NewRecorder(factory *libMetrics.MetricsFactory, logger libLog.Logger) *Recorder {
	return &Recorder{factory: factory, logger: logger}
}

// NewNopRecorder returns a Recorder backed by a no-op MetricsFactory.
// Suitable for unit tests that exercise the readyz/selfprobe code paths
// without asserting on metric output.
func NewNopRecorder() *Recorder {
	return &Recorder{factory: libMetrics.NewNopFactory()}
}

// EmitCheckDuration records the latency of a single /readyz dep check.
// MUST be called from every probe execution. Invalid (dep, status) tuples
// are silently dropped to preserve the bounded-cardinality contract.
func (r *Recorder) EmitCheckDuration(ctx context.Context, dep, status string, d time.Duration) {
	if r == nil || r.factory == nil {
		return
	}

	if !isValidDep(dep) || !isValidStatus(status) {
		return
	}

	hist, err := r.factory.Histogram(metricCheckDuration)
	if err != nil || hist == nil {
		r.logEmitFailure(ctx, metricCheckDuration.Name, err)

		return
	}

	if err := hist.WithLabels(map[string]string{
		"dep":    dep,
		"status": status,
	}).Record(ctx, d.Milliseconds()); err != nil {
		r.logEmitFailure(ctx, metricCheckDuration.Name, err)
	}
}

// EmitCheckStatus increments the outcome counter for a /readyz check.
// MUST be called from every probe execution. Invalid (dep, status) tuples
// are silently dropped to preserve the bounded-cardinality contract.
func (r *Recorder) EmitCheckStatus(ctx context.Context, dep, status string) {
	if r == nil || r.factory == nil {
		return
	}

	if !isValidDep(dep) || !isValidStatus(status) {
		return
	}

	counter, err := r.factory.Counter(metricCheckStatus)
	if err != nil || counter == nil {
		r.logEmitFailure(ctx, metricCheckStatus.Name, err)

		return
	}

	if err := counter.WithLabels(map[string]string{
		"dep":    dep,
		"status": status,
	}).AddOne(ctx); err != nil {
		r.logEmitFailure(ctx, metricCheckStatus.Name, err)
	}
}

// EmitSelfProbeResult sets the startup self-probe gauge for a dep.
// Called once per dep at the end of RunSelfProbe. Invalid dep values are
// silently dropped.
func (r *Recorder) EmitSelfProbeResult(ctx context.Context, dep string, up bool) {
	if r == nil || r.factory == nil {
		return
	}

	if !isValidDep(dep) {
		return
	}

	gauge, err := r.factory.Gauge(metricSelfProbeResult)
	if err != nil || gauge == nil {
		r.logEmitFailure(ctx, metricSelfProbeResult.Name, err)

		return
	}

	var v int64
	if up {
		v = 1
	}

	if err := gauge.WithLabels(map[string]string{"dep": dep}).Set(ctx, v); err != nil {
		r.logEmitFailure(ctx, metricSelfProbeResult.Name, err)
	}
}

// logEmitFailure is best-effort — a missing metric emission is not fatal,
// so we log at Warn level and move on. Nil-safe on the logger so callers
// with no logger configured silently swallow the error.
func (r *Recorder) logEmitFailure(ctx context.Context, metricName string, err error) {
	if r == nil || r.logger == nil || err == nil {
		return
	}

	r.logger.With(
		libLog.String("operation", "observability.readyz.emit"),
		libLog.String("metric.name", metricName),
		libLog.String("error.message", err.Error()),
	).Log(ctx, libLog.LevelWarn, "Failed to emit readyz metric")
}
