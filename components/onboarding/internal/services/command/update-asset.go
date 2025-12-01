// Package command provides CQRS command handlers for the onboarding component.
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
// This function performs a partial update of an asset's mutable fields.
// Assets define currencies and instruments that can be held in accounts.
//
// # Updatable Fields
//
// The following fields can be updated:
//   - Name: Update asset display name
//   - Status: Change asset status
//   - Metadata: Update arbitrary key-value data
//
// Non-updatable fields (set at creation):
//   - ID, OrganizationID, LedgerID, CreatedAt
//   - Code: Asset code is immutable (e.g., "USD", "BRL")
//   - Type: Asset type is immutable (currency, commodity, etc.)
//   - Scale: Decimal precision is immutable
//
// # Code Immutability
//
// Asset codes cannot be changed because:
//   - Accounts reference assets by code
//   - Transaction history uses asset codes
//   - External systems may reference asset codes
//   - Changing codes would break referential integrity
//
// # Status Transitions
//
// Common status transitions:
//   - ACTIVE -> INACTIVE: Disable asset for new transactions
//   - INACTIVE -> ACTIVE: Reactivate asset
//
// Note: Existing balances remain valid regardless of status.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.update_asset_by_id"
//  3. Build partial asset update model
//  4. Update asset in PostgreSQL via repository
//  5. Handle not found error (ErrAssetIDNotFound)
//  6. Update associated metadata in MongoDB
//  7. Return updated asset with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this asset
//   - id: The UUID of the asset to update
//   - uii: UpdateAssetInput containing fields to update
//
// # Returns
//
//   - *mmodel.Asset: The updated asset
//   - error: If asset not found or database operations fail
//
// # Error Scenarios
//
//   - ErrAssetIDNotFound: Asset with given ID not found
//   - Database connection failure
//   - Metadata update failure (MongoDB)
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.update_asset_by_id" with error events.
// Logs operation progress, warnings for not found, errors for failures.
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
