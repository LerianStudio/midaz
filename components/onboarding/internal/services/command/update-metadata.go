package command

import (
	"context"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	metadataOpID := entityName + "-" + entityID
	op := uc.Telemetry.NewEntityOperation("metadata", "update", metadataOpID)

	op.WithAttributes(
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to get metadata on mongodb", err)
			logger.Errorf("Error get metadata on mongodb: %v", err)
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
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		return nil, err
	}

	op.End(ctx, "success")

	return metadataToUpdate, nil
}
