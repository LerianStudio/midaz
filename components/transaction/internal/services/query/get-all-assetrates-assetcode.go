// Package query provides use case implementations for transaction read operations.
// It contains query handlers for retrieving transactions, operations, balances,
// and related entities with filtering and pagination support.
package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// GetAllAssetRatesByAssetCode returns all asset rates by asset codes.
func (uc *UseCase) GetAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, filter http.QueryHeader) ([]*mmodel.AssetRate, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_asset_codes")
	defer span.End()

	logger.Infof("Trying to get asset rate by source asset code: %s and target asset codes: %v", fromAssetCode, filter.ToAssetCodes)

	if err := utils.ValidateCode(fromAssetCode); err != nil {
		businessErr := pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'from' asset code", businessErr)

		logger.Warnf("Error validating 'from' asset code: %v", businessErr)

		return nil, libHTTP.CursorPagination{}, businessErr
	}

	for _, toAssetCode := range filter.ToAssetCodes {
		if err := utils.ValidateCode(toAssetCode); err != nil {
			businessErr := pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'to' asset codes", businessErr)

			logger.Warnf("Error validating 'to' asset codes: %v", businessErr)

			return nil, libHTTP.CursorPagination{}, businessErr
		}
	}

	assetRates, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(ctx, organizationID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset rate by asset codes on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
	}

	if assetRates != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.AssetRate{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
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
