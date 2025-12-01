// Package command provides CQRS command handlers for the transaction component.
package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
)

// CreateOrUpdateAssetRate creates a new asset rate or updates an existing one.
//
// Asset rates define the exchange rate between two assets (currencies).
// This function implements an upsert pattern: if a rate for the currency pair
// already exists, it updates the existing rate; otherwise, it creates a new one.
//
// # Currency Pair Uniqueness
//
// Asset rates are unique by the combination of:
//   - Organization ID (tenant isolation)
//   - Ledger ID (ledger context)
//   - From asset code (source currency)
//   - To asset code (target currency)
//
// This ensures each currency pair has exactly one active rate per ledger.
//
// # Rate Calculation
//
// The rate represents: 1 unit of "From" currency = Rate units of "To" currency
//
// Example: USD -> EUR with rate 0.85 means 1 USD = 0.85 EUR
//
// Scale is used for precision in calculations (number of decimal places).
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_or_update_asset_rate"
//  3. Validate "From" asset code format
//  4. Validate "To" asset code format
//  5. Check for existing rate by currency pair
//  6. If exists: Update the existing rate and metadata
//  7. If not exists: Create new rate with generated UUID
//  8. Create/update associated metadata in MongoDB
//  9. Return the created/updated asset rate
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger where this rate applies
//   - cari: The asset rate input containing From, To, Rate, Scale, Source, TTL, and Metadata
//
// # Returns
//
//   - *assetrate.AssetRate: The created or updated asset rate
//   - error: If validation fails or database operations fail
//
// # Input Fields
//
//   - From: Source asset code (validated for format)
//   - To: Target asset code (validated for format)
//   - Rate: The exchange rate value
//   - Scale: Decimal precision for the rate
//   - Source: Origin of the rate data (e.g., "manual", "external_api")
//   - TTL: Time-to-live in seconds (when rate expires)
//   - ExternalID: Optional external system identifier
//   - Metadata: Optional key-value pairs for additional data
//
// # Error Scenarios
//
//   - Invalid "From" asset code format
//   - Invalid "To" asset code format
//   - Database query failure (FindByCurrencyPair)
//   - Database create/update failure
//   - Metadata create/update failure (MongoDB)
//
// # Observability
//
// Creates tracing span "command.create_or_update_asset_rate" with error events.
// Logs operation progress and any errors encountered.
//
// # Example
//
//	input := &assetrate.CreateAssetRateInput{
//	    From:   "USD",
//	    To:     "EUR",
//	    Rate:   85,      // 0.85 with scale 2
//	    Scale:  2,
//	    Source: "manual",
//	    TTL:    &ttl,
//	}
//	rate, err := uc.CreateOrUpdateAssetRate(ctx, orgID, ledgerID, input)
func (uc *UseCase) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_or_update_asset_rate")
	defer span.End()

	logger.Infof("Initializing the create or update asset rate operation: %v", cari)

	if err := libCommons.ValidateCode(cari.From); err != nil {
		err := pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'from' asset code", err)

		return nil, err
	}

	if err := libCommons.ValidateCode(cari.To); err != nil {
		err := pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate 'to' asset code", err)

		return nil, err
	}

	externalID := cari.ExternalID
	emptyExternalID := libCommons.IsNilOrEmpty(externalID)

	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	logger.Infof("Trying to find existing asset rate by currency pair: %v", cari)

	arFound, err := uc.AssetRateRepo.FindByCurrencyPair(ctx, organizationID, ledgerID, cari.From, cari.To)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset rate by currency pair", err)

		logger.Errorf("Error creating asset rate: %v", err)

		return nil, err
	}

	if arFound != nil {
		logger.Infof("Trying to update asset rate: %v", cari)

		arFound.Rate = rate
		arFound.Scale = &scale
		arFound.Source = cari.Source
		arFound.TTL = *cari.TTL
		arFound.UpdatedAt = time.Now()

		if !emptyExternalID {
			arFound.ExternalID = *externalID
		}

		arFound, err = uc.AssetRateRepo.Update(ctx, organizationID, ledgerID, uuid.MustParse(arFound.ID), arFound)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset rate", err)

			logger.Errorf("Error updating asset rate: %v", err)

			return nil, err
		}

		metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), arFound.ID, cari.Metadata)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

			return nil, err
		}

		arFound.Metadata = metadataUpdated

		return arFound, nil
	}

	if emptyExternalID {
		idStr := libCommons.GenerateUUIDv7().String()
		externalID = &idStr
	}

	assetRateDB := &assetrate.AssetRate{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     *externalID,
		From:           cari.From,
		To:             cari.To,
		Rate:           rate,
		Scale:          &scale,
		Source:         cari.Source,
		TTL:            *cari.TTL,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	logger.Infof("Trying to create asset rate: %v", cari)

	assetRate, err := uc.AssetRateRepo.Create(ctx, assetRateDB)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset rate on repository", err)

		logger.Errorf("Error creating asset rate: %v", err)

		return nil, err
	}

	if cari.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   assetRate.ID,
			EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
			Data:       cari.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset rate metadata", err)

			logger.Errorf("Error into creating asset rate metadata: %v", err)

			return nil, err
		}

		assetRate.Metadata = cari.Metadata
	}

	return assetRate, nil
}
