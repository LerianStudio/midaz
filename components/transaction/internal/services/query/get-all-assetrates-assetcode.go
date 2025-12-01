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

// GetAllAssetRatesByAssetCode retrieves asset exchange rates for currency conversion.
//
// This method queries all exchange rates from a source asset code to one or more
// target asset codes. Asset rates are used for multi-currency transactions where
// amounts need to be converted between different assets.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_asset_rate_by_asset_codes"
//
//	Step 2: Source Code Validation
//	  - Validate fromAssetCode format via libCommons.ValidateCode
//	  - If invalid: Return validation business error
//
//	Step 3: Target Codes Validation
//	  - Iterate through filter.ToAssetCodes
//	  - Validate each target code format
//	  - If any invalid: Return validation business error
//
//	Step 4: Asset Rates Retrieval
//	  - Query AssetRateRepo.FindAllByAssetCodes with pagination
//	  - If retrieval fails: Return error with span event
//	  - Returns asset rates with cursor pagination info
//
//	Step 5: Metadata Enrichment
//	  - If asset rates found: Query MongoDB for metadata list
//	  - Build metadata map keyed by entity ID
//	  - Attach metadata to each asset rate
//
//	Step 6: Response
//	  - Return enriched asset rates with pagination cursor
//
// Asset Rate Structure:
//
// Each asset rate represents a conversion factor from one asset to another:
//   - FromAssetCode: Source currency/asset (e.g., "USD")
//   - ToAssetCode: Target currency/asset (e.g., "EUR")
//   - Rate: Conversion multiplier (e.g., 0.92 for USD->EUR)
//   - EffectiveAt: Timestamp when rate became effective
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the asset rates
//   - fromAssetCode: Source asset code to convert from
//   - filter: Query parameters including ToAssetCodes and pagination
//
// Returns:
//   - []*assetrate.AssetRate: List of asset rates with metadata
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - Invalid fromAssetCode format
//   - Invalid toAssetCode format in filter
//   - Database connection failure
//   - MongoDB metadata retrieval failure
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
