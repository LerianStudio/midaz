// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordingContext wires an in-memory SpanRecorder into the lib-observability tracking
// context so use-case methods record onto inspectable SDK spans.
func recordingContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx := libObservability.ContextWithTracer(context.Background(), tp.Tracer("command-test"))

	return ctx, recorder
}

// findSpan returns the first ended span whose name matches, failing the test if absent.
func findSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return s
		}
	}

	t.Fatalf("span %q was not recorded; recorded spans: %v", name, spanNames(recorder))

	return nil
}

func spanNames(recorder *tracetest.SpanRecorder) []string {
	names := make([]string, 0, len(recorder.Ended()))
	for _, s := range recorder.Ended() {
		names = append(names, s.Name())
	}

	return names
}

// findEvent returns the first event on s with the given name.
func findEvent(s sdktrace.ReadOnlySpan, eventName string) (sdktrace.Event, bool) {
	for _, e := range s.Events() {
		if e.Name == eventName {
			return e, true
		}
	}

	return sdktrace.Event{}, false
}

// eventText renders an event's attribute values for substring matching. The two span-error
// helpers name their events differently — a business event carries the message as the event
// NAME, while a technical one records an OTel `exception` event carrying the message in
// exception.message — so the message has to be looked for in both places.
func eventText(e sdktrace.Event) string {
	parts := make([]string, 0, len(e.Attributes))
	for _, a := range e.Attributes {
		parts = append(parts, a.Value.Emit())
	}

	return strings.Join(parts, " ")
}

// boolAttrs indexes a span's boolean attributes by key.
func boolAttrs(s sdktrace.ReadOnlySpan) map[string]bool {
	out := make(map[string]bool, len(s.Attributes()))

	for _, a := range s.Attributes() {
		if a.Value.Type() == attribute.BOOL {
			out[string(a.Key)] = a.Value.AsBool()
		}
	}

	return out
}
