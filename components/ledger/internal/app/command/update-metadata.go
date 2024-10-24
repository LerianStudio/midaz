package command

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
)

func (uc *UseCase) UpdateMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update metadata for %s: %v", entityName, entityID)

	if len(metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			return nil, common.ValidateBusinessError(err, entityName)
		}

		if err := uc.MetadataRepo.Update(ctx, entityName, entityID, metadata); err != nil {
			logger.Errorf("Error into updating %s metadata: %v", entityName, err)
			return nil, err
		}

	}

	return metadata, nil
}
