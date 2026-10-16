// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package spantest provides in-memory span recording and lookup helpers for
// ledger unit tests that assert on the spans a use case or adapter emits.
//
// It is a test-only package: import it only from _test.go files. Production
// code MUST NOT import it, so the OTel SDK test recorder never reaches a
// shipped binary through it.
package spantest

import (
	"context"
	"strings"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// tracerName is the instrumentation scope of the spans recorded through RecordingContext.
const tracerName = "spantest"

// RecordingContext wires an in-memory SpanRecorder into the lib-observability tracking
// context so use-case methods record onto inspectable SDK spans. Each call builds its own
// provider and recorder, so parallel tests never share recorded state. The recorder is a
// synchronous span processor and starts no goroutine, so nothing needs shutting down.
func RecordingContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx := libObservability.ContextWithTracer(context.Background(), tp.Tracer(tracerName))

	return ctx, recorder
}

// FindSpan returns the first ended span whose name matches, failing the test if absent.
func FindSpan(tb testing.TB, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	tb.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return s
		}
	}

	tb.Fatalf("span %q was not recorded; recorded spans: %v", name, SpanNames(recorder))

	return nil
}

// HasSpan reports whether an ended span with the given name was recorded.
func HasSpan(recorder *tracetest.SpanRecorder, name string) bool {
	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return true
		}
	}

	return false
}

// CountSpans returns how many ended spans carry the given name.
func CountSpans(recorder *tracetest.SpanRecorder, name string) int {
	count := 0

	for _, s := range recorder.Ended() {
		if s.Name() == name {
			count++
		}
	}

	return count
}

// AssertNoSpan fails the test when any of the given span names was recorded. It documents
// that a step was folded into its parent span instead of opening a span of its own.
func AssertNoSpan(tb testing.TB, recorder *tracetest.SpanRecorder, names ...string) {
	tb.Helper()

	for _, name := range names {
		if HasSpan(recorder, name) {
			tb.Errorf("span %q must not be recorded; recorded spans: %v", name, SpanNames(recorder))
		}
	}
}

// SpanNames lists the names of every ended span, in end order.
func SpanNames(recorder *tracetest.SpanRecorder) []string {
	ended := recorder.Ended()

	names := make([]string, 0, len(ended))
	for _, s := range ended {
		names = append(names, s.Name())
	}

	return names
}

// FindEvent returns the first event on s with the given name.
func FindEvent(s sdktrace.ReadOnlySpan, eventName string) (sdktrace.Event, bool) {
	for _, e := range s.Events() {
		if e.Name == eventName {
			return e, true
		}
	}

	return sdktrace.Event{}, false
}

// EventText renders an event's attribute values for substring matching. The two span-error
// helpers name their events differently — a business event carries the message as the event
// NAME, while a technical one records an OTel `exception` event carrying the message in
// exception.message — so the message has to be looked for in both places.
func EventText(e sdktrace.Event) string {
	parts := make([]string, 0, len(e.Attributes))
	for _, a := range e.Attributes {
		parts = append(parts, a.Value.String())
	}

	return strings.Join(parts, " ")
}

// BoolAttrs indexes a span's boolean attributes by key.
func BoolAttrs(s sdktrace.ReadOnlySpan) map[string]bool {
	out := make(map[string]bool, len(s.Attributes()))

	for _, a := range s.Attributes() {
		if a.Value.Type() == attribute.BOOL {
			out[string(a.Key)] = a.Value.AsBool()
		}
	}

	return out
}
