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
	"github.com/google/uuid"
)

// GetAssetByID retrieves a single asset by ID with metadata.
//
// This method implements the get asset query use case, which:
// 1. Fetches the asset from PostgreSQL by ID
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the asset object
// 4. Returns the enriched asset
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the asset to retrieve
//
// Returns:
//   - *mmodel.Asset: Asset with metadata
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrAssetIDNotFound: Asset doesn't exist or is deleted
//
// OpenTelemetry:
//   - Creates span "query.get_asset_by_id"
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
