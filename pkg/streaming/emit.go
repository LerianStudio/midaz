// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"context"
	"os"
	"strconv"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"go.opentelemetry.io/otel/trace"
)

const (
	importantEmitTimeoutEnv     = "STREAMING_IMPORTANT_EMIT_TIMEOUT_MS"
	defaultImportantEmitTimeout = 5 * time.Second
)

// EventBuilder builds a typed lib-streaming event using the resolved tenant ID
// and the caller's configured CloudEvents source.
type EventBuilder func(tenantID, source string) (libStreaming.Event, error)

// EmitImportant centralizes IMPORTANT-posture direct emission mechanics.
// Build and emit failures are recorded on the provided span and logged at Warn,
// but never returned to the caller.
func EmitImportant(ctx context.Context, span trace.Span, logger libLog.Logger, emitter libStreaming.Emitter, source string, eventKey string, build EventBuilder) {
	if emitter == nil {
		return
	}

	event, buildErr := build(ResolveTenantID(ctx), source)
	if buildErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build "+eventKey+" event", buildErr)
		logger.Log(ctx, libLog.LevelWarn, "Skipping "+eventKey+" emit; build failed", libLog.Err(buildErr))

		return
	}

	emitCtx, cancel := context.WithTimeout(ctx, importantEmitTimeout())
	defer cancel()

	if emitErr := emitter.Emit(emitCtx, event); emitErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to emit "+eventKey, emitErr)
		logger.Log(ctx, libLog.LevelWarn, "Streaming emit failed for "+eventKey, libLog.Err(emitErr))
	}
}

func importantEmitTimeout() time.Duration {
	value := os.Getenv(importantEmitTimeoutEnv)
	if value == "" {
		return defaultImportantEmitTimeout
	}

	milliseconds, err := strconv.Atoi(value)
	if err != nil || milliseconds <= 0 {
		return defaultImportantEmitTimeout
	}

	return time.Duration(milliseconds) * time.Millisecond
}
