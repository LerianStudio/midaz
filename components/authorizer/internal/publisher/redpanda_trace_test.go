// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestBuildRecordHeaders_InjectsTraceparent verifies that when ctx carries an
// active OpenTelemetry span and the global propagator includes TraceContext,
// buildRecordHeaders emits a "traceparent" header on the produced record.
// This is the regression guard for B6 (FINAL_REVIEW.md) — without header
// injection, 2PC commit intents published to Redpanda arrive at the recovery
// worker with no parent trace, breaking end-to-end distributed tracing
// across Authorize → Redpanda → Recovery.
func TestBuildRecordHeaders_InjectsTraceparent(t *testing.T) {
	// Configure a W3C TraceContext propagator globally so
	// libOpentelemetry.InjectTraceHeadersIntoQueue has something to write.
	originalPropagator := otel.GetTextMapPropagator()

	t.Cleanup(func() { otel.SetTextMapPropagator(originalPropagator) })

	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Swap in an SDK tracer provider so spans we start have a valid, sampled
	// SpanContext (the default no-op provider produces invalid contexts that
	// the propagator refuses to inject).
	originalProvider := otel.GetTracerProvider()

	t.Cleanup(func() { otel.SetTracerProvider(originalProvider) })

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	otel.SetTracerProvider(tp)

	// Start a span and use its context to build headers.
	ctx, span := tp.Tracer("authorizer.publisher.test").Start(context.Background(), "test-span")
	defer span.End()

	msg := Message{
		Topic:        "ledger.balance.operations",
		PartitionKey: "acct-123",
		Payload:      []byte(`{"commit":"intent"}`),
		Headers:      map[string]string{"x-correlation-id": "corr-abc"},
		ContentType:  "application/json",
	}

	headers := buildRecordHeaders(ctx, msg)

	// Collect headers into a case-insensitive map for assertion. Kafka headers
	// preserve case, but the W3C propagator writes "Traceparent" (title-case)
	// while spec consumers match case-insensitively. We normalize here so the
	// test asserts presence regardless of the propagator's casing.
	got := make(map[string]string, len(headers))
	for _, h := range headers {
		got[strings.ToLower(h.Key)] = string(h.Value)
	}

	// Caller-supplied headers must be preserved.
	assert.Equal(t, "corr-abc", got["x-correlation-id"], "caller header must survive")
	assert.Equal(t, "application/json", got["content-type"], "content-type must be attached")

	// Trace context must be injected — this is the B6 contract.
	require.Contains(t, got, "traceparent", "traceparent header must be present when ctx carries an active span")
	assert.NotEmpty(t, got["traceparent"], "traceparent must not be empty")

	// Format sanity: W3C traceparent is "00-<trace-id>-<span-id>-<flags>".
	// We don't assert the exact IDs (random), but we assert the structural
	// invariant so malformed injections fail the test.
	assert.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`, got["traceparent"])
}

// TestBuildRecordHeaders_NoSpanOmitsTraceparent guards the inverse: when ctx
// has no active span, we must not inject spurious/empty trace headers that
// would confuse downstream consumers.
func TestBuildRecordHeaders_NoSpanOmitsTraceparent(t *testing.T) {
	originalPropagator := otel.GetTextMapPropagator()

	t.Cleanup(func() { otel.SetTextMapPropagator(originalPropagator) })

	otel.SetTextMapPropagator(propagation.TraceContext{})

	msg := Message{
		Topic:   "ledger.balance.operations",
		Payload: []byte(`{}`),
	}

	headers := buildRecordHeaders(context.Background(), msg)

	for _, h := range headers {
		assert.NotEqual(t, "traceparent", strings.ToLower(h.Key), "traceparent must not be injected without an active span")
	}
}
