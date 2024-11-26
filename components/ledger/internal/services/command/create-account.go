package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpointers"
	"github.com/google/uuid"
)

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account: %v", cai)

	if common.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if err := common.ValidateAccountType(cai.Type); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate account type", err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	status := uc.determineStatus(cai)
	balanceValue := float64(0)

	balance := mmodel.Balance{
		Available: &balanceValue,
		OnHold:    &balanceValue,
		Scale:     &balanceValue,
	}

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		mopentelemetry.HandleSpanError(&span, "Failed to find asset", constant.ErrAssetCodeNotFound)

		return nil, common.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	var portfolioUUID uuid.UUID

	if common.IsNilOrEmpty(cai.EntityID) && !common.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find portfolio", err)

			logger.Errorf("Error find portfolio to get Entity ID: %v", err)

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !common.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find parent account", err)

			return nil, common.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		}

		if acc.AssetCode != cai.AssetCode {
			mopentelemetry.HandleSpanError(&span, "Failed to validate parent account", constant.ErrMismatchedAssetCode)

			return nil, common.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		}
	}

	if !common.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

			return nil, err
		}
	}

	if cai.AllowReceiving == nil {
		cai.AllowReceiving = mpointers.Bool(true)
	}

	if cai.AllowSending == nil {
		cai.AllowSending = mpointers.Bool(true)
	}

	account := &mmodel.Account{
		ID:              common.GenerateUUIDv7().String(),
		AssetCode:       cai.AssetCode,
		Alias:           cai.Alias,
		Name:            cai.Name,
		Type:            cai.Type,
		ParentAccountID: cai.ParentAccountID,
		ProductID:       cai.ProductID,
		OrganizationID:  organizationID.String(),
		PortfolioID:     cai.PortfolioID,
		LedgerID:        ledgerID.String(),
		EntityID:        cai.EntityID,
		Balance:         balance,
		Status:          status,
		AllowSending:    cai.AllowSending,
		AllowReceiving:  cai.AllowReceiving,
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
	if cai.Status.IsEmpty() || common.IsNilOrEmpty(&cai.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cai.Status
	}

	status.Description = cai.Status.Description

	return status
}
