package command

import (
	"context"
	"errors"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// DeleteProductByID delete a product from the repository by ids.
func (uc *UseCase) DeleteProductByID(ctx context.Context, organizationID, ledgerID, id string) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove product for id: %s", id)

	if err := uc.ProductRepo.Delete(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Error deleting product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return c.ValidateBusinessError(c.ProductIDNotFoundBusinessError, reflect.TypeOf(r.Product{}).Name())
		}

		return err
	}

	return nil
}
