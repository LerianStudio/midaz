package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(s.Asset{}).Name(),
				Message:    fmt.Sprintf("Asset with ledger id %s and asset id %s was not found", ledgerID, id),
				Code:       "ASSET_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if len(uii.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uii.Metadata); err != nil {
			return nil, err
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(s.Asset{}).Name(), id.String(), uii.Metadata); err != nil {
			return nil, err
		}

		assetUpdated.Metadata = uii.Metadata
	}

	return assetUpdated, nil
}
