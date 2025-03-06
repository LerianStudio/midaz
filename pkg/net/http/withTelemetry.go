package http

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/pkg"
	cn "github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/gofiber/fiber/v2"
)

// Define a custom context key for fiber.Ctx
type fiberCtxKey struct{}

type TelemetryMiddleware struct {
	Telemetry *mopentelemetry.Telemetry
}

// NewTelemetryMiddleware creates a new instance of TelemetryMiddleware.
func NewTelemetryMiddleware(tl *mopentelemetry.Telemetry) *TelemetryMiddleware {
	return &TelemetryMiddleware{tl}
}

// WithTelemetry is a middleware that adds tracing to the context.
func (tm *TelemetryMiddleware) WithTelemetry(tl *mopentelemetry.Telemetry) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tracer := otel.Tracer(tl.LibraryName)
		ctx := pkg.ContextWithTracer(c.UserContext(), tracer)

		if strings.Contains(c.Path(), "swagger") && c.Path() != "/swagger/index.html" {
			return c.Next()
		}

		ctx, span := tracer.Start(ctx, c.Method()+" "+pkg.ReplaceUUIDWithPlaceholder(c.Path()))
		defer span.End()

		c.SetUserContext(ctx)

		// Add fiber.Ctx to context so it can be used in collectMetrics
		ctx = context.WithValue(ctx, fiberCtxKey{}, c)

		err := tm.collectMetrics(ctx)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to collect metrics", err)

			return WithError(c, err)
		}

		return c.Next()
	}
}

// EndTracingSpans is a middleware that ends the tracing spans.
func (tm *TelemetryMiddleware) EndTracingSpans(c *fiber.Ctx) error {
	ctx := c.UserContext()
	if ctx == nil {
		return nil
	}

	err := c.Next()

	go func() {
		trace.SpanFromContext(ctx).End()
	}()

	return err
}

// WithTelemetryInterceptor is a gRPC interceptor that adds tracing to the context.
func (tm *TelemetryMiddleware) WithTelemetryInterceptor(tl *mopentelemetry.Telemetry) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		tracer := otel.Tracer(tl.LibraryName)
		ctx, span := tracer.Start(mopentelemetry.ExtractContext(ctx), info.FullMethod)

		ctx = pkg.ContextWithTracer(ctx, tracer)

		// Store gRPC method info in context for metrics
		ctx = context.WithValue(ctx, "grpc_method", info.FullMethod)

		err := tm.collectMetrics(ctx)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to collect metrics", err)

			e := pkg.ValidateBusinessError(cn.ErrInternalServer, "Failed to collect metrics")

			jsonStringError, err := pkg.StructToJSONString(e)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to marshal error response", err)

				return nil, status.Error(codes.Internal, "Failed to marshal error response")
			}

			return nil, status.Error(codes.FailedPrecondition, jsonStringError)
		}

		// Track the start time for duration measurement
		startTime := time.Now()

		resp, err := handler(ctx, req)

		// Calculate request duration
		duration := time.Since(startTime).Milliseconds()

		// Record gRPC metrics
		meter := otel.Meter(tm.ServiceName)

		// gRPC request counter
		grpcCounter, _ := meter.Int64Counter(
			"grpc.server.request_count",
			metric.WithDescription("Number of gRPC requests"),
			metric.WithUnit("{request}"),
		)

		// gRPC duration histogram
		grpcDuration, _ := meter.Int64Histogram(
			"grpc.server.duration",
			metric.WithDescription("Duration of gRPC requests"),
			metric.WithUnit("ms"),
		)

		// Determine status code from error
		statusCode := "OK"
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "gRPC request failed", err)
			st, _ := status.FromError(err)
			statusCode = st.Code().String()
		}

		// Add attributes
		attributes := []attribute.KeyValue{
			attribute.String("service.name", tm.ServiceName),
			attribute.String("grpc.method", info.FullMethod),
			attribute.String("status_code", statusCode),
		}

		// Record metrics
		grpcCounter.Add(ctx, 1, metric.WithAttributes(attributes...))
		grpcDuration.Record(ctx, duration, metric.WithAttributes(attributes...))

		return resp, err
	}
}

// EndTracingSpansInterceptor is a gRPC interceptor that ends the tracing spans.
func (tm *TelemetryMiddleware) EndTracingSpansInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)

		go func() {
			trace.SpanFromContext(ctx).End()
		}()

		return resp, err
	}
}

func (tm *TelemetryMiddleware) collectMetrics(ctx context.Context) error {
	meter := otel.Meter(tm.ServiceName)

	// CPU usage
	cpuGauge, err := meter.Int64Gauge(
		"system.cpu.usage",
		metric.WithDescription("CPU usage in percentage"),
		metric.WithUnit("percentage"),
	)
	if err != nil {
		return err
	}

	// Memory usage
	memGauge, err := meter.Int64Gauge(
		"system.mem.usage",
		metric.WithDescription("Memory usage in percentage"),
		metric.WithUnit("percentage"),
	)
	if err != nil {
		return err
	}

	// Get fiber context if available
	if fiberCtx, ok := ctx.Value(fiberCtxKey{}).(*fiber.Ctx); ok {
		// Track HTTP request metrics
		httpCounter, err := meter.Int64Counter(
			"http.server.request_count",
			metric.WithDescription("Number of HTTP requests"),
			metric.WithUnit("{request}"),
		)
		if err != nil {
			return err
		}

		// Record HTTP request
		attributes := []attribute.KeyValue{
			attribute.String("service.name", tm.ServiceName),
			attribute.String("path", fiberCtx.Path()),
			attribute.String("method", fiberCtx.Method()),
			attribute.String("status_code", strconv.Itoa(fiberCtx.Response().StatusCode())),
		}
		httpCounter.Add(ctx, 1, metric.WithAttributes(attributes...))
	}

	// Collect system metrics in background
	go pkg.GetCPUUsage(ctx, cpuGauge)
	go pkg.GetMemUsage(ctx, memGauge)

	return nil
}
