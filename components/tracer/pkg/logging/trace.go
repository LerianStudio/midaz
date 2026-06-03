// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package logging provides utilities for structured logging with trace context.
package logging

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace"
)

// WithTrace enriches a logger with trace context (trace.id and span.id).
// This enables correlation between logs and distributed traces in observability tools.
//
// Usage:
//
//	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
//	ctx, span := tracer.Start(ctx, "handler.rule.create")
//	defer span.End()
//	logger = logging.WithTrace(ctx, logger)
//
// Returns the original logger if no valid span is found in context.
//
//nolint:ireturn
func WithTrace(ctx context.Context, logger libLog.Logger) libLog.Logger {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return logger.With(
			libLog.String("trace.id", span.SpanContext().TraceID().String()),
			libLog.String("span.id", span.SpanContext().SpanID().String()),
		)
	}

	return logger
}
