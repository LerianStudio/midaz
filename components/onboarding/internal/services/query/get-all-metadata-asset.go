package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataAssets fetch all Assets from the repository
func (uc *UseCase) GetAllMetadataAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_assets")
	defer span.End()

	logger.Infof("Retrieving assets")

	metadataFilter := filter
	metadataFilter.UseMetadata = false

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), metadataFilter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
		assert.That(assert.ValidUUID(meta.EntityID),
			"metadata entity ID must be valid UUID",
			"value", meta.EntityID,
			"index", idx)
		uuids[idx] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	assets, err := uc.AssetRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		logger.Errorf("Error getting assets on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get assets on repo", err)

			logger.Warn("No assets found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get assets on repo", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	assets = paginateSlice(assets, filter.Page, filter.Limit)

	return assets, nil
}
