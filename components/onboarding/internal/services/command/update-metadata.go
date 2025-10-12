package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates entity metadata in MongoDB using JSON Merge Patch semantics.
//
// This function implements RFC 7396 JSON Merge Patch semantics for metadata updates:
// - New keys are added
// - Existing keys with new values are updated
// - Keys with null values are removed
// - The operation is a shallow merge (not deep)
//
// If no metadata is provided (nil), it initializes an empty metadata object.
// This differs from create operations where nil metadata results in no MongoDB document.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - entityName: The type name of the entity (e.g., "Account", "Organization")
//   - entityID: The UUID string of the entity to update metadata for
//   - metadata: The metadata patch to apply (can be nil to clear metadata)
//
// Returns:
//   - map[string]any: The merged metadata result after applying the patch
//   - error: MongoDB query or update errors
func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	// Step 1: If metadata is provided, merge with existing metadata using JSON Merge Patch semantics
	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb", err)

			logger.Errorf("Error get metadata on mongodb: %v", err)

			return nil, err
		}

		// Merge new metadata with existing, giving precedence to new values
		if existingMetadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		// Step 2: If metadata is nil, initialize empty metadata object (clear all metadata)
		metadataToUpdate = map[string]any{}
	}

	// Step 3: Persist the merged metadata to MongoDB
	if err := uc.MetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}
