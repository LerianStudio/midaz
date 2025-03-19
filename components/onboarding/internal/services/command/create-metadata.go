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

	// Create a new metadata operation with telemetry
	metadataOpID := entityName + "-" + entityID // Use entityName and entityID for the operation ID
	op := uc.Telemetry.NewEntityOperation("metadata", "create", metadataOpID)

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID),
	)

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   entityID,
			EntityName: entityName,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, entityName, &meta); err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to create metadata", err)
			logger.Errorf("Error into creating %s metadata: %v", entityName, err)

			// Record error
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "creation_error", err)

			return nil, err
		}

		// Mark operation as successful
		op.End(ctx, "success")

		return metadata, nil
	}

	// No metadata to create, still mark as successful
	op.End(ctx, "success")

	return nil, nil
}
