// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"
)

// metadataDeleter is the slice of the onboarding and transaction metadata
// repositories the soft delete needs.
type metadataDeleter interface {
	Delete(ctx context.Context, collection, id string) error
}

// metadataDeleteRetryPolicy bounds how many times, and how far apart, the
// metadata soft delete is attempted within one request.
type metadataDeleteRetryPolicy struct {
	// Attempts is the total number of Delete calls, the first one included.
	Attempts int
	// BaseDelay is the first backoff step before jitter; each attempt doubles it.
	BaseDelay time.Duration
}

// defaultMetadataDeleteRetryPolicy is the production policy: three attempts
// with full-jitter waits of at most 50 ms and 100 ms, keeping the added latency
// of a failing Mongo below 500 ms per request.
func defaultMetadataDeleteRetryPolicy() metadataDeleteRetryPolicy {
	return metadataDeleteRetryPolicy{Attempts: 3, BaseDelay: 50 * time.Millisecond}
}

// softDeleteOnboardingMetadata soft-deletes the metadata of an entity whose
// metadata lives in the onboarding Mongo database.
func (uc *UseCase) softDeleteOnboardingMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string) {
	uc.softDeleteMetadata(ctx, span, logger, uc.OnboardingMetadataRepo, entityName, entityID)
}

// softDeleteTransactionMetadata soft-deletes the metadata of an entity whose
// metadata lives in the transaction Mongo database.
func (uc *UseCase) softDeleteTransactionMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string) {
	uc.softDeleteMetadata(ctx, span, logger, uc.TransactionMetadataRepo, entityName, entityID)
}

// softDeleteMetadata marks the entity's metadata as deleted, retrying under the
// UseCase policy and stopping as soon as ctx is done. It is best-effort: once
// the attempts run out it records the failure on span and logs one Warn with
// the entity id, and never returns an error, because the entity row is already
// deleted and the client cannot repeat the DELETE, so a transient Mongo failure
// is absorbed inside the request and a persistent one is left to the operator.
func (uc *UseCase) softDeleteMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, repo metadataDeleter, entityName, entityID string) {
	policy := uc.metadataDeleteRetry
	if policy.Attempts <= 0 {
		policy = defaultMetadataDeleteRetryPolicy()
	}

	var (
		err      error
		attempts int
	)

	for attempts = 1; ; attempts++ {
		if err = repo.Delete(ctx, entityName, entityID); err == nil {
			return
		}

		if attempts >= policy.Attempts || ctx.Err() != nil {
			break
		}

		if waitErr := libBackoff.WaitContext(ctx, libBackoff.FullJitter(libBackoff.Exponential(policy.BaseDelay, attempts-1))); waitErr != nil {
			break
		}
	}

	libOpentelemetry.HandleSpanError(span, "Failed to soft delete metadata", err)

	logger.Log(ctx, libLog.LevelWarn, "Failed to soft delete metadata; the entity stays deleted",
		libLog.String("entity_name", entityName),
		libLog.String("entity_id", entityID),
		libLog.Int("attempts", attempts),
		libLog.Err(err))
}
