// Package command provides CQRS command handlers for the onboarding component.
package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
)

// CreateMetadata creates metadata for an entity in MongoDB.
//
// This function stores arbitrary key-value pairs associated with any entity
// in the system. Metadata provides a flexible extension mechanism for entities
// without requiring schema changes.
//
// # Metadata Purpose
//
// Metadata enables:
//   - Custom fields per entity instance
//   - Integration with external systems (external IDs, references)
//   - Business-specific attributes (categories, tags)
//   - Audit information (source system, import batch)
//
// # Nil Handling
//
// If metadata is nil, the function returns nil without creating a document.
// This is a no-op pattern that simplifies calling code - callers don't need
// to check for nil before calling.
//
// # Storage
//
// Metadata is stored in MongoDB (not PostgreSQL) because:
//   - Flexible schema supports arbitrary key-value pairs
//   - No migrations needed for new fields
//   - Efficient document-based storage
//   - Separate scaling from transactional data
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_metadata"
//  3. If metadata is nil, return nil (no-op)
//  4. Build MongoDB metadata document with timestamps
//  5. Persist metadata to MongoDB via repository
//  6. Return the metadata or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - entityName: The type name of the entity (e.g., "Organization", "Ledger")
//   - entityID: The unique identifier of the entity (UUID string)
//   - metadata: Key-value pairs to store (can be nil for no-op)
//
// # Returns
//
//   - map[string]any: The stored metadata (nil if input was nil)
//   - error: If MongoDB create operation fails
//
// # Error Scenarios
//
//   - MongoDB connection failure
//   - Document write failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.create_metadata".
// Logs entity name and ID at info level, errors at error level.
//
// # Example
//
//	metadata := map[string]any{
//	    "external_id": "CRM-12345",
//	    "source":      "import",
//	    "tags":        []string{"vip", "corporate"},
//	}
//	stored, err := uc.CreateMetadata(ctx, "Organization", orgID, metadata)
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
