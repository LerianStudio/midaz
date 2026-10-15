// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"
)

// discardStagedMetadataMessage is the span and log message of a failed discard.
const discardStagedMetadataMessage = "Failed to discard staged metadata after a failed insert"

// discardStagedMetadataTimeout bounds the purge of a staged metadata document. The purge runs on
// a context detached from the request's cancellation, so this is its only deadline.
const discardStagedMetadataTimeout = 5 * time.Second

// discardStagedOnboardingMetadata hard-deletes, best effort, the onboarding metadata document a
// create wrote before its SQL insert failed. Nil or empty metadata means nothing was staged, so
// the repository is not called.
//
// The purge runs on context.WithoutCancel(ctx) bounded by discardStagedMetadataTimeout, so a
// request whose context already expired (the usual cause of a failed insert) still discards the
// document. It never returns an error: the insert failure is what the caller reports.
//
// A document stays behind only when the purge itself fails; that failure is recorded on span and
// logged at Error with the entity name and ID. The leftover is keyed by an entity ID that never
// exists in PostgreSQL, so a GET by ID answers 404 and metadata-filtered listings, which resolve
// matched IDs against PostgreSQL, never return it. It is not inert, though: while it exists, any
// value it holds under a unique metadata index stays reserved, and a later create that uses the
// same value is rejected as a duplicate until the document is removed.
func (uc *UseCase) discardStagedOnboardingMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}

	discardStagedMetadata(ctx, span, logger, entityName, entityID, uc.OnboardingMetadataRepo.Purge)
}

// discardStagedTransactionMetadata is discardStagedOnboardingMetadata over the transaction-side
// metadata store (TransactionMetadataRepo), with the same detached, bounded, never-returns purge
// and the same leftover semantics when the purge itself fails.
func (uc *UseCase) discardStagedTransactionMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}

	discardStagedMetadata(ctx, span, logger, entityName, entityID, uc.TransactionMetadataRepo.Purge)
}

// discardStagedMetadata runs the purge on a context that ignores the caller's cancellation and
// carries its own deadline, and reports, without returning, a failure.
func discardStagedMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, purgeFn func(ctx context.Context, collection, id string) error) {
	purgeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), discardStagedMetadataTimeout)
	defer cancel()

	if err := purgeFn(purgeCtx, entityName, entityID); err != nil {
		libOpentelemetry.HandleSpanError(span, discardStagedMetadataMessage, err)
		logger.Log(
			ctx, libLog.LevelError, discardStagedMetadataMessage,
			libLog.Err(err),
			libLog.String("entityName", entityName),
			libLog.String("entityId", entityID),
		)
	}
}
