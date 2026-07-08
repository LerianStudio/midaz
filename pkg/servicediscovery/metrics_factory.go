// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	"go.opentelemetry.io/otel/attribute"
)

// Service Discovery metric descriptors. They are the single registration point
// for the SD telemetry surface and back the MetricsFactory recorder below.
var (
	sdRegisterTotal = metrics.Metric{
		Name:        "sd_register_total",
		Unit:        "1",
		Description: "Total Service Discovery registrations initiated.",
	}

	sdDeregisterTotal = metrics.Metric{
		Name:        "sd_deregister_total",
		Unit:        "1",
		Description: "Total Service Discovery deregistrations by result.",
	}

	sdResolveTotal = metrics.Metric{
		Name:        "sd_resolve_total",
		Unit:        "1",
		Description: "Total Service Discovery resolutions by service and result.",
	}

	sdResolveDurationMs = metrics.Metric{
		Name:        "sd_resolve_duration_milliseconds",
		Unit:        "ms",
		Description: "Service Discovery resolve duration in milliseconds.",
	}
)

// metricsFactoryRecorder is the OTel-backed MetricsRecorder. It records SD metrics
// through a lib-observability MetricsFactory. Recording is best-effort: builder or
// record errors are logged at Warn and never propagated, so telemetry cannot affect
// the register/deregister/resolve call sites.
type metricsFactoryRecorder struct {
	factory *metrics.MetricsFactory
	logger  libLog.Logger
}

var _ MetricsRecorder = (*metricsFactoryRecorder)(nil)

// NewMetricsFactoryRecorder builds a MetricsRecorder backed by factory. A nil
// factory (telemetry or SD disabled) yields a NopMetricsRecorder so call sites
// pay zero overhead and never need a nil check.
func NewMetricsFactoryRecorder(factory *metrics.MetricsFactory, logger libLog.Logger) MetricsRecorder {
	if factory == nil {
		return NopMetricsRecorder{}
	}

	return &metricsFactoryRecorder{factory: factory, logger: logger}
}

func (r *metricsFactoryRecorder) RegisterInitiated(ctx context.Context) {
	r.addOne(ctx, sdRegisterTotal)
}

func (r *metricsFactoryRecorder) DeregisterResult(ctx context.Context, result string) {
	r.addOne(ctx, sdDeregisterTotal, attribute.String("result", result))
}

func (r *metricsFactoryRecorder) ResolveResult(ctx context.Context, service, result string, durationMs int64) {
	r.addOne(ctx, sdResolveTotal,
		attribute.String("service", service),
		attribute.String("result", result))

	r.record(ctx, sdResolveDurationMs, durationMs, attribute.String("service", service))
}

// addOne increments the named counter by one with the given attributes. Builder
// and add errors are logged at Warn and swallowed; recording never affects the
// caller.
func (r *metricsFactoryRecorder) addOne(ctx context.Context, m metrics.Metric, attrs ...attribute.KeyValue) {
	counter, err := r.factory.Counter(m)
	if err != nil {
		r.warn(ctx, "failed to build service discovery counter", err)

		return
	}

	if err := counter.WithAttributes(attrs...).AddOne(ctx); err != nil {
		r.warn(ctx, "failed to record service discovery counter", err)
	}
}

// record observes value on the named histogram with the given attributes. Builder
// and record errors are logged at Warn and swallowed; recording never affects the
// caller.
func (r *metricsFactoryRecorder) record(ctx context.Context, m metrics.Metric, value int64, attrs ...attribute.KeyValue) {
	histogram, err := r.factory.Histogram(m)
	if err != nil {
		r.warn(ctx, "failed to build service discovery histogram", err)

		return
	}

	if err := histogram.WithAttributes(attrs...).Record(ctx, value); err != nil {
		r.warn(ctx, "failed to record service discovery histogram", err)
	}
}

// warn logs a recording failure at Warn when a logger is present. It never carries
// metric values or attributes, only the error, so no PII or financial data leaks.
func (r *metricsFactoryRecorder) warn(ctx context.Context, msg string, err error) {
	if r.logger == nil {
		return
	}

	r.logger.Log(ctx, libLog.LevelWarn, msg, libLog.Err(err))
}
