package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

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

		span.SetAttributes(
			attribute.String("app.request.request_id", reqId),
		)

		err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", metadata)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
		}

		if err := uc.MetadataRepo.Create(ctx, entityName, &meta); err != nil {
			logger.Errorf("Error into creating %s metadata: %v", entityName, err)
			return nil, err
		}

		return metadata, nil
	}

	return nil, nil
}
