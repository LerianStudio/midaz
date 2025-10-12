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

// CreateOrUpdateAssetRate creates or updates an exchange rate between two assets.
//
// Asset rates enable multi-currency/multi-asset transactions by defining conversion
// rates between asset pairs (e.g., USD to EUR, BTC to USD). If a rate already exists
// for the currency pair, it updates the existing rate; otherwise, creates a new one.
//
// Validation:
// - Both asset codes must be valid (uppercase, alphanumeric)
// - Automatically generates externalID if not provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the rate
//   - ledgerID: Ledger UUID containing the rate
//   - cari: Asset rate input with from/to codes, rate, scale, source, TTL
//
// Returns:
//   - *assetrate.AssetRate: The created or updated asset rate
//   - error: Validation or persistence errors
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
		if err := libCommons.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
			err := pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate metadata", err)

			return nil, err
		}

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
