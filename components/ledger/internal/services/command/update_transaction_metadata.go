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

// metadataUpdateAttempts bounds the reads and conditional writes of one transaction or operation
// metadata update that keeps losing to concurrent writes.
const metadataUpdateAttempts = 3

// UpdateTransactionMetadata merges metadata into the stored document; nil clears it. A
// transaction or operation keeps every reserved key the ledger wrote, read fresh.
func (uc *UseCase) UpdateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	if entityName == constant.EntityTransaction || entityName == constant.EntityOperation {
		return uc.updateLedgerWrittenMetadata(ctx, span, logger, entityName, entityID, metadata)
	}

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to get metadata on mongodb", err)

			return nil, err
		}

		if existingMetadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		metadataToUpdate = map[string]any{}
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
			// No entity name: completion adds the frozen keys this document lacks and names it.
			now := time.Now()
			document := &mongodb.Metadata{EntityID: entityID, Data: mongodb.JSON{}, CreatedAt: now, UpdatedAt: now}

			if err := uc.TransactionMetadataRepo.Create(ctx, entityName, document); err != nil {
				recordCommandError(ctx, span, logger, "Failed to create metadata on mongodb", err)

				return nil, err
			}

			continue
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
