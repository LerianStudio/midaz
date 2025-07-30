package query

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"reflect"
)

// GetAllAssetRatesByAssetCode returns all asset rates by asset codes.
func (uc *UseCase) GetAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, filter http.QueryHeader) ([]*assetrate.AssetRate, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_asset_codes")
	defer span.End()

	logger.Infof("Trying to get asset rate by source asset code: %s and target asset codes: %v", fromAssetCode, filter.ToAssetCodes)

	if err := libCommons.ValidateCode(fromAssetCode); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate 'from' asset code", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	for _, toAssetCode := range filter.ToAssetCodes {
		if err := libCommons.ValidateCode(toAssetCode); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to validate 'to' asset codes", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
		}
	}

	assetRates, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(ctx, organizationID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get asset rate by asset codes on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if assetRates != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, libHTTP.CursorPagination{}, err
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

	return assetRates, cur, nil
}
