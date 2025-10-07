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

// GetAllAssets retrieves a paginated list of assets with metadata.
//
// Fetches assets from PostgreSQL with pagination, then enriches with MongoDB metadata.
// Returns empty array if no assets found (not an error). Excludes soft-deleted assets.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (pagination, sorting, date range)
//
// Returns:
//   - []*mmodel.Asset: Array of assets with metadata
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_assets"
func (uc *UseCase) GetAllAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_assets")
	defer span.End()

	logger.Infof("Retrieving assets")

	assets, err := uc.AssetRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting assets on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Warn("No assets found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get assets on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get assets on repo", err)

		return nil, err
	}

	if len(assets) == 0 {
		return assets, nil
	}

	assetIDs := make([]string, len(assets))
	for i, a := range assets {
		assetIDs[i] = a.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), assetIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		return nil, err
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

	return assets, nil
}
