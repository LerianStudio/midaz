// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating metadata.
package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
)

// CreateMetadata persists metadata for a given entity in MongoDB.
//
// This function stores flexible key-value data for any entity, linking it via the
// entity's name and ID. This approach allows for schema flexibility without
// requiring database migrations for metadata changes.
//
// If the provided metadata map is nil, the function will do nothing and return nil.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - entityName: The type name of the entity (e.g., "Organization", "Account").
//   - entityID: The UUID string of the entity.
//   - metadata: A map of key-value pairs to be stored.
//
// Returns:
//   - map[string]any: The created metadata, which is the same as the input map.
//   - error: An error if the persistence fails.
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
