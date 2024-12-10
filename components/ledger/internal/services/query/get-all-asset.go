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
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

// GetAllAssets fetch all Asset from the repository
func (uc *UseCase) GetAllAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_assets")
	defer span.End()

	logger.Infof("Retrieving assets")

	assets, err := uc.AssetRepo.FindAllWithDeleted(ctx, organizationID, ledgerID, filter.ToPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get assets on repo", err)

		logger.Errorf("Error getting assets on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return nil, err
	}

	if assets != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for idx := range assets {
			if data, ok := metadataMap[assets[idx].ID]; ok {
				assets[idx].Metadata = data
			}
		}
	}

	return assets, nil
}
