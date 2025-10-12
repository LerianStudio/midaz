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

// DeleteAssetByID performs a soft delete of an asset and its associated external account.
//
// This function implements a two-step deletion process:
// 1. Delete the external account automatically created for this asset (e.g., "@external/USD")
// 2. Delete the asset itself
//
// The external account must be deleted first to maintain referential integrity.
// Both deletions are soft deletes (setting DeletedAt timestamp) to preserve audit trails.
//
// Repository layer should enforce that assets with non-zero balances in accounts
// cannot be deleted (ErrBalanceRemainingDeletion).
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the asset
//   - ledgerID: The UUID of the ledger containing the asset
//   - id: The UUID of the asset to delete
//
// Returns:
//   - error: ErrAssetIDNotFound if not found, ErrBalanceRemainingDeletion if balances exist, or repository errors
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_asset_by_id")
	defer span.End()

	logger.Infof("Remove asset for id: %s", id)

	// Step 1: Retrieve the asset to get its code for external account lookup
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

	// Step 2: Delete the associated external account first (e.g., "@external/USD").
	// External accounts are auto-created with assets and must be cleaned up together.
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

	// Step 3: Delete the asset itself after external account is removed
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
