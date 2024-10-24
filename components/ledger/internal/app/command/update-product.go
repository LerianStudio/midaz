package command

import (
	"context"
	"errors"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// UpdateProductByID update a product from the repository by given id.
func (uc *UseCase) UpdateProductByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *r.UpdateProductInput) (*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update product: %v", upi)

	product := &r.Product{
		Name:   upi.Name,
		Status: upi.Status,
	}

	productUpdated, err := uc.ProductRepo.Update(ctx, organizationID, ledgerID, id, product)
	if err != nil {
		logger.Errorf("Error updating product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrProductIDNotFound, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	if len(upi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, upi.Metadata); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(r.Product{}).Name())
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(r.Product{}).Name(), id.String(), upi.Metadata); err != nil {
			return nil, err
		}

		productUpdated.Metadata = upi.Metadata
	}

	return productUpdated, nil
}
