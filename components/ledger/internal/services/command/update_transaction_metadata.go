// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// UpdateTransactionMetadata merges metadata into the stored document; nil clears it. A
// transaction or operation writes only client keys, field by field, so a reserved key the ledger
// writes concurrently is never overwritten or dropped.
func (uc *UseCase) UpdateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	if entityName == constant.EntityTransaction || entityName == constant.EntityOperation {
		return uc.updateClientMetadataFields(ctx, span, logger, entityName, entityID, metadata)
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

// updateClientMetadataFields sets each non-nil key and removes each nil one; clearing removes
// every stored client key. The response is the resulting stored document.
func (uc *UseCase) updateClientMetadataFields(
	ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, metadata map[string]any,
) (map[string]any, error) {
	fields := metadata
	if fields == nil {
		existingMetadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to get metadata on mongodb", err)

			return nil, err
		}

		fields = map[string]any{}

		if existingMetadata != nil {
			for key := range existingMetadata.Data {
				if !constant.IsReservedMetadataKey(key) {
					fields[key] = nil
				}
			}
		}
	}

	updated, err := uc.TransactionMetadataRepo.UpdateFields(ctx, entityName, entityID, fields)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return updated.Data, nil
}
