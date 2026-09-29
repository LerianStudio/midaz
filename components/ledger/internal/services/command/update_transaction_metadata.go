// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// metadataUpdateAttempts bounds the reads and writes of one transaction or operation metadata
// update: its own create plus three conditional writes, more than two completions can defeat.
const metadataUpdateAttempts = 4

// UpdateTransactionMetadata applies a metadata patch to the entity's metadata document.
// Nil clears an existing document and creates none; an empty map returns the
// stored metadata without writing; a non-empty map is merged into the stored
// metadata and upserted. A transaction or operation keeps every reserved key
// the ledger wrote, read fresh.
func (uc *UseCase) UpdateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	if entityName == constant.EntityTransaction || entityName == constant.EntityOperation {
		return uc.updateLedgerWrittenMetadata(ctx, span, logger, entityName, entityID, metadata)
	}

	existingMetadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to get metadata on mongodb", err)

		return nil, err
	}

	var metadataToUpdate map[string]any

	switch {
	case metadata == nil:
		if existingMetadata == nil {
			return nil, nil
		}

		metadataToUpdate = map[string]any{}
	case len(metadata) == 0:
		if existingMetadata == nil {
			return nil, nil
		}

		return existingMetadata.Data, nil
	default:
		metadataToUpdate = metadata

		if existingMetadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		}
	}

	if err := uc.TransactionMetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}

// updateLedgerWrittenMetadata merges into the stored document read on each attempt, or keeps only
// its reserved keys for a nil body, and writes only while its entity name and fee-debt keys still
// hold what was read: a concurrent completion writing any of them makes it read and merge again.
// A nil or empty body against a missing document writes nothing; an empty (non-nil) body against
// an existing document returns it unchanged without writing.
func (uc *UseCase) updateLedgerWrittenMetadata(
	ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, metadata map[string]any,
) (map[string]any, error) {
	for range metadataUpdateAttempts {
		stored, err := uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to get metadata on mongodb", err)

			return nil, err
		}

		if stored == nil {
			if len(metadata) == 0 {
				return nil, nil
			}

			now := time.Now()
			document := &mongodb.Metadata{EntityID: entityID, Data: mongodb.JSON{}, CreatedAt: now, UpdatedAt: now}

			if err := uc.TransactionMetadataRepo.Create(ctx, entityName, document); err != nil {
				recordCommandError(ctx, span, logger, "Failed to create metadata on mongodb", err)

				return nil, err
			}

			continue
		}

		if metadata != nil && len(metadata) == 0 {
			return stored.Data, nil
		}

		merged := make(map[string]any)

		for key, value := range stored.Data {
			if metadata != nil || constant.IsReservedMetadataKey(key) {
				merged[key] = value
			}
		}

		merged = libCommons.MergeMaps(metadata, merged)

		guard := make(map[string]any, len(feeDebtMetadataKeys))
		for _, key := range feeDebtMetadataKeys {
			guard[key] = stored.Data[key]
		}

		updated, err := uc.TransactionMetadataRepo.UpdateIfUnchanged(ctx, entityName, entityID, stored.EntityName, merged, guard)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to update metadata on mongodb", err)

			return nil, err
		}

		if updated {
			return merged, nil
		}
	}

	err := fmt.Errorf("update %s metadata: the document kept changing", entityName)
	recordCommandError(ctx, span, logger, "Failed to update metadata on mongodb", err)

	return nil, err
}
