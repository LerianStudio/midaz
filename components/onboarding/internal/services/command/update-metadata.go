package command

import (
	"context"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
)

func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb", err)

			logger.Errorf("Error get metadata on mongodb: %v", err)

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

		return nil, err
	}

	return metadataToUpdate, nil
}
