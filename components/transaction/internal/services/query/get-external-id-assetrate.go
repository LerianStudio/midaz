package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAssetRateByExternalID gets data in the repository.
func (uc *UseCase) GetAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*mmodel.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_external_id")
	defer span.End()

	logger.Infof("Trying to get asset rate by external id: %s", externalID.String())

	assetRate, err := uc.AssetRateRepo.FindByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get asset rate by external id on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	if assetRate != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.AssetRate{}).Name(), assetRate.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, pkg.ValidateInternalError(err, "AssetRate")
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
