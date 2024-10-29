package command

import (
	"context"
	"errors"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// UpdateAssetByID update an asset from the repository by given id.
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *s.UpdateAssetInput) (*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update asset: %v", uii)

	asset := &s.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		logger.Errorf("Error updating asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(s.Asset{}).Name(), id)
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(s.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}
