// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for updating metadata.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates metadata for a given entity in MongoDB using merge semantics.
//
// This function merges the provided metadata with the existing metadata for an entity,
// following RFC 7396 JSON Merge Patch semantics. Fields with null values in the
// input will be deleted, while non-null values will be added or updated.
// If the input metadata is nil, all existing metadata for the entity will be cleared.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - entityName: The type name of the entity (e.g., "Organization", "Account").
//   - entityID: The UUID string of the entity.
//   - metadata: A map of key-value pairs to merge with the existing metadata.
//
// Returns:
//   - map[string]any: The resulting merged metadata.
//   - error: An error if the update fails.
func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb", err)

			logger.Errorf("Error get metadata on mongodb: %v", err)

			return nil, err
		}

		if existingMetadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		metadataToUpdate = map[string]any{}
	}

	if err := uc.MetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}
