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

// CreateAccount creates a new account and persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	accountID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewAccountOperation("create", accountID)

	op.WithAttributes(
		attribute.String("account_type", cai.Type),
		attribute.String("asset_code", cai.AssetCode),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to create account: %v", cai)

	if pkg.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if err := pkg.ValidateAccountType(cai.Type); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to validate account type", err)
		op.WithAttribute("error_detail", "invalid_type")
		op.RecordError(ctx, "validation_error", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	status := uc.determineStatus(cai)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		mopentelemetry.HandleSpanError(&op.span, "Failed to find asset", constant.ErrAssetCodeNotFound)
		op.WithAttribute("error_detail", "asset_not_found")
		op.RecordError(ctx, "validation_error", constant.ErrAssetCodeNotFound)

		return nil, pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	var portfolioUUID uuid.UUID

	if pkg.IsNilOrEmpty(cai.EntityID) && !pkg.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)
		op.WithAttribute("portfolio_id", *cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to find portfolio", err)
			logger.Errorf("Error find portfolio to get Entity ID: %v", err)
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "portfolio_error", err)

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
		if cai.EntityID != nil {
			op.WithAttribute("entity_id", *cai.EntityID)
		}
	}

	if !pkg.IsNilOrEmpty(cai.ParentAccountID) {
		op.WithAttribute("parent_account_id", *cai.ParentAccountID)

		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to find parent account", err)
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "parent_account_error", err)

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		}

		if acc.AssetCode != cai.AssetCode {
			mopentelemetry.HandleSpanError(&op.span, "Failed to validate parent account", constant.ErrMismatchedAssetCode)
			op.WithAttribute("error_detail", "mismatched_asset_code")
			op.RecordError(ctx, "validation_error", constant.ErrMismatchedAssetCode)

			return nil, pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		}
	}

	var alias *string
	if !pkg.IsNilOrEmpty(cai.Alias) {
		alias = cai.Alias
		op.WithAttribute("alias", *cai.Alias)

		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			mopentelemetry.HandleSpanError(&op.span, "Failed to find account by alias", err)
			op.WithAttribute("error_detail", err.Error())
			op.RecordError(ctx, "alias_error", err)

			return nil, err
		}
	} else {
		alias = &accountID
	}

	account := &mmodel.Account{
		ID:              accountID,
		AssetCode:       cai.AssetCode,
		Alias:           alias,
		Name:            cai.Name,
		Type:            cai.Type,
		ParentAccountID: cai.ParentAccountID,
		SegmentID:       cai.SegmentID,
		OrganizationID:  organizationID.String(),
		PortfolioID:     cai.PortfolioID,
		LedgerID:        ledgerID.String(),
		EntityID:        cai.EntityID,
		Status:          status,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create account", err)
		logger.Errorf("Error creating account: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	op.WithAttribute("account_name", acc.Name)

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create account metadata", err)
		logger.Errorf("Error creating account metadata: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	acc.Metadata = metadata

	logger.Infof("Sending account to transaction queue...")
	uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)

	op.End(ctx, "success")

	return acc, nil
}

// determineStatus determines the status of the account.
func (uc *UseCase) determineStatus(cai *mmodel.CreateAccountInput) mmodel.Status {
	var status mmodel.Status
	if cai.Status.IsEmpty() || pkg.IsNilOrEmpty(&cai.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cai.Status
	}

	status.Description = cai.Status.Description

	return status
}
