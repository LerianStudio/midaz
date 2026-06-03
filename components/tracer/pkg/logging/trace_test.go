// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package logging

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// Note: This test sets global tracer provider via otel.SetTracerProvider.
// Do NOT add t.Parallel() as it would cause race conditions.
func TestWithTrace(t *testing.T) {
	tests := []struct {
		name          string
		setupContext  func() (context.Context, func())
		expectTraceID bool
		expectSpanID  bool
		description   string
	}{
		{
			name: "Success - valid span adds trace.id and span.id",
			setupContext: func() (context.Context, func()) {
				exporter := tracetest.NewInMemoryExporter()
				tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
				otel.SetTracerProvider(tp)

				tracer := tp.Tracer("test")
				ctx, span := tracer.Start(context.Background(), "test-span")

				// Return cleanup function instead of deferring here
				cleanup := func() {
					span.End()
					_ = tp.Shutdown(context.Background())
				}

				return ctx, cleanup
			},
			expectTraceID: true,
			expectSpanID:  true,
			description:   "When context has valid span, logger should have trace.id and span.id fields",
		},
		{
			name: "Success - no span returns original logger",
			setupContext: func() (context.Context, func()) {
				return context.Background(), func() {}
			},
			expectTraceID: false,
			expectSpanID:  false,
			description:   "When context has no span, logger should be returned unchanged",
		},
		{
			name: "Success - invalid span returns original logger",
			setupContext: func() (context.Context, func()) {
				return trace.ContextWithSpan(context.Background(), nil), func() {}
			},
			expectTraceID: false,
			expectSpanID:  false,
			description:   "When context has invalid span, logger should be returned unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := testutil.NewMockLogger()
			ctx, cleanup := tt.setupContext()
			defer cleanup()

			result := WithTrace(ctx, mockLogger)

			require.NotNil(t, result, "WithTrace should never return nil")

			if tt.expectTraceID || tt.expectSpanID {
				// Should return a new logger with fields (mockLoggerFieldsRecorder)
				assert.NotEqual(t, mockLogger, result, tt.description)
			} else {
				// Should return the original logger
				assert.Equal(t, mockLogger, result, tt.description)
			}
		})
	}
}

// Note: This test sets global tracer provider via otel.SetTracerProvider.
// Do NOT add t.Parallel() as it would cause race conditions.
func TestWithTrace_FieldValues(t *testing.T) {
	// Setup real tracer to get actual trace/span IDs
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")

	defer span.End()

	mockLogger := testutil.NewMockLogger()

	// Call WithTrace
	enrichedLogger := WithTrace(ctx, mockLogger)

	// Log something to capture the fields
	enrichedLogger.Log(ctx, libLog.LevelDebug, "test message")

	// Verify the mock captured the call with fields
	require.Len(t, mockLogger.Calls, 1, "Should have one log call")

	call := mockLogger.Calls[0]
	assert.Equal(t, "debug", call.Level)

	// Convert fields to map for easier assertion
	fieldsMap := testutil.FieldsToMap(call.Fields)

	// Verify trace.id field exists and has valid format (32 hex chars)
	traceID, ok := fieldsMap["trace.id"]
	assert.True(t, ok, "Should have trace.id field")
	assert.Len(t, traceID, 32, "trace.id should be 32 hex characters")

	// Verify span.id field exists and has valid format (16 hex chars)
	spanID, ok := fieldsMap["span.id"]
	assert.True(t, ok, "Should have span.id field")
	assert.Len(t, spanID, 16, "span.id should be 16 hex characters")
}
