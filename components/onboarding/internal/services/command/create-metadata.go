package command

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)

func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	assert.NotEmpty(entityName, "entityName must not be empty for metadata creation",
		"operation", "CreateMetadata")
	assert.NotEmpty(entityID, "entityID must not be empty for metadata creation",
		"entity_name", entityName)

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
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata", err)
			logger.Errorf("Error creating %s metadata: %v", entityName, err)

			return nil, fmt.Errorf("failed to create: %w", err)
		}

		return metadata, nil
	}

	return nil, nil
}
