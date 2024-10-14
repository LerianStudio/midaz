package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// GetAllProducts fetch all Product from the repository
func (uc *UseCase) GetAllProducts(ctx context.Context, organizationID, ledgerID string, filter commonHTTP.QueryHeader) ([]*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving products")

	products, err := uc.ProductRepo.FindAll(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting products on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	if products != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(r.Product{}).Name(), filter)
		if err != nil {
			return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(r.Product{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range products {
			if data, ok := metadataMap[products[i].ID]; ok {
				products[i].Metadata = data
			}
		}
	}

	return products, nil
}
