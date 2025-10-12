package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
)

// CreateMetadata stores custom metadata for an entity in MongoDB.
//
// Metadata provides a flexible, schema-less extension mechanism for any entity
// (organizations, ledgers, accounts, assets, etc.). It enables storing custom
// attributes without requiring database schema changes, supporting use cases like:
// - Customer information and KYC data
// - Regulatory and compliance attributes
// - Integration references to external systems
// - Business-specific categorization
//
// Metadata is stored separately in MongoDB to avoid bloating the PostgreSQL
// relational tables and to enable flexible querying of custom attributes.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - entityName: The type name of the entity (e.g., "Organization", "Account")
//   - entityID: The UUID string of the entity this metadata belongs to
//   - metadata: A map of custom key-value pairs to store (can be nil)
//
// Returns:
//   - map[string]any: The metadata that was stored (or nil if no metadata provided)
//   - error: MongoDB persistence errors
func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	ctx, span := tracer.Start(ctx, "command.create_metadata")
	defer span.End()

	// Only persist metadata if it's provided (not nil and not empty)
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
