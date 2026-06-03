// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ctxutil

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// NewLoggerFromContext extracts a logger from the project context wrapper.
func NewLoggerFromContext(ctx context.Context) log.Logger {
	if customContext, ok := ctx.Value(libObservability.ContextKey).(*libObservability.ContextValue); ok &&
		customContext != nil && customContext.Logger != nil {
		return customContext.Logger
	}

	return &log.NopLogger{}
}

// NewTracerFromContext extracts a tracer from the project context wrapper.
func NewTracerFromContext(ctx context.Context) trace.Tracer {
	if customContext, ok := ctx.Value(libObservability.ContextKey).(*libObservability.ContextValue); ok &&
		customContext != nil && customContext.Tracer != nil {
		return customContext.Tracer
	}

	return noop.Tracer{}
}

// ContextWithLogger returns a context with the given logger stored under the
// lib-observability context key so that NewLoggerFromContext can retrieve it.
func ContextWithLogger(ctx context.Context, logger log.Logger) context.Context {
	values, _ := ctx.Value(libObservability.ContextKey).(*libObservability.ContextValue)
	if values == nil {
		values = &libObservability.ContextValue{}
	}

	values.Logger = logger

	return context.WithValue(ctx, libObservability.ContextKey, values)
}

// ContextWithTracer returns a context with the given tracer stored under the
// lib-observability context key so that NewTracerFromContext can retrieve it.
func ContextWithTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	values, _ := ctx.Value(libObservability.ContextKey).(*libObservability.ContextValue)
	if values == nil {
		values = &libObservability.ContextValue{}
	}

	values.Tracer = tracer

	return context.WithValue(ctx, libObservability.ContextKey, values)
}

// HeaderIDFromContext extracts the request ID from lib-observability context metadata.
func HeaderIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return uuid.New().String()
	}

	if customContext, ok := ctx.Value(libObservability.ContextKey).(*libObservability.ContextValue); ok && customContext != nil {
		if customContext.HeaderID != "" {
			return customContext.HeaderID
		}
	}

	return uuid.New().String()
}
