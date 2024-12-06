package query

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// GetAllAssetRatesByAssetCode returns all asset rates by asset codes.
func (uc *UseCase) GetAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, filter http.QueryHeader) ([]*assetrate.AssetRate, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_asset_codes")
	defer span.End()

	logger.Infof("Trying to get asset rate by source asset code: %s and target asset codes: %v", fromAssetCode, filter.ToAssetCodes)

	assetRates, err := uc.AssetRateRepo.FindAllByAssetCodes(ctx, organizationID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.Limit, filter.Page)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get asset rate by asset codes on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, err
	}

	if assetRates != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range assetRates {
			if data, ok := metadataMap[assetRates[i].ID]; ok {
				assetRates[i].Metadata = data
			}
		}
	}

	return assetRates, nil
}
