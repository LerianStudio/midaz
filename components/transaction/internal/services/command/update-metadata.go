// Package command provides CQRS command handlers for the transaction component.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates or creates metadata for any entity in MongoDB.
//
// This function performs an upsert operation: if metadata exists for the entity,
// it merges the new data with existing data; if no metadata exists, it creates
// new metadata with the provided data.
//
// # Merge Behavior
//
// When updating existing metadata, the function merges maps rather than replacing:
//   - New keys are added to existing metadata
//   - Existing keys are updated with new values
//   - Keys not in the update remain unchanged
//
// This enables partial updates without losing existing metadata.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span for this operation
//  3. If metadata is nil, return empty map (no-op for nil input)
//  4. Fetch existing metadata for the entity from MongoDB
//  5. If existing metadata found, merge with new metadata
//  6. Persist updated/new metadata to MongoDB
//  7. Return the final merged metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - entityName: The type name of the entity (e.g., "Transaction", "AssetRate")
//   - entityID: The unique identifier of the entity (UUID string)
//   - metadata: Key-value pairs to update/create (can be nil for no-op)
//
// # Returns
//
//   - map[string]any: The final metadata after merge (empty map if input was nil)
//   - error: If MongoDB operations fail (find or update)
//
// # Error Scenarios
//
//   - MongoDB connection failure during FindByEntity
//   - MongoDB connection failure during Update
//   - Context cancellation/timeout during operations
//
// # Observability
//
// Creates tracing span "command.update_metadata" with error events on failure.
// Logs entity name and ID at info level, errors at error level.
//
// # Example
//
//	metadata := map[string]any{
//	    "source": "api",
//	    "priority": "high",
//	}
//	updated, err := uc.UpdateMetadata(ctx, "Transaction", txID.String(), metadata)
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
