// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting an asset.
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
// This use case performs a soft delete by setting the DeletedAt timestamp on both
// the asset and its corresponding external account. The records are preserved
// for auditing but are excluded from normal queries.
//
// Business Rules:
//   - The asset must exist and not have been previously deleted.
//   - The associated external account is automatically soft-deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the asset to be deleted.
//
// Returns:
//   - error: An error if the asset is not found or if the deletion fails.
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
