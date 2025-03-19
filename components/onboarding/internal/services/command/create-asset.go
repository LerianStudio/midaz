package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CreateAsset creates a new asset and persists data in the repository.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput) (*mmodel.Asset, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	assetID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewAssetOperation("create", assetID)

	op.WithAttributes(
		attribute.String("asset_type", cii.Type),
		attribute.String("asset_code", cii.Code),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to create asset: %v", cii)

	var status mmodel.Status
	if cii.Status.IsEmpty() || pkg.IsNilOrEmpty(&cii.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	status.Description = cii.Status.Description

	if err := pkg.ValidateType(cii.Type); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to validate asset type", err)
		op.WithAttribute("error_detail", "invalid_type")
		op.RecordError(ctx, "validation_error", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if err := pkg.ValidateCode(cii.Code); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to validate asset code", err)
		op.WithAttribute("error_detail", "invalid_code")
		op.RecordError(ctx, "validation_error", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if cii.Type == "currency" {
		if err := pkg.ValidateCurrency(cii.Code); err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to validate asset currency", err)
			op.WithAttribute("error_detail", "invalid_currency")
			op.RecordError(ctx, "validation_error", err)

			return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
		}
	}

	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to find asset by name or code", err)
		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	asset := &mmodel.Asset{
		ID:             assetID,
		Name:           cii.Name,
		Type:           cii.Type,
		Code:           cii.Code,
		Status:         status,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	inst, err := uc.AssetRepo.Create(ctx, asset)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create asset", err)
		logger.Errorf("Error creating asset: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create asset metadata", err)
		logger.Errorf("Error creating asset metadata: %v", err)

		return nil, err
	}

	inst.Metadata = metadata

	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to retrieve asset external account", err)
		logger.Errorf("Error retrieving asset external account: %v", err)

		return nil, err
	}

	if len(account) == 0 {
		logger.Infof("Creating external account for asset: %s", cii.Code)

		extAccountOp := uc.Telemetry.NewAccountOperation("create", "external")
		extAccountOp.WithAttributes(
			attribute.String("account_type", "external"),
			attribute.String("asset_code", cii.Code),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("parent_asset_id", inst.ID),
		)
		extAccountOp.RecordSystemicMetric(ctx)

		eAccount := &mmodel.Account{
			ID:              pkg.GenerateUUIDv7().String(),
			AssetCode:       cii.Code,
			Alias:           &aAlias,
			Name:            "External " + cii.Code,
			Type:            "external",
			OrganizationID:  organizationID.String(),
			LedgerID:        ledgerID.String(),
			ParentAccountID: nil,
			SegmentID:       nil,
			PortfolioID:     nil,
			EntityID:        nil,
			Status: mmodel.Status{
				Code:        "external",
				Description: &aStatusDescription,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		acc, err := uc.AccountRepo.Create(ctx, eAccount)
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to create asset external account", err)
			logger.Errorf("Error creating asset external account: %v", err)

			return nil, err
		}

		logger.Infof("External account created for asset %s with alias %s", cii.Code, aAlias)

		logger.Infof("Sending external account to transaction queue...")
		uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)
	}

	op.End(ctx, "success")

	return inst, nil
}
