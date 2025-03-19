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

	metadataOpID := entityName + "-" + entityID
	op := uc.Telemetry.NewEntityOperation("metadata", "create", metadataOpID)

	op.WithAttributes(
		attribute.String("entity_name", entityName),
		attribute.String("entity_id", entityID),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

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
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "creation_error", err)

			return nil, err
		}

		op.End(ctx, "success")

		return metadata, nil
	}

	op.End(ctx, "success")

	return nil, nil
}
