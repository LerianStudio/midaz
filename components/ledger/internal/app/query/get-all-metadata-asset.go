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
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// GetAllMetadataAssets fetch all Assets from the repository
func (uc *UseCase) GetAllMetadataAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter commonHTTP.QueryHeader) ([]*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_assets")
	defer span.End()

	logger.Infof("Retrieving assets")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(s.Asset{}).Name(), filter)
	if err != nil || metadata == nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

		return nil, common.ValidateBusinessError(cn.ErrNoAssetsFound, reflect.TypeOf(s.Asset{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
		uuids[idx] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	assets, err := uc.AssetRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get assets on repo", err)

		logger.Errorf("Error getting assets on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoAssetsFound, reflect.TypeOf(s.Asset{}).Name())
		}

		return nil, err
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	return assets, nil
}
