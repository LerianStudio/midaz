package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// GetAssetByID get an Asset from the repository by given id.
func (uc *UseCase) GetAssetByID(ctx context.Context, organizationID, ledgerID, id string) (*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(s.Asset{}).Name(), id)
		}

		return nil, err
	}

	if asset != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(s.Asset{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb asset: %v", err)
			return nil, err
		}

		if metadata != nil {
			asset.Metadata = metadata.Data
		}
	}

	return asset, nil
}
