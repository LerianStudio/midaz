package query

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

// GetProductByID get a Product from the repository by given id.
func (uc *UseCase) GetProductByID(ctx context.Context, organizationID, ledgerID, id string) (*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving product for id: %s", id)

	product, err := uc.ProductRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting product on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(r.Product{}).Name(),
				Message:    fmt.Sprintf("Product with id %s was not found", id),
				Code:       "PRODUCT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if product != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(r.Product{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb product: %v", err)
			return nil, err
		}

		if metadata != nil {
			product.Metadata = metadata.Data
		}
	}

	return product, nil
}
