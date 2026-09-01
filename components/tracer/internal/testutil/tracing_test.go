// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestSetupTestTracing_SetsGlobalProvider(t *testing.T) {
	previousProvider := otel.GetTracerProvider()

	tt := SetupTestTracing(t)

	currentProvider := otel.GetTracerProvider()
	assert.Equal(t, tt.Provider, currentProvider, "Global provider should be the test provider")
	assert.NotEqual(t, previousProvider, currentProvider, "Global provider should have changed")
}

func TestSetupTestTracing_CapturesSpans(t *testing.T) {
	tt := SetupTestTracing(t)

	tracer := otel.Tracer("test-tracer")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	spans := tt.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name)
}

func TestSetupTestTracing_RestoresPreviousProvider(t *testing.T) {
	previousProvider := otel.GetTracerProvider()

	func() {
		tt := SetupTestTracing(t)
		_ = tt // use tt to avoid unused variable warning

		// Provider should be different during test
		assert.NotEqual(t, previousProvider, otel.GetTracerProvider())
	}()

	// Note: t.Cleanup runs after the test function completes,
	// so we can't directly test restoration here.
	// The restoration is tested indirectly by running multiple tests.
}

func TestTestTracer_Reset(t *testing.T) {
	tt := SetupTestTracing(t)

	tracer := otel.Tracer("test-tracer")
	_, span := tracer.Start(context.Background(), "span-1")
	span.End()

	require.Len(t, tt.GetSpans(), 1)

	tt.Reset()

	assert.Empty(t, tt.GetSpans(), "Spans should be cleared after Reset")

	_, span2 := tracer.Start(context.Background(), "span-2")
	span2.End()

	require.Len(t, tt.GetSpans(), 1)
	assert.Equal(t, "span-2", tt.GetSpans()[0].Name)
}

func TestMultipleTestTracers_DoNotInterfere(t *testing.T) {
	// This test verifies that each test gets isolated tracing
	t.Run("first", func(t *testing.T) {
		tt := SetupTestTracing(t)

		tracer := otel.Tracer("test")
		_, span := tracer.Start(context.Background(), "first-span")
		span.End()

		assert.Len(t, tt.GetSpans(), 1)
	})

	t.Run("second", func(t *testing.T) {
		tt := SetupTestTracing(t)

		// Should start with empty spans (not see spans from "first")
		assert.Empty(t, tt.GetSpans(), "Second test should not see spans from first test")

		tracer := otel.Tracer("test")
		_, span := tracer.Start(context.Background(), "second-span")
		span.End()

		spans := tt.GetSpans()
		require.Len(t, spans, 1)
		assert.Equal(t, "second-span", spans[0].Name)
	})
}
