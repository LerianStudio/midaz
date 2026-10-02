// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
)

// UpdateOnboardingMetadata applies a metadata patch to the entity's metadata document.
// Nil clears an existing document and creates none; an empty map returns the
// stored metadata without writing; a non-empty map is merged into the stored
// metadata and upserted.
func (uc *UseCase) UpdateOnboardingMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	existingMetadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, entityName, entityID)
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

	if err := uc.OnboardingMetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}
