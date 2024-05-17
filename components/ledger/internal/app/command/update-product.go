package command

import (
	"context"
	"errors"
	"fmt"
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

	if upi.Name == "" && upi.Status.IsEmpty() && upi.Metadata == nil {
		return nil, common.UnprocessableOperationError{
			Message: "at least one of the allowed fields must be sent with a valid value [name, status.code, status.description, metadata]",
			Code:    "0006",
			Err:     nil,
		}
	}

	product := &r.Product{
		Name:   upi.Name,
		Status: upi.Status,
	}

	productUpdated, err := uc.ProductRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id), product)
	if err != nil {
		logger.Errorf("Error updating product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(r.Product{}).Name(),
				Message:    fmt.Sprintf("Product with ledger id %s and product id %s was not found", ledgerID, id),
				Code:       "PRODUCT_NOT_FOUND",
				Err:        err,
			}
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
	} else {
		if err := uc.MetadataRepo.Delete(ctx, reflect.TypeOf(r.Product{}).Name(), id); err != nil {
			return nil, err
		}
	}

	return productUpdated, nil
}
