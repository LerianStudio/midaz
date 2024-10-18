package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// DeleteProductByID delete a product from the repository by ids.
func (uc *UseCase) DeleteProductByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove product for id: %s", id.String())

	if err := uc.ProductRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Error deleting product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrProductIDNotFound, reflect.TypeOf(r.Product{}).Name())
		}

		return err
	}

	return nil
}
