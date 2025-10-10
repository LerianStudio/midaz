// Package query implements read operations (queries) for the onboarding service.
// This file contains the query for retrieving an asset by its ID.
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
	"github.com/google/uuid"
)

// GetAssetByID retrieves a single asset by its ID, enriched with metadata.
//
// This use case fetches an asset from the PostgreSQL database and its corresponding
// metadata from MongoDB, then merges them into a single response.
// Soft-deleted assets are excluded from the result.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the asset to retrieve.
//
// Returns:
//   - *mmodel.Asset: The asset with its metadata, or nil if not found.
//   - error: An error if the asset is not found or if the query fails.
func (uc *UseCase) GetAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_by_id")
	defer span.End()

	logger.Infof("Retrieving asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting asset on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset on repo by id", err)

			logger.Warn("No asset found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset on repo by id", err)

		return nil, err
	}

	if asset != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String())
		if err != nil {
			// FIXME: This error handling is incorrect. It returns an ErrAssetIDNotFound, but the error
			// is related to fetching metadata, not the asset itself. The function should either
			// return the asset without metadata or a more appropriate metadata-specific error.
			err := pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb asset", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			asset.Metadata = metadata.Data
		}
	}

	return asset, nil
}
