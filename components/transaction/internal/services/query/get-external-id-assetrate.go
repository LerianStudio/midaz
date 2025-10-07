// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
)

// GetAssetRateByExternalID retrieves an asset rate by external ID with metadata.
//
// Fetches asset rate from PostgreSQL using external ID (for integration with external systems),
// then enriches with MongoDB metadata.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - externalID: External system's ID for the asset rate
//
// Returns:
//   - *assetrate.AssetRate: Asset rate with metadata
//   - error: Error if not found or query fails
//
// OpenTelemetry: Creates span "query.get_asset_rate_by_external_id"
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
