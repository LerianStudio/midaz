// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// datasourceHistogramBucketsMs is the explicit bucket set for
// datasource_check_duration_ms. Customer datasource pings (PostgreSQL schema
// fetch, MongoDB schema fetch) typically run in 5–500ms; the buckets span
// that range and extend to 5s so a slow link is visible at the top end
// rather than dropped into +Inf.
//
// Boundaries deliberately match readyz's bucket set so dashboards can use a
// single bucket scheme across both metric families.
//
//nolint:gochecknoglobals // immutable bucket set used for instrument registration
var datasourceHistogramBucketsMs = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}

// DatasourceMetrics holds the two OTel instruments emitted by the periodic
// HealthChecker loop for customer datasources (PostgreSQL, MongoDB).
//
// These are intentionally a separate metric family from the readyz metrics:
// per Decision D6 in dev-readyz Gate 1, customer datasources are NOT
// included in the /readyz aggregation (a slow customer DB must not 503 the
// whole service). The metrics live here so operators can still alert on
// per-customer-datasource health without polluting /readyz dashboards.
//
// Instrument choices match the readyz pattern (Int64Gauge for last-value
// state, Float64Histogram in ms for latency):
//   - datasource_healthy (Int64Gauge): 1=up, 0=down per datasource_id
//   - datasource_check_duration_ms (Float64Histogram): per-ping latency
//
// All emit methods are safe to call on a nil receiver and on a Metrics
// constructed with a nil meter (noop fallback).
type DatasourceMetrics struct {
	healthy  metric.Int64Gauge
	duration metric.Float64Histogram
}

// NewDatasourceMetrics builds the two instruments on the provided meter. If
// meter is nil, NewDatasourceMetrics falls back to a noop meter so the
// returned struct is always usable. Returns an error only if a real meter
// rejects an instrument registration.
func NewDatasourceMetrics(meter metric.Meter) (*DatasourceMetrics, error) {
	if meter == nil {
		meter = noop.NewMeterProvider().Meter("datasource")
	}

	healthy, err := meter.Int64Gauge(
		"datasource_healthy",
		metric.WithDescription("Last health-check result per customer datasource (1=up, 0=down)"),
		metric.WithUnit("{result}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create datasource_healthy gauge: %w", err)
	}

	duration, err := meter.Float64Histogram(
		"datasource_check_duration_ms",
		metric.WithUnit("ms"),
		metric.WithDescription("Duration of customer datasource health pings in milliseconds"),
		metric.WithExplicitBucketBoundaries(datasourceHistogramBucketsMs...),
	)
	if err != nil {
		return nil, fmt.Errorf("create datasource_check_duration_ms histogram: %w", err)
	}

	return &DatasourceMetrics{
		healthy:  healthy,
		duration: duration,
	}, nil
}

// EmitDatasourceHealthy records the latest health result for a datasource:
// 1 when healthy=true, 0 when healthy=false. The gauge replaces the previous
// value, mirroring selfprobe_result semantics. Safe to call on a nil
// DatasourceMetrics (no-op).
func (m *DatasourceMetrics) EmitDatasourceHealthy(ctx context.Context, datasourceID string, healthy bool) {
	if m == nil {
		return
	}

	value := int64(0)
	if healthy {
		value = 1
	}

	m.healthy.Record(ctx, value, metric.WithAttributes(
		attribute.String("datasource_id", datasourceID),
	))
}

// EmitDatasourceCheckDuration records the latency of a single datasource
// health ping in milliseconds. Safe to call on a nil DatasourceMetrics
// (no-op).
func (m *DatasourceMetrics) EmitDatasourceCheckDuration(ctx context.Context, datasourceID string, d time.Duration) {
	if m == nil {
		return
	}

	m.duration.Record(ctx, msFloat(d), metric.WithAttributes(
		attribute.String("datasource_id", datasourceID),
	))
}

// msFloat converts a time.Duration to a float64 number of milliseconds with
// sub-millisecond resolution. Mirrors the readyz package's helper of the
// same name so cache hits don't bottom out the histogram at zero.
func msFloat(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
