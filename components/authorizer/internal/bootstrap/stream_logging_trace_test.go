// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// errStreamFailed is a sentinel test error for the Warnf path. Defined at
// package scope so the err113 linter is satisfied (no ad-hoc errors.New in
// assertions).
var errStreamFailed = errors.New("stream failed")

// fakeServerStream is a minimal grpc.ServerStream carrying a user-controlled
// context. grpc's real stream types are private; this covers only the
// ss.Context() surface that streamLoggingInterceptor reads.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// TestStreamLoggingInterceptor_IncludesTraceID verifies every log line
// emitted by streamLoggingInterceptor carries trace_id / span_id fields
// extracted from the stream context. Prior to D10-B5 the logging path ran
// independently of the telemetry interceptor and had no correlation IDs —
// operators correlating an APM trace to a log line had to reconstruct the
// mapping from method + timestamp, which fails entirely under load.
func TestStreamLoggingInterceptor_IncludesTraceID(t *testing.T) {
	// Build a real tracer provider so SpanContext().IsValid() returns true.
	tp := sdktrace.NewTracerProvider()

	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("stream-logging-test")

	spanCtx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	sc := trace.SpanFromContext(spanCtx).SpanContext()
	require.True(t, sc.IsValid(), "sanity: span context must be valid")

	wantTraceID := sc.TraceID().String()
	wantSpanID := sc.SpanID().String()

	logger := &captureLogger{}
	interceptor := streamLoggingInterceptor(logger)

	info := &grpc.StreamServerInfo{
		FullMethod:     "/authorizer.v1.BalanceAuthorizer/AuthorizeStream",
		IsClientStream: true,
		IsServerStream: true,
	}

	// Case 1: successful handler — Infof path.
	err := interceptor(nil, &fakeServerStream{ctx: spanCtx}, info, func(_ any, _ grpc.ServerStream) error {
		return nil
	})
	require.NoError(t, err)

	// Case 2: handler returns a non-Canceled error — Warnf path.
	err = interceptor(nil, &fakeServerStream{ctx: spanCtx}, info, func(_ any, _ grpc.ServerStream) error {
		return errStreamFailed
	})
	require.ErrorIs(t, err, errStreamFailed)

	lines := logger.snapshot()
	require.Len(t, lines, 2, "expected both success and failure log lines")

	for _, line := range lines {
		require.Contains(t, line, "trace_id="+wantTraceID,
			"every stream log line MUST carry trace_id, got %q", line)
		require.Contains(t, line, "span_id="+wantSpanID,
			"every stream log line MUST carry span_id, got %q", line)
	}
}

// TestStreamLoggingInterceptor_InvalidContextFallsBackToNone covers the
// no-span case — a stream context with no telemetry attached must render
// trace_id=none / span_id=none so log parsers see a stable token.
func TestStreamLoggingInterceptor_InvalidContextFallsBackToNone(t *testing.T) {
	logger := &captureLogger{}
	interceptor := streamLoggingInterceptor(logger)

	info := &grpc.StreamServerInfo{FullMethod: "/x.y/Z"}

	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, func(_ any, _ grpc.ServerStream) error {
		return nil
	})
	require.NoError(t, err)

	lines := logger.snapshot()
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "trace_id=none")
	require.Contains(t, lines[0], "span_id=none")
}

// TestTraceIdentifiersFromContext_NilContext covers the defensive nil-ctx
// branch. The production interceptor never passes nil, but downstream
// callers relying on the helper benefit from the explicit guard.
func TestTraceIdentifiersFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // SA1012: nil is the scenario under test
	traceID, spanID := traceIdentifiersFromContext(nil)
	require.Equal(t, "none", traceID)
	require.Equal(t, "none", spanID)
}

// Ensure strings package is used (kept for future assertions).
var _ = strings.Contains
