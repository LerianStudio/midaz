// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
)

// streamingTracerName is the OTEL tracer name attached to spans emitted by
// the lib-streaming producer. Held as a constant so the same string is used
// at bootstrap and in any subsequent observability tooling.
const streamingTracerName = "ledger-streaming"

// noopStreamingCloser is the close hook returned by BuildStreamingEmitter
// when streaming is disabled. It exists only so callers can append a single
// uniform cleanup callback to their existing chain.
func noopStreamingCloser() error { return nil }

// BuildStreamingEmitter returns the lib-streaming Emitter the ledger
// component should inject into its command UseCase, plus a close hook the
// caller must run on shutdown (or on bootstrap failure).
//
// Behaviour:
//   - When cfg.StreamingEnabled is false (the documented default for this
//     pilot) the function returns libStreaming.NewNoopEmitter() and a no-op
//     close hook. No franz-go client is constructed and no broker
//     connection is attempted.
//   - When cfg.StreamingEnabled is true the function builds a single-target
//     Producer via libStreaming.New (which itself short-circuits to the
//     NoopEmitter when no brokers are configured), wires the supplied logger
//     and the telemetry tracer, and returns the Producer's Close method as
//     the close hook.
//
// The function intentionally does NOT wire an outbox repository, a DLQ
// publisher, or a custom partition key — those are deferred to follow-up
// tasks. The TODO inside the function tracks the outbox-override in the
// catalog policy that this pilot ships with.
//
// telemetry MAY be nil in tests; the helper falls back to a no-op tracer
// in that case rather than panicking.
func BuildStreamingEmitter(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) (libStreaming.Emitter, func() error, error) {
	if cfg == nil {
		return nil, noopStreamingCloser, fmt.Errorf("BuildStreamingEmitter: nil config")
	}

	// Disabled-feature-flag fallback. Single-event pilot ships disabled by
	// default; turning the flag on requires STREAMING_BROKERS to be set as
	// well, otherwise libStreaming.New itself falls back to the NoopEmitter.
	if !cfg.StreamingEnabled {
		if logger != nil {
			logger.Log(ctx, libLog.LevelInfo, "Streaming disabled (STREAMING_ENABLED=false); using NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// TODO(streaming-outbox-task): account.created is an IMPORTANT-posture
	// event whose delivery policy ultimately requires direct emit +
	// outbox fallback on circuit-open + DLQ on routable failure. The
	// published v1.1.0 of lib-streaming does NOT export those policy
	// constants and hardcodes the policy inside the producer; outbox
	// wiring is also deferred for this pilot. When the outbox subsystem
	// lands, WithOutboxRepository(...) must be passed to libStreaming.New
	// and this override note removed.

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so
	// every STREAMING_* knob (MaxBufferedRecords, BatchMaxBytes, CB ratios,
	// CloseTimeout, etc.) gets its documented default rather than the zero
	// value of the struct. The midaz Config bindings for the subset of
	// STREAMING_* vars surfaced in launch.json / .env.example remain valid
	// — LoadConfig reads the same process environment they bound from.
	streamingCfg, err := libStreaming.LoadConfig()
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to load streaming config: %w", err)
	}

	if len(streamingCfg.Brokers) == 0 && logger != nil {
		logger.Log(ctx, libLog.LevelWarn, "STREAMING_ENABLED=true but STREAMING_BROKERS is empty; falling back to NoopEmitter")
	}

	opts := []libStreaming.EmitterOption{}
	if logger != nil {
		opts = append(opts, libStreaming.WithLogger(logger))
	}

	if telemetry != nil {
		tracer, err := telemetry.Tracer(streamingTracerName)
		if err == nil && tracer != nil {
			opts = append(opts, libStreaming.WithTracer(tracer))
		} else if err != nil && logger != nil {
			logger.Log(ctx, libLog.LevelWarn,
				fmt.Sprintf("Failed to construct streaming tracer; continuing without it: %v", err))
		}

		if telemetry.MetricsFactory != nil {
			opts = append(opts, libStreaming.WithMetricsFactory(telemetry.MetricsFactory))
		}
	}

	emitter, err := libStreaming.New(ctx, streamingCfg, opts...)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to construct streaming emitter: %w", err)
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelInfo, "Streaming emitter constructed",
			libLog.String("brokers", strings.Join(streamingCfg.Brokers, ",")),
			libLog.String("client_id", streamingCfg.ClientID),
			libLog.String("ce_source", streamingCfg.CloudEventsSource),
		)
	}

	// libStreaming.New may still return a NoopEmitter (when brokers is
	// empty); in that case Close is also a no-op so we just delegate.
	return emitter, emitter.Close, nil
}
