package mopentelemetry

import (
	"github.com/LerianStudio/midaz/common"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"time"
)

// newMetricExporter creates a new metric exporter that writes to stdout.
func newMetricExporter() (metric.Exporter, error) {
	exp, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	return exp, nil
}

// newTracerExporter creates a new tracer exporter that writes to stdout.
func newTracerExporter() (*stdouttrace.Exporter, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		// TODO: Add a logger here
		return nil, err
	}
	return exporter, nil
}

// newResource creates a new resource with default attributes.
func newResource() (*resource.Resource, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(common.GetenvOrDefault("SERVICE_NAME", "NO-SERVICE-NAME")),
			semconv.ServiceVersion(common.GetenvOrDefault("VERSION", "NO-VERSION")),
			semconv.DeploymentEnvironment(common.GetenvOrDefault("ENV_NAME", "local")),
		),
	)
	if err != nil {
		// TODO: Add a logger here
		return nil, err
	}

	return r, nil
}

// newMeterProvider creates a new meter provider with stdout exporter and default resource.
func newMeterProvider(res *resource.Resource, exp metric.Exporter) *metric.MeterProvider {
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exp,
			// TODO: (REMOVE THIS) Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
	)

	return mp
}

// newTracerProvider creates a new tracer provider with stdout exporter and default resource.
func newTracerProvider(rsc *resource.Resource, exp *stdouttrace.Exporter) *sdktrace.TracerProvider {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(rsc),
	)

	return tp
}

// InitializeTelemetry initializes the telemetry providers and sets them globally.
func InitializeTelemetry() error {
	r, err := newResource()
	if err != nil {
		// TODO: Add a logger here
		return err
	}

	tExp, err := newTracerExporter()
	if err != nil {
		// TODO: Add a logger here
		return err
	}

	mExp, err := newMetricExporter()
	if err != nil {
		// TODO: Add a logger here
		return err
	}

	mcp := newMeterProvider(r, mExp)
	otel.SetMeterProvider(mcp)

	tcp := newTracerProvider(r, tExp)
	otel.SetTracerProvider(tcp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return nil
}
