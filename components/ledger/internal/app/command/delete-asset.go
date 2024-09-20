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

// DeleteAssetByID delete an asset from the repository by ids.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove asset for id: %s", id)

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Error deleting asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.EntityNotFoundError{
				EntityType: reflect.TypeOf(s.Asset{}).Name(),
				Message:    fmt.Sprintf("Asset with id %s was not found", id),
				Code:       "ASSET_NOT_FOUND",
				Err:        err,
			}
		}

		return err
	}

	return nil
}
