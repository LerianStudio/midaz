// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// readyzHistogramBucketsMs is the explicit bucket set used for
// readyz_check_duration_ms. It spans cache-fast probes (1ms) through
// timeout-slow probes (5000ms) so dashboards can render meaningful p50/p95/p99
// without being dominated by the bottom or top bucket.
//
// The boundaries match the canonical contract in
// dev-readyz/SKILL.md > Gate 5 > "Histogram buckets (ms)".
//
//nolint:gochecknoglobals // immutable bucket set used for instrument registration
var readyzHistogramBucketsMs = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}

// Metrics holds the three canonical /readyz OTel instruments.
//
// Instrument choices:
//   - readyz_check_duration_ms (Float64Histogram, unit=ms): per-dep probe
//     latency. Float64 + ms units lets Grafana render values directly without
//     a unit conversion in the query.
//   - readyz_check_status (Int64Counter): per-dep outcome counter, rate-able
//     via PromQL/MQL. Status label is a closed vocabulary
//     (see types.go > Status), so cardinality is bounded.
//   - selfprobe_result (Int64Gauge): per-dep startup self-probe last value
//     (1=up, 0=down). Synchronous Gauge is the cleanest fit: callers Record
//     the absolute current state and OTel exports the latest sample. We
//     deliberately avoid:
//   - Int64UpDownCounter (would require caller-side delta math, fragile)
//   - Int64ObservableGauge (would require shared state map + callback,
//     more moving parts than we need given Gate 7's call cadence is
//     "once per dep at the end of RunSelfProbe")
//
// All emit methods are safe to call on a nil receiver and on a Metrics
// constructed from NewMetrics(nil) (which falls back to a noop meter). This
// lets handlers and probes call the helpers unconditionally during early
// bootstrap, before the meter is plumbed through.
type Metrics struct {
	checkDuration   metric.Float64Histogram
	checkStatus     metric.Int64Counter
	selfProbeResult metric.Int64Gauge
}

// NewMetrics builds the three instruments on the provided meter. If meter is
// nil, NewMetrics falls back to a noop meter so the returned Metrics can be
// used safely in tests and during partial bootstrap. NewMetrics returns an
// error only if a real meter rejects an instrument registration; the noop
// meter never errors.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	if meter == nil {
		meter = noop.NewMeterProvider().Meter("readyz")
	}

	checkDuration, err := meter.Float64Histogram(
		"readyz_check_duration_ms",
		metric.WithUnit("ms"),
		metric.WithDescription("Duration of /readyz dependency checks in milliseconds"),
		metric.WithExplicitBucketBoundaries(readyzHistogramBucketsMs...),
	)
	if err != nil {
		return nil, fmt.Errorf("create readyz_check_duration_ms histogram: %w", err)
	}

	checkStatus, err := meter.Int64Counter(
		"readyz_check_status",
		metric.WithDescription("Count of /readyz check outcomes per dep and status"),
		metric.WithUnit("{check}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create readyz_check_status counter: %w", err)
	}

	selfProbeResult, err := meter.Int64Gauge(
		"selfprobe_result",
		metric.WithDescription("Last startup self-probe result per dependency (1=up, 0=down)"),
		metric.WithUnit("{result}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create selfprobe_result gauge: %w", err)
	}

	return &Metrics{
		checkDuration:   checkDuration,
		checkStatus:     checkStatus,
		selfProbeResult: selfProbeResult,
	}, nil
}

// EmitCheckDuration records a single per-dep readyz probe duration in
// milliseconds, tagged with the dep name and the resulting Status. Safe to
// call on a nil Metrics (no-op) — this lets handlers be wired before the
// meter is available without nil-check boilerplate.
func (m *Metrics) EmitCheckDuration(ctx context.Context, dep string, status Status, d time.Duration) {
	if m == nil {
		return
	}

	m.checkDuration.Record(ctx, msFloat(d), metric.WithAttributes(
		attribute.String("dep", dep),
		attribute.String("status", string(status)),
	))
}

// EmitCheckStatus increments the per-dep readyz outcome counter. Each call
// records exactly one observation tagged with dep and status. Safe to call
// on a nil Metrics (no-op).
func (m *Metrics) EmitCheckStatus(ctx context.Context, dep string, status Status) {
	if m == nil {
		return
	}

	m.checkStatus.Add(ctx, 1, metric.WithAttributes(
		attribute.String("dep", dep),
		attribute.String("status", string(status)),
	))
}

// EmitSelfProbeResult records the latest startup self-probe outcome for a
// dependency: 1 when up=true, 0 when up=false. Subsequent calls overwrite
// the previous value (gauge semantics). Safe to call on a nil Metrics
// (no-op).
//
// Gate 7's RunSelfProbe is the production caller; this method is provided
// here so the instrument is registered with the meter at bootstrap time.
func (m *Metrics) EmitSelfProbeResult(ctx context.Context, dep string, up bool) {
	if m == nil {
		return
	}

	value := int64(0)
	if up {
		value = 1
	}

	m.selfProbeResult.Record(ctx, value, metric.WithAttributes(
		attribute.String("dep", dep),
	))
}

// msFloat converts a time.Duration to a float64 number of milliseconds with
// fractional resolution. We deliberately avoid d.Milliseconds() because it
// truncates sub-millisecond probes to zero, which would silently bottom out
// the histogram for cache hits.
func msFloat(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
