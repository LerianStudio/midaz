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
)

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account: %v", cai)

	if pkg.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if err := pkg.ValidateAccountType(cai.Type); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate account type", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	status := uc.determineStatus(cai)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		mopentelemetry.HandleSpanError(&span, "Failed to find asset", constant.ErrAssetCodeNotFound)

		return nil, pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	var portfolioUUID uuid.UUID

	if pkg.IsNilOrEmpty(cai.EntityID) && !pkg.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find portfolio", err)

			logger.Errorf("Error find portfolio to get Entity ID: %v", err)

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !pkg.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find parent account", err)

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		}

		if acc.AssetCode != cai.AssetCode {
			mopentelemetry.HandleSpanError(&span, "Failed to validate parent account", constant.ErrMismatchedAssetCode)

			return nil, pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		}
	}

	if !pkg.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

			return nil, err
		}
	}

	account := &mmodel.Account{
		ID:              pkg.GenerateUUIDv7().String(),
		AssetCode:       cai.AssetCode,
		Alias:           cai.Alias,
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

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create account metadata", err)

		logger.Errorf("Error creating account metadata: %v", err)

		return nil, err
	}

	acc.Metadata = metadata

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
