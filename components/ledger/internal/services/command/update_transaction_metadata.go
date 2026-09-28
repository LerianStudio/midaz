// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// UpdateTransactionMetadata merges metadata into the stored document; nil clears it. A
// transaction or operation keeps every reserved key already stored even when cleared, because
// the ledger alone writes them and a request body cannot name one.
func (uc *UseCase) UpdateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	metadataToUpdate := metadata
	if metadataToUpdate == nil {
		metadataToUpdate = map[string]any{}
	}

	if metadata != nil || entityName == constant.EntityTransaction || entityName == constant.EntityOperation {
		existingMetadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to get metadata on mongodb", err)

			return nil, err
		}

		if existingMetadata != nil && metadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		} else if existingMetadata != nil {
			for key, value := range existingMetadata.Data {
				if constant.IsReservedMetadataKey(key) {
					metadataToUpdate[key] = value
				}
			}
		}
	}

	if err := uc.TransactionMetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}
