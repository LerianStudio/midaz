package command

import (
	"context"
	"errors"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// UpdateProductByID update a product from the repository by given id.
func (uc *UseCase) UpdateProductByID(ctx context.Context, organizationID, ledgerID, id string, upi *r.UpdateProductInput) (*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update product: %v", upi)

	product := &r.Product{
		Name:   upi.Name,
		Status: upi.Status,
	}

	productUpdated, err := uc.ProductRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id), product)
	if err != nil {
		logger.Errorf("Error updating product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, c.ValidateBusinessError(c.ProductIDNotFoundBusinessError, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	if len(upi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, upi.Metadata); err != nil {
			return nil, err
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(r.Product{}).Name(), id, upi.Metadata); err != nil {
			return nil, err
		}

		productUpdated.Metadata = upi.Metadata
	}

	return productUpdated, nil
}
