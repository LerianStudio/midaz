package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// UpdateAssetByID update an asset from the repository by given id.
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *s.UpdateAssetInput) (*s.Asset, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	logger.Infof("Trying to update asset: %v", uii)

	asset := &s.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update asset on repo by id", err)

		logger.Errorf("Error updating asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(s.Asset{}).Name(), id)
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(s.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}
