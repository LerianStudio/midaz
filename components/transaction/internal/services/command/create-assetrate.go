package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// CreateOrUpdateAssetRate creates or updates an asset rate.
func (uc *UseCase) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_or_update_asset_rate")
	defer span.End()

	logger.Infof("Initializing the create or update asset rate operation: %v", cari)

	if err := pkg.ValidateCode(cari.From); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate 'from' asset code", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	if err := pkg.ValidateCode(cari.To); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate 'to' asset code", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	externalID := cari.ExternalID
	emptyExternalID := pkg.IsNilOrEmpty(externalID)

	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	logger.Infof("Trying to find existing asset rate by currency pair: %v", cari)

	arFound, err := uc.AssetRateRepo.FindByCurrencyPair(ctx, organizationID, ledgerID, cari.From, cari.To)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to find asset rate by currency pair", err)

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
			mopentelemetry.HandleSpanError(&span, "Failed to update asset rate", err)

			logger.Errorf("Error updating asset rate: %v", err)

			return nil, err
		}

		if cari.Metadata != nil {
			if err := pkg.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to validate metadata", err)

				return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
			}

			meta := mongodb.Metadata{
				EntityID:   arFound.ID,
				EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
				Data:       cari.Metadata,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), &meta); err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to create asset rate metadata", err)

				logger.Errorf("Error into creating asset rate metadata: %v", err)

				return nil, err
			}

			arFound.Metadata = cari.Metadata
		}

		return arFound, nil
	}

	if emptyExternalID {
		idStr := pkg.GenerateUUIDv7().String()
		externalID = &idStr
	}

	assetRateDB := &assetrate.AssetRate{
		ID:             pkg.GenerateUUIDv7().String(),
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
		mopentelemetry.HandleSpanError(&span, "Failed to create asset rate on repository", err)

		logger.Errorf("Error creating asset rate: %v", err)

		return nil, err
	}

	if cari.Metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to validate metadata", err)

			return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
		}

		meta := mongodb.Metadata{
			EntityID:   assetRate.ID,
			EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
			Data:       cari.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), &meta); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create asset rate metadata", err)

			logger.Errorf("Error into creating asset rate metadata: %v", err)

			return nil, err
		}

		assetRate.Metadata = cari.Metadata
	}

	return assetRate, nil
}
