package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services"
	"github.com/google/uuid"
)

// DeleteProductByID delete a product from the repository by ids.
func (uc *UseCase) DeleteProductByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_product_by_id")
	defer span.End()

	logger.Infof("Remove product for id: %s", id.String())

	if err := uc.ProductRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete product on repo by id", err)

		logger.Errorf("Error deleting product on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrProductIDNotFound, reflect.TypeOf(mmodel.Product{}).Name())
		}

		return err
	}

	return nil
}
