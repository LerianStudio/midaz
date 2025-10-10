// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new account.
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

// CreateAccount creates a new account in the repository.
//
// This use case orchestrates the creation of a new account by:
// 1. Validating dependencies like RabbitMQ and account type existence.
// 2. Ensuring data integrity by checking asset codes, portfolios, and parent accounts.
// 3. Generating a unique alias if one is not provided.
// 4. Persisting the account in PostgreSQL and its metadata in MongoDB.
// 5. Publishing an event to RabbitMQ for balance initialization in the transaction service.
//
// Business Rules:
//   - An asset code must be valid and exist in the ledger.
//   - If provided, the parent account must exist and share the same asset code.
//   - An account alias must be unique within the ledger; if not provided, the account ID is used.
//   - If accounting validation is enabled, the account type must exist.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the account.
//   - ledgerID: The UUID of the ledger where the account will be created.
//   - cai: The input data for creating the account.
//
// Returns:
//   - *mmodel.Account: The newly created account, complete with its metadata.
//   - error: An error if the creation fails due to business rule violations or database issues.
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

		// FIXME: This logic is incorrect. FindByAlias returns an error if the account is *not* found.
		// The code should check if the error is `services.ErrDatabaseItemNotFound` and proceed in that case.
		// If the error is nil, it means an account with the alias already exists, and an `ErrAliasUnavailability`
		// error should be returned. Any other error should be returned directly.
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

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

// determineStatus sets the default status for a new account.
// If the provided status is empty, it defaults to "ACTIVE".
func (uc *UseCase) determineStatus(cai *mmodel.CreateAccountInput) mmodel.Status {
	if cai.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cai.Status.Code) {
		return mmodel.Status{
			Code:        "ACTIVE",
			Description: cai.Status.Description,
		}
	}
	return cai.Status
}

// applyAccountingValidations checks for the existence of an account type if validation is enabled.
//
// This validation is controlled by the ACCOUNT_TYPE_VALIDATION environment variable, which
// should contain a comma-separated list of "organizationID:ledgerID" pairs where
// validation is required. The "external" account type is always exempt from this check.
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
