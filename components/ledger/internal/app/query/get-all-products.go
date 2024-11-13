package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	"github.com/google/uuid"
)

// GetAllProducts fetch all Product from the repository
func (uc *UseCase) GetAllProducts(ctx context.Context, organizationID, ledgerID uuid.UUID, filter commonHTTP.QueryHeader) ([]*mmodel.Product, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_products")
	defer span.End()

	logger.Infof("Retrieving products")

	products, err := uc.ProductRepo.FindAll(ctx, organizationID, ledgerID, filter.Limit, filter.Page)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get products on repo", err)

		logger.Errorf("Error getting products on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(mmodel.Product{}).Name())
		}

		return nil, err
	}

	if products != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Product{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(mmodel.Product{}).Name())
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
