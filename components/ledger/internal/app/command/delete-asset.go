package command

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

// DeleteAssetByID delete an asset from the repository by ids.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove asset for id: %s", id)

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Error deleting asset on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(s.Asset{}).Name(), id)
		}

		return err
	}

	return nil
}
