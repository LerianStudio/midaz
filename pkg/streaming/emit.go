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

// EmitRequestBuilder builds a typed lib-streaming EmitRequest using the
// resolved tenant ID.
//
// CloudEvents `source` is owned by the Builder at producer construction
// time. Catalog-bound fields (ResourceType, EventType, SchemaVersion)
// resolve from the Catalog at emit time via the DefinitionKey on the
// EmitRequest. The closure therefore takes only the request-scoped
// tenant ID.
type EmitRequestBuilder func(tenantID string) (libStreaming.EmitRequest, error)

// EmitImportant centralizes IMPORTANT-posture direct emission mechanics.
// Build and emit failures are recorded on the provided span and logged at
// Warn, but never returned to the caller — durability of IMPORTANT events
// is owned by PG + (follow-up task) the outbox subsystem, not by the
// synchronous Emit call.
//
// eventKey is the catalog DefinitionKey (e.g. "account.created"); it is
// used purely as a log/span attribution string so operators can correlate
// emit-site logs with the underlying lib-streaming request.
func EmitImportant(ctx context.Context, span trace.Span, logger libLog.Logger, emitter libStreaming.Emitter, eventKey string, build EmitRequestBuilder) {
	if emitter == nil {
		return
	}

	request, buildErr := build(ResolveTenantID(ctx))
	if buildErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build "+eventKey+" event", buildErr)
		logger.Log(ctx, libLog.LevelWarn, "Skipping "+eventKey+" emit; build failed", libLog.Err(buildErr))

		return
	}

	emitCtx, cancel := context.WithTimeout(ctx, importantEmitTimeout())
	defer cancel()

	if emitErr := emitter.Emit(emitCtx, request); emitErr != nil {
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
