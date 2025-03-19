package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// DeleteAssetByID delete an asset from the repository by ids.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)

	op := uc.Telemetry.NewAssetOperation("delete", id.String())

	op.WithAttributes(
		attribute.String("asset_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Remove asset for id: %s", id)

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to get asset on repo by id", err)
		logger.Errorf("Error getting asset on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "find_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		return err
	}

	op.WithAttribute("asset_code", asset.Code)

	aAlias := constant.DefaultExternalAccountAliasPrefix + asset.Code

	acc, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to retrieve asset external account", err)
		logger.Errorf("Error retrieving asset external account: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "list_account_error", err)

		return err
	}

	if len(acc) > 0 {
		extAccountOp := uc.Telemetry.NewAccountOperation("delete", acc[0].ID)
		extAccountOp.WithAttributes(
			attribute.String("account_type", "external"),
			attribute.String("account_alias", aAlias),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("parent_asset_id", id.String()),
		)

		extAccountCtx := extAccountOp.StartTrace(ctx)
		extAccountOp.RecordSystemicMetric(extAccountCtx)

		err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, nil, uuid.MustParse(acc[0].ID))
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to delete asset external account", err)
			mopentelemetry.HandleSpanError(&extAccountOp.span, "Failed to delete external account", err)
			logger.Errorf("Error deleting asset external account: %v", err)

			extAccountOp.WithAttribute("error_detail", err.Error())
			extAccountOp.RecordError(extAccountCtx, "delete_error", err)
			extAccountOp.End(extAccountCtx, "error")

			op.WithAttribute("error_detail", err.Error())
			op.WithAttribute("account_id", acc[0].ID)
			op.RecordError(ctx, "delete_account_error", err)

			return err
		}

		extAccountOp.End(extAccountCtx, "success")
	}

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete asset on repo by id", err)
		logger.Errorf("Error deleting asset on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name(), id)
		}

		return err
	}

	op.End(ctx, "success")

	return nil
}
