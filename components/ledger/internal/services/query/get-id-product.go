package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// GetProductByID get a Product from the repository by given id.
func (uc *UseCase) GetProductByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Product, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_product_by_id")
	defer span.End()

	logger.Infof("Retrieving product for id: %s", id.String())

	product, err := uc.ProductRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get product on repo by id", err)

		logger.Errorf("Error getting product on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrProductIDNotFound, reflect.TypeOf(mmodel.Product{}).Name())
		}

		return nil, err
	}

	if product != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Product{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb product", err)

			logger.Errorf("Error get metadata on mongodb product: %v", err)

			return nil, err
		}

		if metadata != nil {
			product.Metadata = metadata.Data
		}
	}

	return product, nil
}
