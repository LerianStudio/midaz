// Package command provides CQRS command handlers for onboarding domain operations.
//
// This package implements the command side of the CQRS pattern for the onboarding
// bounded context, handling all write operations for ledger entities:
//   - Organizations, Ledgers, Assets, Accounts
//   - Segments, Portfolios, Account Types
//   - Entity metadata management
//
// Command handlers in this package:
//   - Validate business rules before persistence
//   - Coordinate with multiple repositories (PostgreSQL + MongoDB)
//   - Emit domain events via RabbitMQ (when applicable)
//   - Integrate with external services via gRPC (balance management)
//
// Architecture:
//
// Commands follow the UseCase pattern where a single UseCase struct aggregates
// all repository dependencies. Each command method:
//  1. Extracts tracing context (logger, tracer, requestID)
//  2. Creates OpenTelemetry span for observability
//  3. Validates input and business rules
//  4. Persists changes to PostgreSQL (primary data)
//  5. Persists metadata to MongoDB (document store)
//  6. Returns updated entity or error
//
// Error Handling:
//
// Commands return domain-specific business errors that are mapped to HTTP status
// codes by the transport layer. Common error scenarios:
//   - Entity not found (404)
//   - Validation failures (400)
//   - Authorization failures (401/403)
//   - Database conflicts (409)
//
// Related Packages:
//   - components/onboarding/internal/services/query: Read operations (CQRS query side)
//   - components/onboarding/internal/adapters: Repository implementations
//   - pkg/mmodel: Domain models and DTOs
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// UpdateMetadata updates entity metadata in MongoDB with merge semantics.
//
// This method provides a generic metadata update capability for any entity type
// in the onboarding domain. It implements merge semantics where new metadata
// keys are added and existing keys are updated (not replaced entirely).
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Existing Metadata Retrieval
//	  - Query MongoDB for current metadata by entity type and ID
//	  - If retrieval fails: Return error (prevents data loss from blind overwrites)
//
//	Step 3: Metadata Merge
//	  - If input metadata is nil: Initialize empty map
//	  - If existing metadata found: Merge input with existing (input takes precedence)
//	  - Merge preserves keys not present in input
//
//	Step 4: Persistence
//	  - Update merged metadata in MongoDB
//	  - Return updated metadata map
//
// Merge Semantics:
//
// The merge operation uses libCommons.MergeMaps which:
//   - Preserves existing keys not present in new metadata
//   - Overwrites existing keys with new values
//   - Adds new keys from input metadata
//
// Example:
//
//	Existing: {"key1": "old", "key2": "preserved"}
//	Input:    {"key1": "new", "key3": "added"}
//	Result:   {"key1": "new", "key2": "preserved", "key3": "added"}
//
// Parameters:
//   - ctx: Request context with tracing information
//   - entityName: Type name of the entity (e.g., "Ledger", "Account")
//   - entityID: UUID string of the entity
//   - metadata: New metadata key-value pairs to merge
//
// Returns:
//   - map[string]any: Merged metadata after update
//   - error: MongoDB retrieval or update error
//
// Error Scenarios:
//   - MongoDB connection failure during retrieval
//   - MongoDB connection failure during update
//   - Context cancellation
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
