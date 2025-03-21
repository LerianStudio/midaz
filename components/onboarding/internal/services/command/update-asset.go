package command

import (
	"context"
	"errors"
	"reflect"

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

	op := uc.Telemetry.NewAssetOperation("update", id.String())

	op.WithAttributes(
		attribute.String("asset_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to update asset: %v", uii)

	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update asset on repo by id", err)
		logger.Errorf("Error updating asset on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		return nil, err
	}

	op.WithAttribute("asset_name", assetUpdated.Name)
	op.WithAttribute("asset_type", assetUpdated.Type)

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata on repo by id", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_metadata_error", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	op.End(ctx, "success")

	return assetUpdated, nil
}
