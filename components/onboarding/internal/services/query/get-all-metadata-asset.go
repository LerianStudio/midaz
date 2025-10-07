// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataAssets retrieves assets filtered by metadata criteria.
//
// Metadata-first query: Searches MongoDB for matching metadata, then fetches assets from PostgreSQL.
// Use case: Finding assets by custom metadata fields (e.g., metadata.category=cryptocurrency).
//
// Query flow: MongoDB → PostgreSQL (filter by metadata first)
// OpenTelemetry: Creates span "query.get_all_metadata_assets"
func (uc *UseCase) GetAllMetadataAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_assets")
	defer span.End()

	logger.Infof("Retrieving assets")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
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

		return nil, err
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	return assets, nil
}
