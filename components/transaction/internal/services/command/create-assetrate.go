package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// CreateOrUpdateAssetRate creates or updates an asset rate.
func (uc *UseCase) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a rate operation telemetry entity
	op := uc.Telemetry.NewAssetRateOperation("upsert", cari.From+"-"+cari.To)

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("from_asset", cari.From),
		attribute.String("to_asset", cari.To),
		attribute.String("source", *cari.Source),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Initializing the create or update asset rate operation: %v", cari)

	if err := pkg.ValidateCode(cari.From); err != nil {
		// Record validation error
		op.RecordError(ctx, "validation_error", err)
		op.WithAttribute("error_detail", "invalid_from_code")
		op.End(ctx, "failed")

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	if err := pkg.ValidateCode(cari.To); err != nil {
		// Record validation error
		op.RecordError(ctx, "validation_error", err)
		op.WithAttributes(
			attribute.String("error_detail", "invalid_to_code"),
			attribute.String("to_asset", cari.To),
		)
		op.End(ctx, "failed")

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
	}

	externalID := cari.ExternalID
	emptyExternalID := pkg.IsNilOrEmpty(externalID)

	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	logger.Infof("Trying to find existing asset rate by currency pair: %v", cari)

	arFound, err := uc.AssetRateRepo.FindByCurrencyPair(ctx, organizationID, ledgerID, cari.From, cari.To)
	if err != nil {
		// Record error
		op.RecordError(ctx, "find_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Error creating asset rate: %v", err)

		return nil, err
	}

	if arFound != nil {
		// Create update operation - track as a nested operation
		updateOp := uc.Telemetry.NewAssetRateOperation("update", arFound.ID)
		updateOp.WithAttributes(
			attribute.String("from_asset", cari.From),
			attribute.String("to_asset", cari.To),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
		)

		// Start tracing for update operation
		updateCtx := updateOp.StartTrace(ctx)
		updateOp.RecordSystemicMetric(updateCtx)

		logger.Infof("Trying to update asset rate: %v", cari)

		arFound.Rate = rate
		arFound.Scale = &scale
		arFound.Source = cari.Source
		arFound.TTL = *cari.TTL
		arFound.UpdatedAt = time.Now()

		if !emptyExternalID {
			arFound.ExternalID = *externalID
		}

		arFound, err = uc.AssetRateRepo.Update(updateCtx, organizationID, ledgerID, uuid.MustParse(arFound.ID), arFound)
		if err != nil {
			// Record error in update operation
			updateOp.RecordError(updateCtx, "update_error", err)
			updateOp.End(updateCtx, "failed")

			// Also record in parent operation
			op.RecordError(ctx, "update_error", err)
			op.End(ctx, "failed")

			logger.Errorf("Error updating asset rate: %v", err)

			return nil, err
		}

		if cari.Metadata != nil {
			if err := pkg.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
				// Record metadata validation error
				updateOp.RecordError(updateCtx, "metadata_validation_error", err)
				updateOp.End(updateCtx, "failed")

				// Also record in parent operation
				op.RecordError(ctx, "metadata_validation_error", err)
				op.End(ctx, "failed")

				return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
			}

			meta := mongodb.Metadata{
				EntityID:   arFound.ID,
				EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
				Data:       cari.Metadata,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			if err := uc.MetadataRepo.Create(updateCtx, reflect.TypeOf(assetrate.AssetRate{}).Name(), &meta); err != nil {
				// Record metadata creation error
				updateOp.RecordError(updateCtx, "metadata_creation_error", err)
				updateOp.End(updateCtx, "partial_success")

				logger.Errorf("Error into creating asset rate metadata: %v", err)

				return nil, err
			}

			arFound.Metadata = cari.Metadata
		}

		// Record business metrics - rate value
		updateOp.RecordBusinessMetric(updateCtx, "rate", arFound.Rate)

		// Mark update as successful
		updateOp.End(updateCtx, "success")

		// Record business metrics in main operation too
		op.RecordBusinessMetric(ctx, "rate", arFound.Rate)

		// Mark main operation as successful
		op.End(ctx, "success")

		return arFound, nil
	}

	// This is a create operation instead of update
	createOp := uc.Telemetry.NewAssetRateOperation("create", cari.From+"-"+cari.To)
	createOp.WithAttributes(
		attribute.String("from_asset", cari.From),
		attribute.String("to_asset", cari.To),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for create operation
	createCtx := createOp.StartTrace(ctx)
	createOp.RecordSystemicMetric(createCtx)

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

	// Add asset rate ID to telemetry
	createOp.WithAttribute("assetrate_id", assetRateDB.ID)

	logger.Infof("Trying to create asset rate: %v", cari)

	assetRate, err := uc.AssetRateRepo.Create(createCtx, assetRateDB)
	if err != nil {
		// Record creation error
		createOp.RecordError(createCtx, "creation_error", err)
		createOp.End(createCtx, "failed")

		// Also record in parent operation
		op.RecordError(ctx, "creation_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Error creating asset rate: %v", err)

		return nil, err
	}

	if cari.Metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
			// Record metadata validation error
			createOp.RecordError(createCtx, "metadata_validation_error", err)
			createOp.End(createCtx, "failed")

			// Also record in parent operation
			op.RecordError(ctx, "metadata_validation_error", err)
			op.End(ctx, "failed")

			return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(assetrate.AssetRate{}).Name())
		}

		meta := mongodb.Metadata{
			EntityID:   assetRate.ID,
			EntityName: reflect.TypeOf(assetrate.AssetRate{}).Name(),
			Data:       cari.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(createCtx, reflect.TypeOf(assetrate.AssetRate{}).Name(), &meta); err != nil {
			// Record metadata creation error
			createOp.RecordError(createCtx, "metadata_creation_error", err)
			createOp.End(createCtx, "partial_success")

			logger.Errorf("Error into creating asset rate metadata: %v", err)

			return nil, err
		}

		assetRate.Metadata = cari.Metadata
	}

	// Record business metrics - rate value
	createOp.RecordBusinessMetric(createCtx, "rate", assetRate.Rate)

	// Mark creation as successful
	createOp.End(createCtx, "success")

	// Record business metrics in main operation too
	op.RecordBusinessMetric(ctx, "rate", assetRate.Rate)

	// Mark main operation as successful
	op.End(ctx, "success")

	return assetRate, nil
}
