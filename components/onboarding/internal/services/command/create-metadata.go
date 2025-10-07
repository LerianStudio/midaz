// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
)

// CreateMetadata creates metadata for an entity in MongoDB.
//
// This method persists flexible key-value metadata for any entity type. Metadata is stored
// separately from primary entity data to support:
//   - Flexible schema (no predefined fields)
//   - Large metadata values (up to 2000 characters per value)
//   - Schema evolution without migrations
//   - MongoDB's document model advantages
//
// The metadata is linked to the entity via:
//   - EntityName: Type of entity (e.g., "Organization", "Account")
//   - EntityID: UUID of the entity
//
// This allows querying metadata by entity type or ID, and supports metadata-based
// filtering in list queries.
//
// Parameters:
//   - ctx: Context for tracing and logging
//   - entityName: Type name of the entity (e.g., "Organization", "Account", "Ledger")
//   - entityID: UUID string of the entity
//   - metadata: Map of key-value pairs to store (nil is allowed)
//
// Returns:
//   - map[string]any: The created metadata (same as input), or nil if input was nil
//   - error: Database error if persistence fails, nil if successful or metadata is nil
//
// Behavior:
//   - If metadata is nil, returns (nil, nil) without creating anything
//   - If metadata is empty map, still creates the document in MongoDB
//   - Timestamps (CreatedAt, UpdatedAt) are automatically set
//
// Example:
//
//	metadata := map[string]any{
//	    "department": "Finance",
//	    "cost_center": 12345,
//	    "region": "EMEA",
//	}
//	result, err := uc.CreateMetadata(ctx, "Account", accountID, metadata)
//	if err != nil {
//	    return nil, err
//	}
//
// OpenTelemetry:
//   - Creates span "command.create_metadata"
//   - Logs entity name and ID
//   - Records errors in logs (not as span events)
func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	ctx, span := tracer.Start(ctx, "command.create_metadata")
	defer span.End()

	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   entityID,
			EntityName: entityName,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, entityName, &meta); err != nil {
			logger.Errorf("Error into creating %s metadata: %v", entityName, err)
			return nil, err
		}

		return metadata, nil
	}

	return nil, nil
}
