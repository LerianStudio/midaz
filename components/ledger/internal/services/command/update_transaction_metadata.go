// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
)

func (uc *UseCase) UpdateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	var existingMetadata *mongodb.Metadata

	if metadata != nil {
		var err error

		existingMetadata, err = uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb", err)

			logger.Log(ctx, libLog.LevelError, "Error get metadata on mongodb", libLog.Err(err))

			return nil, err
		}
	}

	result, err := uc.updateTransactionMetadataFromSnapshot(ctx, entityName, entityID, metadata, existingMetadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)
	}

	return result, err
}

func (uc *UseCase) updateTransactionMetadataFromSnapshot(
	ctx context.Context,
	entityName, entityID string,
	metadata map[string]any,
	existingMetadata *mongodb.Metadata,
) (map[string]any, error) {
	metadataToUpdate := metadata
	if metadataToUpdate == nil {
		metadataToUpdate = map[string]any{}
	} else if existingMetadata != nil {
		metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
	}

	if err := uc.TransactionMetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		return nil, err
	}

	return metadataToUpdate, nil
}
