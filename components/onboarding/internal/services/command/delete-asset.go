package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// DeleteAssetByID delete an asset from the repository by ids.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_asset_by_id")
	defer span.End()

	logger.Infof("Remove asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get asset on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Asset ID not found: %s", id.String())
			return libCommons.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		logger.Errorf("Error getting asset: %v", err)

		return err
	}

	aAlias := constant.DefaultExternalAccountAliasPrefix + asset.Code

	acc, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve asset external account", err)
		logger.Errorf("Error retrieving asset external account: %v", err)

		return err
	}

	if len(acc) > 0 {
		err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, nil, uuid.MustParse(acc[0].ID))
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to delete asset external account", err)
			logger.Errorf("Error deleting asset external account: %v", err)

			return err
		}
	}

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete asset on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Asset ID not found: %s", id.String())
			return libCommons.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		logger.Errorf("Error deleting asset: %v", err)

		return err
	}

	return nil
}
