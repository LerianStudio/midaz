package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "metadata", "update",
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID))

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb", err)

			logger.Errorf("Error get metadata on mongodb: %v", err)

			// Record error
			uc.recordOnboardingError(ctx, "metadata", "find_error",
				attribute.String("entity_name", entityName),
				attribute.String("entity_id", entityID),
				attribute.String("error_detail", err.Error()))

			return nil, err
		}

		if existingMetadata != nil {
			metadataToUpdate = pkg.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		metadataToUpdate = map[string]any{}
	}

	if err := uc.MetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on mongodb", err)

		// Record error
		uc.recordOnboardingError(ctx, "metadata", "update_error",
			attribute.String("entity_name", entityName),
			attribute.String("entity_id", entityID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "metadata", "update", "success",
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID))

	return metadataToUpdate, nil
}
