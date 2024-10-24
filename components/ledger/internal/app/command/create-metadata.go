package command

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	"time"
)

func (uc *UseCase) CreateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create metadata for %s: %v", entityName, entityID)

	if metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			return nil, common.ValidateBusinessError(err, entityName)
		}

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
