package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/google/uuid"
)

// UpdateProductByID update a product from the repository by given id.
func (uc *UseCase) UpdateProductByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateProductInput) (*mmodel.Product, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_product_by_id")
	defer span.End()

	logger.Infof("Trying to update product: %v", upi)

	product := &mmodel.Product{
		Name:   upi.Name,
		Status: upi.Status,
	}

	productUpdated, err := uc.ProductRepo.Update(ctx, organizationID, ledgerID, id, product)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update product on repo by id", err)

		logger.Errorf("Error updating product on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(constant.ErrProductIDNotFound, reflect.TypeOf(mmodel.Product{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Product{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	productUpdated.Metadata = metadataUpdated

	return productUpdated, nil
}
