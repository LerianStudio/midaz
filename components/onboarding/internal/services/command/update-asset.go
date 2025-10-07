// Package command implements write operations (commands) for the onboarding service.
// This file contains the UpdateAssetByID command implementation.
package command

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

// UpdateAssetByID updates an existing asset in the repository.
//
// This method implements the update asset use case, which:
// 1. Updates the asset in PostgreSQL
// 2. Updates associated metadata in MongoDB using merge semantics
// 3. Returns the updated asset with merged metadata
//
// Business Rules:
//   - Only provided fields are updated (partial updates supported)
//   - Asset code cannot be changed (immutable, not in update input)
//   - Asset type cannot be changed (immutable, not in update input)
//   - Name can be updated
//   - Status can be updated
//   - Metadata is merged with existing
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Empty status means "don't update status"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (assets table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the asset to update
//   - uii: Update asset input with fields to update
//
// Returns:
//   - *mmodel.Asset: Updated asset with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrAssetIDNotFound: Asset doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateAssetInput{
//	    Name:   "US Dollar - Updated",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	}
//	asset, err := useCase.UpdateAssetByID(ctx, orgID, ledgerID, assetID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_asset_by_id"
//   - Records errors as span events
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	logger.Infof("Trying to update asset: %v", uii)

	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		logger.Errorf("Error updating asset on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Warnf("Asset ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}
