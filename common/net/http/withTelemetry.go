package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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
		ctx, _ := tracer.Start(c.Context(), c.Method()+" "+c.Path())

		ctx = mopentelemetry.ContextWithTracer(ctx, tracer)

		c.SetUserContext(ctx)

		err := tm.collectMetrics(c)
		if err != nil {
			return WithError(c, err)
		}

		return c.Next()
	}
}

// EndTracingSpans is a middleware that ends the tracing spans.
func (tm *TelemetryMiddleware) EndTracingSpans() fiber.Handler {
	return func(c *fiber.Ctx) error {
		trace.SpanFromContext(c.Context()).End()

		return c.Next()
	}
}

func (tm *TelemetryMiddleware) collectMetrics(c *fiber.Ctx) error {
	cpuGauge, err := otel.Meter(tm.ServiceName).Int64Gauge("system.cpu.usage", metric.WithUnit("percentage"))
	if err != nil {
		return err
	}

	memGauge, err := otel.Meter(tm.ServiceName).Int64Gauge("system.mem.usage", metric.WithUnit("percentage"))
	if err != nil {
		return err
	}

	cpuUsage := common.GetCPUUsage()
	memUsage := common.GetMemUsage()

	cpuGauge.Record(c.Context(), cpuUsage)
	memGauge.Record(c.Context(), memUsage)

	return nil
}
