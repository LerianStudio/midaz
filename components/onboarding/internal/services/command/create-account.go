package command

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account: %v", cai)

	if !uc.RabbitMQRepo.CheckRabbitMQHealth() {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Message Broker is unavailable", err)

		logger.Errorf("Message Broker is unavailable: %v", err)

		return nil, err
	}

	err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting validations failed", err)

		logger.Errorf("Accounting validations failed: %v", err)

		return nil, err
	}

	if libCommons.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	status := uc.determineStatus(cai)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset", err)

		return nil, err
	}

	var portfolioUUID uuid.UUID

	if libCommons.IsNilOrEmpty(cai.EntityID) && !libCommons.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find portfolio", err)

			logger.Errorf("Error find portfolio to get Entity ID: %v", err)

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find parent account", err)

			return nil, err
		}

		if acc.AssetCode != cai.AssetCode {
			err := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate parent account", err)

			return nil, err
		}
	}

	ID := libCommons.GenerateUUIDv7().String()

	var alias *string
	if !libCommons.IsNilOrEmpty(cai.Alias) {
		alias = cai.Alias

		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

			return nil, err
		}
	} else {
		alias = &ID
	}

	blocked := false
	if cai.Blocked != nil {
		blocked = *cai.Blocked
	}

	account := &mmodel.Account{
		ID:              ID,
		AssetCode:       cai.AssetCode,
		Alias:           alias,
		Name:            cai.Name,
		Type:            cai.Type,
		Blocked:         blocked,
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
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)

		logger.Errorf("Error creating account: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account metadata", err)

		logger.Errorf("Error creating account metadata: %v", err)

		return nil, err
	}

	acc.Metadata = metadata

	logger.Infof("Sending account to transaction queue...")
	uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)

	return acc, nil
}

// determineStatus determines the status of the account.
func (uc *UseCase) determineStatus(cai *mmodel.CreateAccountInput) mmodel.Status {
	var status mmodel.Status
	if cai.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cai.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cai.Status
	}

	status.Description = cai.Status.Description

	return status
}

// applyAccountingValidations applies the accounting validations to the account.
func (uc *UseCase) applyAccountingValidations(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.apply_accounting_validations")
	defer span.End()

	accountingValidation := os.Getenv("ACCOUNT_TYPE_VALIDATION")
	if !strings.Contains(accountingValidation, organizationID.String()+":"+ledgerID.String()) {
		logger.Infof("Accounting validations are disabled")

		return nil
	}

	if strings.ToLower(key) == "external" {
		logger.Infof("External account type, skipping validation")

		return nil
	}

	_, err := uc.AccountTypeRepo.FindByKey(ctx, organizationID, ledgerID, key)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountType, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Not found, invalid account type", err)

			logger.Warnf("Account type not found, invalid account type")

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account type", err)

		logger.Errorf("Error finding account type: %v", err)

		return err
	}

	return nil
}
