package command

import (
	"context"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new metadata operation with telemetry for update
	metadataOpID := entityName + "-" + entityID
	op := uc.Telemetry.NewEntityOperation("metadata", "update", metadataOpID)

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

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to get metadata on mongodb", err)

			logger.Errorf("Error get metadata on mongodb: %v", err)

			// Record error
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "find_error", err)

			return nil, err
		}

		if existingMetadata != nil {
			metadataToUpdate = pkg.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		metadataToUpdate = map[string]any{}
	}

	if err := uc.MetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata on mongodb", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		return nil, err
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return metadataToUpdate, nil
}
