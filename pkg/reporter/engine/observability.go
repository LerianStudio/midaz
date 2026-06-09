// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"go.opentelemetry.io/otel/trace"
)

// obs adapts the worker's lib-observability tracer onto the engine's minimal
// Observability port. It wraps the trace.Tracer the worker already builds
// (telemetry.Tracer(cfg.OtelLibraryName)) so engine-internal operations join the
// same trace as the surrounding reporter span — free tracing continuity without
// the engine core importing a tracing library.
type obs struct {
	tracer trace.Tracer
}

// Compile-time check that obs satisfies the engine's optional Observability
// port.
var _ fetcher.Observability = (*obs)(nil)

// NewObservability builds an Observability adapter over the given tracer. A nil
// tracer yields a no-op adapter so telemetry-disabled deployments never panic.
func NewObservability(tracer trace.Tracer) fetcher.Observability {
	return &obs{tracer: tracer}
}

// StartSpan starts a span for the named engine operation and returns the derived
// context plus an end function the engine defers. With a nil tracer it returns
// the context unchanged and a no-op end function, so a telemetry-disabled
// deployment behaves identically minus the span.
func (o *obs) StartSpan(ctx context.Context, operation string) (context.Context, func()) {
	if o.tracer == nil {
		return ctx, func() {}
	}

	ctx, span := o.tracer.Start(ctx, operation)

	return ctx, func() { span.End() }
}
