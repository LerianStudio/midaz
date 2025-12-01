package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
)

// GetAssetRateByExternalID retrieves an asset rate by its external identifier.
//
// Asset rates define currency exchange rates used for multi-currency transactions.
// The external ID allows integration with external rate providers (e.g., forex APIs)
// by mapping their identifiers to internal asset rate records.
//
// Use Cases:
//   - Currency conversion during cross-currency transactions
//   - Rate synchronization with external forex providers
//   - Audit trail for rate lookups during transaction processing
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//	  - Log external ID being queried for debugging
//
//	Step 2: Fetch Asset Rate from PostgreSQL
//	  - Query by external ID within organization/ledger scope
//	  - External ID is unique within the ledger context
//	  - Return error if not found or database error
//
//	Step 3: Fetch Metadata from MongoDB (if rate found)
//	  - Query metadata document by asset rate ID
//	  - Metadata may contain provider-specific attributes
//	  - Assign metadata to rate if present
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope the rate lookup
//   - externalID: External system identifier for the rate
//
// Returns:
//   - *assetrate.AssetRate: Rate with metadata, nil if not found
//   - error: Database or metadata lookup error
//
// Error Scenarios:
//   - Repository error: Asset rate not found or database unavailable
//   - Metadata error: MongoDB query failed (rate still returned as nil)
//
// External ID Format:
//
// External IDs are typically provider-specific:
//   - Forex API: "fx_usd_brl_2024010112" (rate ID with timestamp)
//   - Manual entry: "manual_rate_001"
//   - Import batch: "import_batch_123_rate_45"
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
