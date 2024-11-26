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

// CreateAssetRate creates a new asset rate and persists data in the repository.
func (uc *UseCase) CreateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset_rate")
	defer span.End()

	logger.Infof("Trying to create asset rate: %v", cari)

	if err := pkg.ValidateCode(cari.BaseAssetCode); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate base asset code", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	if err := pkg.ValidateCode(cari.CounterAssetCode); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate counter asset code", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	assetRateDB := &assetrate.AssetRate{
		ID:               pkg.GenerateUUIDv7().String(),
		BaseAssetCode:    cari.BaseAssetCode,
		CounterAssetCode: cari.CounterAssetCode,
		Amount:           cari.Amount,
		Scale:            cari.Scale,
		Source:           cari.Source,
		OrganizationID:   organizationID.String(),
		LedgerID:         ledgerID.String(),
		CreatedAt:        time.Now(),
	}

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
