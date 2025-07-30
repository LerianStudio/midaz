package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"reflect"
)

// UpdateAssetByID update an asset from the repository by given id.
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.asset_id", id.String()),
	)

	if err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", uii); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Trying to update asset: %v", uii)

	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update asset on repo by id", err)

		logger.Errorf("Error updating asset on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}
