package http

import (
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// WithTracing is a middleware that adds tracing to the context.
func WithTracing() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tracer := otel.Tracer("midaz")
		ctx, _ := tracer.Start(c.Context(), c.Method()+" "+c.Route().Path)

		c.Locals("tracer", tracer)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// EndTracingSpans is a middleware that ends the tracing spans.
func EndTracingSpans() fiber.Handler {
	return func(c *fiber.Ctx) error {
		trace.SpanFromContext(c.Context()).End()

		return c.Next()
	}
}
