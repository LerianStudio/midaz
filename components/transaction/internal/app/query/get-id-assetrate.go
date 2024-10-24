package query

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/common/mlog"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	"github.com/google/uuid"
)

// GetAssetRateByID gets data in the repository.
func (uc *UseCase) GetAssetRateByID(ctx context.Context, organizationID, ledgerID, assetRateID uuid.UUID) (*ar.AssetRate, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to get asset rate")

	assetRate, err := uc.AssetRateRepo.Find(ctx, organizationID, ledgerID, assetRateID)
	if err != nil {
		logger.Errorf("Error getting asset rate: %v", err)
		return nil, err
	}

	if assetRate != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(ar.AssetRate{}).Name(), assetRateID.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)
			return nil, err
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
