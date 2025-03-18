package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateAssetByID update an asset from the repository by given id.
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "asset", "update",
		attribute.String("asset_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to update asset: %v", uii)

	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update asset on repo by id", err)

		logger.Errorf("Error updating asset on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "asset", "update_error",
			attribute.String("asset_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		// Record error
		uc.recordOnboardingError(ctx, "asset", "update_metadata_error",
			attribute.String("asset_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "asset", "update", "success",
		attribute.String("asset_id", id.String()),
		attribute.String("asset_name", assetUpdated.Name))

	return assetUpdated, nil
}
