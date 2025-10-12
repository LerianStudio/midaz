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
// This function performs a partial update of asset properties. Only name and status
// can be updated; the asset type and code are immutable after creation to maintain
// referential integrity across accounts and transactions.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the asset
//   - ledgerID: The UUID of the ledger containing the asset
//   - id: The UUID of the asset to update
//   - uii: The update input containing fields to modify (name, status, metadata)
//
// Returns:
//   - *mmodel.Asset: The updated asset with refreshed metadata
//   - error: ErrAssetIDNotFound if not found, or repository errors
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	logger.Infof("Trying to update asset: %v", uii)

	// Construct partial update entity (only name and status can be updated)
	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	// Persist the update to PostgreSQL
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

	// Update metadata in MongoDB using JSON Merge Patch semantics
	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), id.String(), uii.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}
