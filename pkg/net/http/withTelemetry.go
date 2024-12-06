package http

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
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

type TelemetryMiddleware struct {
	*mopentelemetry.Telemetry
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
	err := c.Next()

	go func() {
		trace.SpanFromContext(c.UserContext()).End()
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

		resp, err := handler(ctx, req)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "gRPC request failed", err)
		}

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
	cpuGauge, err := otel.Meter(tm.ServiceName).Int64Gauge("system.cpu.usage", metric.WithUnit("percentage"))
	if err != nil {
		return err
	}

	memGauge, err := otel.Meter(tm.ServiceName).Int64Gauge("system.mem.usage", metric.WithUnit("percentage"))
	if err != nil {
		return err
	}

	cmdExec := pkg.Syscmd{}
	cpuUsage := pkg.GetCPUUsage(ctx, &cmdExec)
	memUsage := pkg.GetMemUsage(ctx, &cmdExec)

	cpuGauge.Record(ctx, cpuUsage)
	memGauge.Record(ctx, memUsage)

	return nil
}
