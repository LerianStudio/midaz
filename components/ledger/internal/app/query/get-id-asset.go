package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// GetAssetByID get an Asset from the repository by given id.
func (uc *UseCase) GetAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_by_id")
	defer span.End()

	logger.Infof("Retrieving asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get asset on repo by id", err)

		logger.Errorf("Error getting asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(s.Asset{}).Name(), id)
		}

		return nil, err
	}

	if asset != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(s.Asset{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset", err)

			logger.Errorf("Error get metadata on mongodb asset: %v", err)

			return nil, err
		}

		if metadata != nil {
			asset.Metadata = metadata.Data
		}
	}

	return asset, nil
}
