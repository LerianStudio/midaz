// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestObservability_StartSpan_DerivesContextAndEnd(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	o := NewObservability(tp.Tracer("test"))

	ctx, end := o.StartSpan(context.Background(), "engine.test.op")
	assert.NotNil(t, ctx)
	assert.NotNil(t, end)

	// The derived context carries the active span.
	assert.True(t, oteltrace.SpanFromContext(ctx).SpanContext().IsValid())

	// End must be safe to call.
	end()
}

func TestObservability_NilTracer_NoOp(t *testing.T) {
	t.Parallel()

	o := NewObservability(nil)

	ctx := context.Background()
	derived, end := o.StartSpan(ctx, "engine.test.op")

	// Nil tracer returns the context unchanged and a no-op end func — no panic.
	assert.Equal(t, ctx, derived)
	require.NotPanics(t, end)
}
