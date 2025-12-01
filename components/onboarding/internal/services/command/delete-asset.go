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

// DeleteAssetByID removes an asset and its external account from the repository.
//
// This function performs a comprehensive deletion that includes both the asset
// and its automatically-created external account. The external account is used
// for transactions with external parties (outside the ledger).
//
// # External Account Cleanup
//
// When an asset is created, a corresponding external account is automatically
// created with the alias pattern: "@external/{asset_code}" (e.g., "@external/USD").
// This function deletes both:
//  1. The external account (if it exists)
//  2. The asset itself
//
// # Deletion Order
//
// The deletion follows this order:
//  1. Verify asset exists
//  2. Find and delete external account by alias
//  3. Delete the asset
//
// This order ensures referential integrity is maintained.
//
// # Deletion Constraints
//
// Before deletion, consider:
//   - All accounts using this asset should be closed
//   - All balances in this asset should be zeroed
//   - Transaction history will reference the deleted asset
//   - Asset rates using this asset should be removed
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.delete_asset_by_id"
//  3. Fetch asset to verify existence and get asset code
//  4. Build external account alias: "@external/{asset_code}"
//  5. Find external account by alias
//  6. If external account exists, delete it
//  7. Delete the asset
//  8. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this asset
//   - id: The UUID of the asset to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Asset not found (ErrAssetIDNotFound)
//   - External account deletion fails
//   - Asset deletion fails
//
// # Error Scenarios
//
//   - ErrAssetIDNotFound: Asset with given ID not found
//   - External account retrieval failure
//   - External account deletion failure
//   - Asset deletion failure
//   - Database connection failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.delete_asset_by_id" with error events.
// Logs asset ID at info level, warnings for not found, errors for failures.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_asset_by_id")
	defer span.End()

	logger.Infof("Remove asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Warnf("Asset ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get asset on repo by id", err)

		logger.Errorf("Error getting asset: %v", err)

		return err
	}

	aAlias := constant.DefaultExternalAccountAliasPrefix + asset.Code

	acc, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve asset external account", err)

		logger.Errorf("Error retrieving asset external account: %v", err)

		return err
	}

	if len(acc) > 0 {
		err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, nil, uuid.MustParse(acc[0].ID))
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete asset external account", err)

			logger.Errorf("Error deleting asset external account: %v", err)

			return err
		}
	}

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Warnf("Asset ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete asset on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete asset on repo by id", err)

		logger.Errorf("Error deleting asset: %v", err)

		return err
	}

	return nil
}
