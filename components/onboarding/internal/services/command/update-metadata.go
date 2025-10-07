// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates metadata for an entity in MongoDB using merge semantics.
//
// This method updates entity metadata following RFC 7396 JSON Merge Patch semantics:
//   - Fetches existing metadata from MongoDB
//   - Merges new metadata with existing metadata
//   - Null values in new metadata delete fields from existing metadata
//   - Non-null values in new metadata update or add fields
//   - Fields not mentioned in new metadata are preserved
//
// Merge Behavior:
//   - If metadata is nil: Replaces with empty map (clears all metadata)
//   - If metadata is empty map: Preserves existing metadata
//   - If metadata has values: Merges with existing (new values override)
//   - If no existing metadata: Creates new metadata document
//
// This allows for:
//   - Partial metadata updates (only update specific keys)
//   - Field deletion (set field to null)
//   - Additive updates (add new fields without affecting existing)
//
// Parameters:
//   - ctx: Context for tracing and logging
//   - entityName: Type name of the entity (e.g., "Organization", "Account")
//   - entityID: UUID string of the entity
//   - metadata: Map of key-value pairs to merge (nil clears all)
//
// Returns:
//   - map[string]any: The merged metadata result
//   - error: Database error if fetch or update fails
//
// Example:
//
//	// Existing metadata: {"dept": "Finance", "region": "US"}
//	// Update with: {"region": "EU", "cost_center": 123}
//	// Result: {"dept": "Finance", "region": "EU", "cost_center": 123}
//
//	update := map[string]any{
//	    "region":      "EU",
//	    "cost_center": 123,
//	}
//	result, err := uc.UpdateMetadata(ctx, "Account", accountID, update)
//
// OpenTelemetry:
//   - Creates span "command.update_metadata"
//   - Records errors as span events
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
