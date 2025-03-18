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

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "account", "create",
		attribute.String("account_type", cai.Type),
		attribute.String("asset_code", cai.AssetCode),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to create account: %v", cai)

	if pkg.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if err := pkg.ValidateAccountType(cai.Type); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate account type", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "validation_error",
			attribute.String("account_type", cai.Type),
			attribute.String("error_detail", "invalid_type"))

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	status := uc.determineStatus(cai)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		mopentelemetry.HandleSpanError(&span, "Failed to find asset", constant.ErrAssetCodeNotFound)

		// Record error
		uc.recordOnboardingError(ctx, "account", "validation_error",
			attribute.String("asset_code", cai.AssetCode),
			attribute.String("error_detail", "asset_not_found"))

		return nil, pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	var portfolioUUID uuid.UUID

	if pkg.IsNilOrEmpty(cai.EntityID) && !pkg.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find portfolio", err)

			logger.Errorf("Error find portfolio to get Entity ID: %v", err)

			// Record error
			uc.recordOnboardingError(ctx, "account", "portfolio_error",
				attribute.String("portfolio_id", *cai.PortfolioID),
				attribute.String("error_detail", err.Error()))

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !pkg.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find parent account", err)

			// Record error
			uc.recordOnboardingError(ctx, "account", "parent_account_error",
				attribute.String("parent_account_id", *cai.ParentAccountID),
				attribute.String("error_detail", err.Error()))

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		}

		if acc.AssetCode != cai.AssetCode {
			mopentelemetry.HandleSpanError(&span, "Failed to validate parent account", constant.ErrMismatchedAssetCode)

			// Record error
			uc.recordOnboardingError(ctx, "account", "validation_error",
				attribute.String("parent_account_id", *cai.ParentAccountID),
				attribute.String("error_detail", "mismatched_asset_code"))

			return nil, pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		}
	}

	ID := pkg.GenerateUUIDv7().String()

	var alias *string
	if !pkg.IsNilOrEmpty(cai.Alias) {
		alias = cai.Alias

		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

			// Record error
			uc.recordOnboardingError(ctx, "account", "alias_error",
				attribute.String("alias", *cai.Alias),
				attribute.String("error_detail", err.Error()))

			return nil, err
		}
	} else {
		alias = &ID
	}

	account := &mmodel.Account{
		ID:              ID,
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
		mopentelemetry.HandleSpanError(&span, "Failed to create account", err)

		logger.Errorf("Error creating account: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "creation_error",
			attribute.String("account_name", cai.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create account metadata", err)

		logger.Errorf("Error creating account metadata: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "metadata_error",
			attribute.String("account_id", acc.ID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	acc.Metadata = metadata

	logger.Infof("Sending account to transaction queue...")
	uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "account", "create", "success",
		attribute.String("account_id", acc.ID),
		attribute.String("account_name", acc.Name),
		attribute.String("account_type", acc.Type),
		attribute.String("asset_code", acc.AssetCode))

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
