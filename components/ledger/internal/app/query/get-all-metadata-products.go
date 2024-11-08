package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// GetAllMetadataProducts fetch all Products from the repository
func (uc *UseCase) GetAllMetadataProducts(ctx context.Context, organizationID, ledgerID uuid.UUID, filter commonHTTP.QueryHeader) ([]*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_products")
	defer span.End()

	logger.Infof("Retrieving products")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(r.Product{}).Name(), filter)
	if err != nil || metadata == nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo by query params", err)

		return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(r.Product{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	products, err := uc.ProductRepo.FindByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get products on repo by query params", err)

		logger.Errorf("Error getting products on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoProductsFound, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	for i := range products {
		if data, ok := metadataMap[products[i].ID]; ok {
			products[i].Metadata = data
		}
	}

	return products, nil
}
