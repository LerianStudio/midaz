package query

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
)

// GetAssetRateByID gets data in the repository.
func (uc *UseCase) GetAssetRateByID(ctx context.Context, organizationID, ledgerID, assetRateID uuid.UUID) (*assetrate.AssetRate, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_id")
	defer span.End()

	logger.Infof("Trying to get asset rate")

	assetRate, err := uc.AssetRateRepo.Find(ctx, organizationID, ledgerID, assetRateID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get asset rate on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, err
	}

	if assetRate != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), assetRateID.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, err
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
