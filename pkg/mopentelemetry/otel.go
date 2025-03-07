package mopentelemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mlog"
)

type Telemetry struct {
	LibraryName               string
	ServiceName               string
	ServiceVersion            string
	DeploymentEnv             string
	CollectorExporterEndpoint string
	TracerProvider            *sdktrace.TracerProvider
	MetricProvider            *sdkmetric.MeterProvider
	LoggerProvider            *sdklog.LoggerProvider
	shutdown                  func()
	EnableTelemetry           bool
}

// NewResource creates a new resource with default attributes.
func (tl *Telemetry) newResource() (*sdkresource.Resource, error) {
	r, err := sdkresource.Merge(
		sdkresource.Default(),
		sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(tl.ServiceName),
			semconv.ServiceVersion(tl.ServiceVersion),
			semconv.DeploymentEnvironment(tl.DeploymentEnv)),
	)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// NewLoggerExporter creates a new logger exporter that writes to stdout.
func (tl *Telemetry) newLoggerExporter(ctx context.Context) (*otlploggrpc.Exporter, error) {
	// Check if OTEL_EXPORTER_OTLP_ENDPOINT is set, otherwise use the configured endpoint
	endpoint := pkg.GetenvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", tl.CollectorExporterEndpoint)

	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// newMetricExporter creates a new metric exporter that writes to stdout.
func (tl *Telemetry) newMetricExporter(ctx context.Context) (*otlpmetricgrpc.Exporter, error) {
	// Check if OTEL_EXPORTER_OTLP_ENDPOINT is set, otherwise use the configured endpoint
	endpoint := pkg.GetenvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", tl.CollectorExporterEndpoint)

	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return exp, nil
}

// newTracerExporter creates a new tracer exporter that writes to stdout.
func (tl *Telemetry) newTracerExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	// Check if OTEL_EXPORTER_OTLP_ENDPOINT is set, otherwise use the configured endpoint
	endpoint := pkg.GetenvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", tl.CollectorExporterEndpoint)

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// NewLoggerProvider creates a new logger provider with stdout exporter and default resource.
func (tl *Telemetry) newLoggerProvider(rsc *sdkresource.Resource, exp *otlploggrpc.Exporter) *sdklog.LoggerProvider {
	bp := sdklog.NewBatchProcessor(exp)
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(rsc), sdklog.WithProcessor(bp))

	return lp
}

// newMeterProvider creates a new meter provider with stdout exporter and default resource.
func (tl *Telemetry) newMeterProvider(res *sdkresource.Resource, exp *otlpmetricgrpc.Exporter) *sdkmetric.MeterProvider {
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
	)

	return mp
}

// newTracerProvider creates a new tracer provider with stdout exporter and default resource.
func (tl *Telemetry) newTracerProvider(rsc *sdkresource.Resource, exp *otlptrace.Exporter) *sdktrace.TracerProvider {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(rsc),
	)

	return tp
}

// ShutdownTelemetry shuts down the telemetry providers and exporters.
func (tl *Telemetry) ShutdownTelemetry() {
	tl.shutdown()
}

// InitializeTelemetry initializes the telemetry providers and sets them globally. (Logger is being passed as a parameter because it not exists in the global context at this point to be injected)
func (tl *Telemetry) InitializeTelemetry(logger mlog.Logger) *Telemetry {
	ctx := context.Background()

	if !tl.EnableTelemetry {
		logger.Warn("Telemetry turned off ⚠️ ")

		return nil
	}

	logger.Infof("Initializing telemetry...")

	r, err := tl.newResource()
	if err != nil {
		logger.Fatalf("can't initialize resource: %v", err)
	}

	tExp, err := tl.newTracerExporter(ctx)
	if err != nil {
		logger.Fatalf("can't initialize tracer exporter: %v", err)
	}

	mExp, err := tl.newMetricExporter(ctx)
	if err != nil {
		logger.Fatalf("can't initialize metric exporter: %v", err)
	}

	lExp, err := tl.newLoggerExporter(ctx)
	if err != nil {
		logger.Fatalf("can't initialize logger exporter: %v", err)
	}

	mp := tl.newMeterProvider(r, mExp)
	otel.SetMeterProvider(mp)
	tl.MetricProvider = mp

	tp := tl.newTracerProvider(r, tExp)
	otel.SetTracerProvider(tp)
	tl.TracerProvider = tp

	lp := tl.newLoggerProvider(r, lExp)
	global.SetLoggerProvider(lp)

	tl.shutdown = func() {
		err := tExp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown tracer exporter: %v", err)
		}

		err = mExp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown metric exporter: %v", err)
		}

		err = lExp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown logger exporter: %v", err)
		}

		err = mp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown metric provider: %v", err)
		}

		err = tp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown tracer provider: %v", err)
		}

		err = lp.Shutdown(ctx)
		if err != nil {
			logger.Fatalf("can't shutdown logger provider: %v", err)
		}
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Start background system metrics collection
	go tl.startBackgroundMetricsCollection(ctx, logger)

	logger.Infof("Telemetry initialized ✅ ")

	return &Telemetry{
		LibraryName:    tl.LibraryName,
		TracerProvider: tp,
		MetricProvider: mp,
		shutdown:       tl.shutdown,
	}
}

// startBackgroundMetricsCollection collects system metrics on a regular interval
func (tl *Telemetry) startBackgroundMetricsCollection(ctx context.Context, logger mlog.Logger) {
	meter := otel.Meter(tl.ServiceName)

	// CPU usage metric
	cpuGauge, err := meter.Int64Gauge(
		"system.cpu.usage",
		metric.WithDescription("CPU usage in percentage"),
		metric.WithUnit("percentage"),
	)
	if err != nil {
		logger.Errorf("Failed to create CPU gauge: %v", err)
		return
	}

	// Memory usage metric
	memGauge, err := meter.Int64Gauge(
		"system.mem.usage",
		metric.WithDescription("Memory usage in percentage"),
		metric.WithUnit("percentage"),
	)
	if err != nil {
		logger.Errorf("Failed to create memory gauge: %v", err)
		return
	}

	// Service info metric (for service discovery)
	serviceInfo, err := meter.Int64UpDownCounter(
		"service.info",
		metric.WithDescription("Information about the service"),
		metric.WithUnit("{info}"),
	)
	if err != nil {
		logger.Errorf("Failed to create service info metric: %v", err)
		return
	}

	// Record service information once
	serviceAttributes := []attribute.KeyValue{
		attribute.String("service.name", tl.ServiceName),
		attribute.String("service.version", tl.ServiceVersion),
		attribute.String("deployment.environment", tl.DeploymentEnv),
	}
	serviceInfo.Add(ctx, 1, metric.WithAttributes(serviceAttributes...))

	// Run continuous collection at regular intervals
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get CPU and memory metrics
			pkg.GetCPUUsage(ctx, cpuGauge)
			pkg.GetMemUsage(ctx, memGauge)
		}
	}
}

// SetSpanAttributesFromStruct converts a struct to a JSON string and sets it as an attribute on the span.
func SetSpanAttributesFromStruct(span *trace.Span, key string, valueStruct any) error {
	vStr, err := pkg.StructToJSONString(valueStruct)
	if err != nil {
		return err
	}

	(*span).SetAttributes(attribute.KeyValue{
		Key:   attribute.Key(key),
		Value: attribute.StringValue(vStr),
	})

	return nil
}

// HandleSpanError sets the status of the span to error and records the error.
func HandleSpanError(span *trace.Span, message string, err error) {
	(*span).SetStatus(codes.Error, message+": "+err.Error())
	(*span).RecordError(err)
}

// InjectContext injects the context with the OpenTelemetry headers (in lowercase) and returns the new context.
func InjectContext(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	if md == nil {
		md = metadata.New(nil)
	}

	// Returns the canonical format of the MIME header key s.
	// The canonicalization converts the first letter and any letter
	// following a hyphen to upper case; the rest are converted to lowercase.
	// For example, the canonical key for "accept-encoding" is "Accept-Encoding".
	// MIME header keys are assumed to be ASCII only.
	// If s contains a space or invalid header field bytes, it is
	// returned without modifications.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(md))

	if traceparentValue, exists := md["Traceparent"]; exists {
		md[constant.MDTraceparent] = traceparentValue
		delete(md, "Traceparent")
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// ExtractContext extracts the OpenTelemetry headers (in lowercase) from the context and returns the new context.
func ExtractContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)

	if traceparentValue, exists := md["traceparent"]; exists {
		md["Traceparent"] = traceparentValue
		delete(md, "traceparent")
	}

	if ok {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(md))
	}

	return ctx
}
