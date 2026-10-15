// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/rand"

	"go.opentelemetry.io/otel/trace"
)

// unsampledPollContext returns ctx carrying a valid span context whose sampled flag is
// cleared, for the BalanceSync poll only.
//
// The poll runs every few milliseconds and almost always finds nothing to do; traced as
// a root it was half of the ledger's traces, all of them empty. The tracer provider uses
// the ParentBased sampler, which drops every span whose parent is valid but not sampled,
// so the poll's own span and the Redis driver span under it are discarded on purpose.
// The flush is handed the collector's original context, not this one, so a poll that
// claims keys still records the whole flush trace.
//
// Tenant, logger and tracer stay on the returned context because it wraps ctx. If the
// sampler ever stops being ParentBased, the poll spans are simply recorded again: the
// helper then has no effect and breaks nothing.
func unsampledPollContext(ctx context.Context) context.Context {
	// One read covers both ids: 16 bytes of trace id followed by 8 of span id.
	var ids [24]byte

	// crypto/rand does not fail on supported platforms; should it ever, the poll keeps
	// the caller's context and is traced as before instead of failing the worker.
	if _, err := rand.Read(ids[:]); err != nil {
		return ctx
	}

	var (
		traceID trace.TraceID
		spanID  trace.SpanID
	)

	copy(traceID[:], ids[:len(traceID)])
	copy(spanID[:], ids[len(traceID):])

	// An all-zero id makes the span context invalid, and an invalid parent turns the poll
	// span back into a sampled root. Force a non-zero byte so that cannot happen.
	traceID[len(traceID)-1] |= 1
	spanID[len(spanID)-1] |= 1

	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(0).WithSampled(false),
	}))
}
