package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	"time"
)

func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	ctx, span := tracer.Start(ctx, "command.create_metadata")
	defer span.End()

	if metadata != nil {
		meta := m.Metadata{
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
