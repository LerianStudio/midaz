// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

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

// CreateAccount creates a new account and persists it to the repository.
//
// This method implements the create account use case, which:
// 1. Validates RabbitMQ health (required for sending account to transaction service)
// 2. Applies accounting validations if enabled (account type must exist)
// 3. Sets default name if not provided (e.g., "USD deposit account")
// 4. Validates asset code exists
// 5. Validates portfolio exists if provided
// 6. Validates parent account exists and has matching asset code
// 7. Validates or generates account alias (must be unique)
// 8. Creates the account in PostgreSQL
// 9. Creates associated metadata in MongoDB
// 10. Sends account to transaction service queue for balance initialization
// 11. Returns the complete account with metadata
//
// Business Rules:
//   - Asset code must exist in the ledger
//   - Parent account (if provided) must exist and have the same asset code
//   - Account alias must be unique within the ledger
//   - If no alias provided, uses account ID as alias
//   - Portfolio (if provided) must exist, and its entity ID is copied to account
//   - Status defaults to ACTIVE if not provided
//   - Account type must exist if accounting validation is enabled
//   - External account type bypasses accounting validation
//   - RabbitMQ must be healthy (accounts need to be sent to transaction service)
//
// Data Storage:
//   - Primary data: PostgreSQL (accounts table)
//   - Metadata: MongoDB (flexible key-value storage)
//   - Queue: RabbitMQ (account creation event for transaction service)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that owns this account
//   - ledgerID: UUID of the ledger that contains this account
//   - cai: Create account input with all required and optional fields
//
// Returns:
//   - *mmodel.Account: Created account with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrMessageBrokerUnavailable: RabbitMQ is not healthy
//   - ErrInvalidAccountType: Account type doesn't exist (when validation enabled)
//   - ErrAssetCodeNotFound: Asset code doesn't exist
//   - ErrPortfolioIDNotFound: Portfolio doesn't exist
//   - ErrInvalidParentAccountID: Parent account doesn't exist
//   - ErrMismatchedAssetCode: Parent account has different asset code
//   - ErrAliasUnavailability: Alias is already in use
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateAccountInput{
//	    Name:      "Corporate Checking",
//	    AssetCode: "USD",
//	    Type:      "deposit",
//	    Alias:     ptr.String("@corporate_checking"),
//	}
//	account, err := useCase.CreateAccount(ctx, orgID, ledgerID, input)
//	if err != nil {
//	    return nil, err
//	}
//
// OpenTelemetry:
//   - Creates span "command.create_account"
//   - Records errors as span events
//   - Tracks validation and creation steps
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

// determineStatus determines the status for a new account.
//
// This helper function applies default status logic:
//   - If status is empty or code is empty, defaults to ACTIVE
//   - Otherwise, uses the provided status
//   - Always preserves the description field from input
//
// Parameters:
//   - cai: Create account input containing the status
//
// Returns:
//   - mmodel.Status: The determined status (with ACTIVE default)
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

// applyAccountingValidations validates that the account type exists if accounting validation is enabled.
//
// This function implements optional account type validation based on environment configuration.
// Accounting validation can be enabled per organization:ledger pair via the
// ACCOUNT_TYPE_VALIDATION environment variable.
//
// Validation Logic:
// 1. Check if accounting validation is enabled for this org:ledger pair
// 2. Skip validation for "external" account types (system-managed)
// 3. Verify the account type exists in the account types table
// 4. Return error if account type not found
//
// Environment Configuration:
//   - ACCOUNT_TYPE_VALIDATION: Comma-separated list of "orgID:ledgerID" pairs
//   - Example: "uuid1:uuid2,uuid3:uuid4"
//   - If org:ledger pair is in the list, validation is enforced
//
// Parameters:
//   - ctx: Context for tracing and logging
//   - organizationID: Organization UUID
//   - ledgerID: Ledger UUID
//   - key: Account type key to validate (e.g., "deposit", "loan")
//
// Returns:
//   - error: ErrInvalidAccountType if type doesn't exist, nil if valid or validation disabled
//
// Example:
//
//	// Enable validation for specific org:ledger
//	os.Setenv("ACCOUNT_TYPE_VALIDATION", "123e4567-...:987f6543-...")
//
//	// This will validate account type exists
//	err := uc.applyAccountingValidations(ctx, orgID, ledgerID, "deposit")
//
// OpenTelemetry:
//   - Creates span "command.apply_accounting_validations"
//   - Records validation results
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
