package mruntime

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ErrPanic is the sentinel error for recovered panics recorded to spans.
var ErrPanic = errors.New("panic")

// PanicSpanEventName is the event name used when recording panic events on spans.
const PanicSpanEventName = "panic.recovered"

// RecordPanicToSpan records a recovered panic as an error event on the current span.
// This enriches distributed traces with panic information for debugging.
//
// The function:
//   - Adds a "panic.recovered" event with panic value, stack trace, and goroutine name
//   - Records the panic as an error using span.RecordError
//   - Sets the span status to Error with a descriptive message
//
// Parameters:
//   - ctx: Context containing the active span
//   - panicValue: The value passed to panic()
//   - stack: The stack trace captured via debug.Stack()
//   - goroutineName: The name of the goroutine where the panic occurred
//
// If there is no active span in the context, this function is a no-op.
func RecordPanicToSpan(ctx context.Context, panicValue any, stack []byte, goroutineName string) {
	recordPanicToSpanInternal(ctx, panicValue, stack, "", goroutineName)
}

// RecordPanicToSpanWithComponent is like RecordPanicToSpan but also includes the component name.
// This is useful for HTTP/gRPC handlers where both component and handler name are relevant.
//
// Parameters:
//   - ctx: Context containing the active span
//   - panicValue: The value passed to panic()
//   - stack: The stack trace captured via debug.Stack()
//   - component: The service component (e.g., "transaction", "onboarding")
//   - goroutineName: The name of the handler or goroutine
func RecordPanicToSpanWithComponent(ctx context.Context, panicValue any, stack []byte, component, goroutineName string) {
	recordPanicToSpanInternal(ctx, panicValue, stack, component, goroutineName)
}

// recordPanicToSpanInternal is the shared implementation for recording panic events.
func recordPanicToSpanInternal(ctx context.Context, panicValue any, stack []byte, component, goroutineName string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	panicStr := fmt.Sprintf("%v", panicValue)

	// Build attributes list
	attrs := []attribute.KeyValue{
		attribute.String("panic.value", panicStr),
		attribute.String("panic.stack", string(stack)),
		attribute.String("panic.goroutine_name", goroutineName),
	}

	// Add component if provided
	if component != "" {
		attrs = append(attrs, attribute.String("panic.component", component))
	}

	// Add detailed event with all panic information
	span.AddEvent(PanicSpanEventName, trace.WithAttributes(attrs...))

	// Record as error for error-tracking integrations
	span.RecordError(fmt.Errorf("%w: %v", ErrPanic, panicValue))

	// Set span status to Error
	statusMsg := "panic recovered in " + goroutineName
	if component != "" {
		statusMsg = fmt.Sprintf("panic recovered in %s/%s", component, goroutineName)
	}

	span.SetStatus(codes.Error, statusMsg)
}
