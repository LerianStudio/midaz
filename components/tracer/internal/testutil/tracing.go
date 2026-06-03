// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package testutil provides shared test utilities.
package testutil

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var (
	originalProviderOnce sync.Once
	originalProvider     trace.TracerProvider
	// globalProviderMu serializes tests that mutate the global tracer provider.
	// Acquired in SetupTestTracing, released in Cleanup.
	globalProviderMu sync.Mutex
)

// SpanStub is a type alias for tracetest.SpanStub for convenience.
type SpanStub = tracetest.SpanStub

// TestTracer encapsulates tracing setup for tests.
// It captures spans in memory and automatically restores the previous
// global tracer provider when the test completes.
type TestTracer struct {
	Exporter         *tracetest.InMemoryExporter
	Provider         *sdktrace.TracerProvider
	previousProvider trace.TracerProvider
	cleanupOnce      sync.Once
}

// SetupTestTracing creates a test tracer and sets it as the global provider.
// The previous global provider is automatically restored when the test completes.
// Uses t.Cleanup() to ensure proper teardown even if the test fails.
// Thread-safe: acquires globalProviderMu to serialize tests that mutate the global
// tracer provider, preventing parallel tests from clobbering each other's providers.
func SetupTestTracing(t *testing.T) *TestTracer {
	t.Helper()

	globalProviderMu.Lock()

	originalProviderOnce.Do(func() {
		originalProvider = otel.GetTracerProvider()
	})

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	otel.SetTracerProvider(tp)

	tt := &TestTracer{
		Exporter:         exporter,
		Provider:         tp,
		previousProvider: originalProvider,
	}

	t.Cleanup(func() {
		tt.Cleanup()
	})

	return tt
}

// Cleanup restores the previous provider, shuts down the test provider,
// and releases the global provider mutex so the next test can proceed.
// This is called automatically via t.Cleanup(), but can be called manually if needed.
func (tt *TestTracer) Cleanup() {
	tt.cleanupOnce.Do(func() {
		otel.SetTracerProvider(tt.previousProvider)
		_ = tt.Provider.Shutdown(context.Background())

		globalProviderMu.Unlock()
	})
}

// GetSpans returns all captured spans.
func (tt *TestTracer) GetSpans() tracetest.SpanStubs {
	return tt.Exporter.GetSpans()
}

// Reset clears all captured spans.
func (tt *TestTracer) Reset() {
	tt.Exporter.Reset()
}
