// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving an asset rate by its external ID.
package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
)

// GetAssetRateByExternalID retrieves an asset rate by its external ID, enriched with metadata.
//
// This use case is designed for integrations with external systems, allowing them
// to look up an asset rate using their own identifier.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - externalID: The external system's ID for the asset rate.
//
// Returns:
//   - *assetrate.AssetRate: The asset rate with its metadata.
//   - error: An error if the asset rate is not found or if the retrieval fails.
func (uc *UseCase) GetAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*assetrate.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_external_id")
	defer span.End()

	logger.Infof("Trying to get asset rate by external id: %s", externalID.String())

	assetRate, err := uc.AssetRateRepo.FindByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get asset rate by external id on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, err
	}

	if assetRate != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), assetRate.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, err
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
