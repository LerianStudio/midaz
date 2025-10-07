// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllAssetRatesByAssetCode retrieves asset rates for currency conversions with metadata.
//
// Fetches asset rates from PostgreSQL for a source asset to multiple target assets,
// then enriches with MongoDB metadata. Used for multi-currency transaction processing.
//
// The method:
// 1. Validates source asset code (alphanumeric, uppercase)
// 2. Validates all target asset codes
// 3. Fetches asset rates with cursor pagination
// 4. Fetches metadata for all rates
// 5. Merges metadata into rate objects
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - fromAssetCode: Source asset code (e.g., "USD")
//   - filter: Query parameters with ToAssetCodes array (e.g., ["EUR", "GBP"])
//
// Returns:
//   - []*assetrate.AssetRate: Array of asset rates with metadata
//   - libHTTP.CursorPagination: Pagination cursor info
//   - error: Business error if validation or query fails
//
// OpenTelemetry: Creates span "query.get_asset_rate_by_asset_codes"
func (uc *UseCase) GetAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, filter http.QueryHeader) ([]*assetrate.AssetRate, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_asset_codes")
	defer span.End()

	logger.Infof("Trying to get asset rate by source asset code: %s and target asset codes: %v", fromAssetCode, filter.ToAssetCodes)

	if err := libCommons.ValidateCode(fromAssetCode); err != nil {
		err := pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'from' asset code", err)

		logger.Warnf("Error validating 'from' asset code: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	for _, toAssetCode := range filter.ToAssetCodes {
		if err := libCommons.ValidateCode(toAssetCode); err != nil {
			err := pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'to' asset codes", err)

			logger.Warnf("Error validating 'to' asset codes: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	assetRates, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(ctx, organizationID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset rate by asset codes on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if assetRates != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb asset rate", err)

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
