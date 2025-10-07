// Package command implements write operations (commands) for the onboarding service.
// This file contains the DeleteAssetByID command implementation.
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

// DeleteAssetByID soft-deletes an asset and its associated external account.
//
// This method implements the delete asset use case, which:
// 1. Fetches the asset to validate it exists
// 2. Finds and deletes the associated external account
// 3. Soft-deletes the asset itself
//
// Business Rules:
//   - Asset must exist and not be already deleted
//   - Asset should not have any remaining balances in accounts
//   - Associated external account is automatically deleted
//   - Soft delete is idempotent (deleting already deleted asset returns error)
//
// External Account Cleanup:
//   - Every asset has an associated external account with alias "@external/{CODE}"
//   - This external account is automatically deleted when the asset is deleted
//   - External account deletion happens before asset deletion
//   - If external account doesn't exist, continues with asset deletion
//
// Soft Deletion:
//   - Sets DeletedAt timestamp on both asset and external account
//   - Records remain in database for audit purposes
//   - Excluded from normal queries (WHERE deleted_at IS NULL)
//   - Cannot be undeleted (no restore operation)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the asset to delete
//
// Returns:
//   - error: Business error if asset not found, database error if deletion fails
//
// Possible Errors:
//   - ErrAssetIDNotFound: Asset doesn't exist or already deleted
//   - ErrBalanceRemainingDeletion: Asset has remaining balances (enforced elsewhere)
//   - Database errors: Foreign key violations, connection failures
//
// Example:
//
//	err := useCase.DeleteAssetByID(ctx, orgID, ledgerID, assetID)
//	if err != nil {
//	    return err
//	}
//	// Asset and its external account are soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_asset_by_id"
//   - Records errors as span events
//   - Tracks both external account and asset deletion
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
