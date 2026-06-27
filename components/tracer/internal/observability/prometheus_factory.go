// Copyright (c) 2026 Lerian Studio.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package observability

import (
	"context"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/log"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/otlptranslator"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// readyzMeterName is the OTel instrumentation scope used for the readyz
// metrics. Keeping this isolated from the application's primary instrumentation
// scope ("tracer-api") makes operator queries and dashboards explicit about
// which subsystem produced the series — and it lets the bridged MeterProvider
// be shut down independently of the main lib-commons telemetry stack.
const readyzMeterName = "tracer-readyz"

// NewPrometheusBackedFactory builds a libMetrics.MetricsFactory whose meter
// is wired to an OpenTelemetry → Prometheus bridge. The bridge is registered
// on the supplied prometheus.Registerer (typically prometheus.DefaultRegisterer
// so the existing /metrics handler exposes the series) using:
//
//   - UnderscoreEscapingWithoutSuffixes translation strategy — preserves the
//     canonical metric names (no `_total`/`_milliseconds` appended).
//   - WithoutTargetInfo, WithoutScopeInfo — drops the auto-generated
//     `target_info` / `otel_scope_info` series we don't surface today.
//
// The returned shutdown function MUST be invoked at process shutdown so the
// bridged MeterProvider releases its background resources cleanly. Bootstrap
// composes this with the rest of the graceful-shutdown chain.
//
// A nil registerer falls back to prometheus.DefaultRegisterer — convenient
// for production where the existing /metrics handler scrapes the default
// registry, but tests should always pass a fresh per-test registry to avoid
// the "duplicate collector" panic that the package-global registry produces
// across parallel tests.
func NewPrometheusBackedFactory(
	registerer prometheus.Registerer,
	logger libLog.Logger,
) (factory *libMetrics.MetricsFactory, shutdown func() error, err error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	exporter, err := otelprom.New(
		otelprom.WithRegisterer(registerer),
		otelprom.WithTranslationStrategy(otlptranslator.UnderscoreEscapingWithoutSuffixes),
		otelprom.WithoutTargetInfo(),
		otelprom.WithoutScopeInfo(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("readyz prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter(readyzMeterName)

	factory, err = libMetrics.NewMetricsFactory(meter, logger)
	if err != nil {
		// MeterProvider.Shutdown returns nil when called on a freshly-built
		// provider with no exporters running, so the rollback is best-effort.
		_ = provider.Shutdown(context.Background())

		return nil, nil, fmt.Errorf("readyz metrics factory: %w", err)
	}

	return factory, func() error {
		return provider.Shutdown(context.Background())
	}, nil
}
