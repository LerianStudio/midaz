// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ctxutil

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewLoggerFromContext(t *testing.T) {
	t.Parallel()

	sharedLogger := &log.NopLogger{}

	tests := []struct {
		name         string
		setupCtx     func() context.Context
		expectSame   log.Logger
		expectIsNone bool
	}{
		{
			name: "Context with logger",
			setupCtx: func() context.Context {
				return ContextWithLogger(context.Background(), sharedLogger)
			},
			expectSame: sharedLogger,
		},
		{
			name: "Empty context - returns NopLogger",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectIsNone: true,
		},
		{
			name: "Context with ContextValue but nil logger",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), libObservability.ContextKey, &libObservability.ContextValue{
					Logger: nil,
				})
			},
			expectIsNone: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tt.setupCtx()
			logger := NewLoggerFromContext(ctx)

			assert.NotNil(t, logger)

			if tt.expectSame != nil {
				assert.Equal(t, tt.expectSame, logger, "Expected the exact logger instance that was set")
			} else if tt.expectIsNone {
				_, isNopLogger := logger.(*log.NopLogger)
				assert.True(t, isNopLogger, "Expected logger to be *log.NopLogger type")
			}
		})
	}
}

func TestNewTracerFromContext(t *testing.T) {
	t.Parallel()

	sharedTracer := noop.Tracer{}

	tests := []struct {
		name       string
		setupCtx   func() context.Context
		expectSame trace.Tracer
	}{
		{
			name: "Context with tracer",
			setupCtx: func() context.Context {
				return ContextWithTracer(context.Background(), sharedTracer)
			},
			expectSame: sharedTracer,
		},
		{
			name: "Empty context - returns noop tracer",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectSame: noop.Tracer{},
		},
		{
			name: "Context with ContextValue but nil tracer - returns noop tracer",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), libObservability.ContextKey, &libObservability.ContextValue{
					Tracer: nil,
				})
			},
			expectSame: noop.Tracer{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tt.setupCtx()
			tracer := NewTracerFromContext(ctx)

			assert.NotNil(t, tracer)

			if tt.expectSame != nil {
				assert.Equal(t, tt.expectSame, tracer, "Expected the exact tracer instance that was set")
			}
		})
	}
}

func TestContextWithLogger(t *testing.T) {
	t.Parallel()

	t.Run("Success - Add logger to empty context", func(t *testing.T) {
		t.Parallel()

		logger := &log.NopLogger{}
		ctx := ContextWithLogger(context.Background(), logger)

		assert.NotNil(t, ctx)

		retrievedLogger := NewLoggerFromContext(ctx)
		assert.Equal(t, logger, retrievedLogger)
	})

	t.Run("Success - Add logger to context with existing tracer", func(t *testing.T) {
		t.Parallel()

		tracer := noop.Tracer{}
		ctx := ContextWithTracer(context.Background(), tracer)

		logger := &log.NopLogger{}
		ctx = ContextWithLogger(ctx, logger)

		retrievedLogger := NewLoggerFromContext(ctx)
		retrievedTracer := NewTracerFromContext(ctx)

		assert.Equal(t, logger, retrievedLogger)
		assert.NotNil(t, retrievedTracer)
	})

	t.Run("Success - Replace existing logger", func(t *testing.T) {
		t.Parallel()

		logger1 := &log.NopLogger{}
		ctx := ContextWithLogger(context.Background(), logger1)

		logger2 := &log.NopLogger{}
		ctx = ContextWithLogger(ctx, logger2)

		retrievedLogger := NewLoggerFromContext(ctx)
		assert.Equal(t, logger2, retrievedLogger)
	})
}

func TestContextWithTracer(t *testing.T) {
	t.Parallel()

	t.Run("Success - Add tracer to empty context", func(t *testing.T) {
		t.Parallel()

		tracer := noop.Tracer{}
		ctx := ContextWithTracer(context.Background(), tracer)

		assert.NotNil(t, ctx)

		retrievedTracer := NewTracerFromContext(ctx)
		assert.NotNil(t, retrievedTracer)
	})

	t.Run("Success - Add tracer to context with existing logger", func(t *testing.T) {
		t.Parallel()

		logger := &log.NopLogger{}
		ctx := ContextWithLogger(context.Background(), logger)

		tracer := noop.Tracer{}
		ctx = ContextWithTracer(ctx, tracer)

		retrievedLogger := NewLoggerFromContext(ctx)
		retrievedTracer := NewTracerFromContext(ctx)

		assert.Equal(t, logger, retrievedLogger)
		assert.NotNil(t, retrievedTracer)
	})
}

// TestPropagation_LibObservabilityWritePath_ReadByCtxutil is the silent-break
// guard for the lib-observability re-source: it asserts that values written via
// lib-observability's OWN context write path (the path NewTrackingFromContext /
// ContextWith* use under libObservability.ContextKey) are read back identically
// through reporter's ctxutil accessors. If ctxutil ever read a different key
// than lib-observability writes, logger/tracer/HeaderID propagation would break
// silently on the production path; this test fails loudly instead.
func TestPropagation_LibObservabilityWritePath_ReadByCtxutil(t *testing.T) {
	t.Parallel()

	logger := &log.NopLogger{}
	tracer := noop.Tracer{}
	const headerID = "req-1234567890"

	ctx := context.Background()
	ctx = libObservability.ContextWithLogger(ctx, logger)
	ctx = libObservability.ContextWithTracer(ctx, tracer)
	ctx = libObservability.ContextWithHeaderID(ctx, headerID)

	assert.Equal(t, logger, NewLoggerFromContext(ctx), "logger written via lib-observability must round-trip through ctxutil")
	assert.Equal(t, tracer, NewTracerFromContext(ctx), "tracer written via lib-observability must round-trip through ctxutil")
	assert.Equal(t, headerID, HeaderIDFromContext(ctx), "HeaderID written via lib-observability must round-trip through ctxutil")
}
