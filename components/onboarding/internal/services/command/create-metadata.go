package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	ctx, span := tracer.Start(ctx, "command.create_metadata")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "metadata", "create",
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID))

	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   entityID,
			EntityName: entityName,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, entityName, &meta); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create metadata", err)
			logger.Errorf("Error into creating %s metadata: %v", entityName, err)

			// Record error
			uc.recordOnboardingError(ctx, "metadata", "creation_error",
				attribute.String("entity_name", entityName),
				attribute.String("entity_id", entityID),
				attribute.String("error_detail", err.Error()))

			return nil, err
		}

		// Record successful completion and duration
		uc.recordOnboardingDuration(ctx, startTime, "metadata", "create", "success",
			attribute.String("entity_name", entityName),
			attribute.String("entity_id", entityID))

		return metadata, nil
	}

	return nil, nil
}
