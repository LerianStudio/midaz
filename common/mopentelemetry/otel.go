package mopentelemetry

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"log"
	"os"
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
	exporter, err := otlploggrpc.New(ctx, otlploggrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")), otlploggrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// newMetricExporter creates a new metric exporter that writes to stdout.
func (tl *Telemetry) newMetricExporter(ctx context.Context) (*otlpmetricgrpc.Exporter, error) {
	exp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(tl.CollectorExporterEndpoint), otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return exp, nil
}

// newTracerExporter creates a new tracer exporter that writes to stdout.
func (tl *Telemetry) newTracerExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(tl.CollectorExporterEndpoint), otlptracegrpc.WithInsecure())
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

func (tl *Telemetry) ShutdownTelemetry() {
	tl.shutdown()
}

// InitializeTelemetry initializes the telemetry providers and sets them globally.
func (tl *Telemetry) InitializeTelemetry() *Telemetry {
	ctx := context.Background()

	r, err := tl.newResource()
	if err != nil {
		log.Fatalf("can't initialize resource: %v", err)
	}

	tExp, err := tl.newTracerExporter(ctx)
	if err != nil {
		log.Fatalf("can't initialize tracer exporter: %v", err)
	}

	mExp, err := tl.newMetricExporter(ctx)
	if err != nil {
		log.Fatalf("can't initialize metric exporter: %v", err)
	}

	lExp, err := tl.newLoggerExporter(ctx)
	if err != nil {
		log.Fatalf("can't initialize logger exporter: %v", err)
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
			log.Fatalf("can't shutdown tracer exporter: %v", err)
		}

		err = mExp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("can't shutdown metric exporter: %v", err)
		}

		err = lExp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("can't shutdown logger exporter: %v", err)
		}

		err = mp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("can't shutdown metric provider: %v", err)
		}

		err = tp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("can't shutdown tracer provider: %v", err)
		}

		err = lp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("can't shutdown logger provider: %v", err)
		}
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return &Telemetry{
		LibraryName:    tl.LibraryName,
		TracerProvider: tp,
		MetricProvider: mp,
		shutdown:       tl.shutdown,
	}
}

// SetSpanAttributesFromStruct converts a struct to a JSON string and sets it as an attribute on the span.
func SetSpanAttributesFromStruct(span *trace.Span, key string, valueStruct any) error {
	vStr, err := common.StructToJSONString(valueStruct)
	if err != nil {
		return err
	}

	(*span).SetAttributes(attribute.KeyValue{
		Key:   attribute.Key(key),
		Value: attribute.StringValue(vStr),
	})

	return nil
}

func HandleSpanError(span *trace.Span, message string, err error) {
	(*span).SetStatus(codes.Error, message+": "+err.Error())
	(*span).RecordError(err)
}
