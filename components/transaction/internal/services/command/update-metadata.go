// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates metadata for an entity using merge semantics.
//
// This method implements RFC 7396 JSON Merge Patch for metadata updates:
// 1. Fetches existing metadata from MongoDB
// 2. Merges new metadata with existing (new values override)
// 3. Null values in new metadata delete fields
// 4. Updates metadata in MongoDB
// 5. Returns merged metadata
//
// Merge Behavior:
//   - If metadata is nil: Replaces with empty map (clears all metadata)
//   - If metadata is empty map: Preserves existing metadata
//   - If metadata has values: Merges with existing
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - entityName: Type of entity (e.g., "Transaction", "Operation")
//   - entityID: UUID of the entity
//   - metadata: New metadata to merge
//
// Returns:
//   - map[string]any: Merged metadata
//   - error: Database error if fetch or update fails
//
// OpenTelemetry: Creates span "command.update_metadata"
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
