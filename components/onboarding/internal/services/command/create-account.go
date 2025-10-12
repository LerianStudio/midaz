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

// CreateAccount creates a new account and persists it in the repository.
//
// This function orchestrates the complete account creation process, including:
// 1. Validating message broker availability (required for transaction queue)
// 2. Applying accounting type validations
// 3. Validating asset existence
// 4. Resolving portfolio and entity references
// 5. Validating parent account relationships
// 6. Checking alias uniqueness
// 7. Persisting the account
// 8. Creating associated metadata
// 9. Publishing account creation to transaction service queue
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning this account
//   - ledgerID: The UUID of the ledger containing this account
//   - cai: The account creation input containing all required fields
//
// Returns:
//   - *mmodel.Account: The created account with generated ID and metadata
//   - error: Business validation or persistence errors
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account: %v", cai)

	// Step 1: Verify message broker health before proceeding, as we need to publish
	// the account creation event to the transaction service
	if !uc.RabbitMQRepo.CheckRabbitMQHealth() {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Message Broker is unavailable", err)

		logger.Errorf("Message Broker is unavailable: %v", err)

		return nil, err
	}

	// Step 2: Apply account type validations if enabled for this organization/ledger.
	// This ensures the account type exists and is valid per the chart of accounts.
	err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting validations failed", err)

		logger.Errorf("Accounting validations failed: %v", err)

		return nil, err
	}

	// Step 3: Generate a default account name if not provided.
	// Format: "{AssetCode} {AccountType} account" (e.g., "USD deposit account")
	if libCommons.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	// Step 4: Determine account status, defaulting to ACTIVE if not specified
	status := uc.determineStatus(cai)

	// Step 5: Validate that the specified asset exists in this ledger.
	// An account must be associated with a valid asset (currency, crypto, etc.)
	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset", err)

		return nil, err
	}

	var portfolioUUID uuid.UUID

	// Step 6: Resolve EntityID from Portfolio if needed.
	// If no EntityID is provided but a PortfolioID is, we inherit the EntityID from the portfolio.
	// This maintains the relationship between accounts and external entities.
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

	// Step 7: Validate parent account relationship if this is a sub-account.
	// Sub-accounts must have a valid parent and must share the same asset code.
	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find parent account", err)

			return nil, err
		}

		// Ensure parent and child accounts use the same asset (e.g., both USD)
		if acc.AssetCode != cai.AssetCode {
			err := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate parent account", err)

			return nil, err
		}
	}

	// Step 8: Generate a UUIDv7 for the account. UUIDv7 provides time-ordered IDs for better indexing.
	ID := libCommons.GenerateUUIDv7().String()

	// Step 9: Handle account alias. If provided, validate uniqueness. If not, use the generated ID as alias.
	// Aliases provide human-readable identifiers for accounts (e.g., "@cash_account")
	var alias *string
	if !libCommons.IsNilOrEmpty(cai.Alias) {
		alias = cai.Alias

		// Verify the alias is not already in use within this ledger
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

			return nil, err
		}
	} else {
		alias = &ID
	}

	// Step 10: Construct the account entity with all validated and resolved fields
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

	// Step 11: Persist the account to PostgreSQL
	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)

		logger.Errorf("Error creating account: %v", err)

		return nil, err
	}

	// Step 12: Store custom metadata in MongoDB if provided.
	// Metadata is stored separately to support flexible, schema-less attributes.
	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account metadata", err)

		logger.Errorf("Error creating account metadata: %v", err)

		return nil, err
	}

	acc.Metadata = metadata

	// Step 13: Publish account creation event to transaction service via RabbitMQ.
	// This allows the transaction service to initialize balance tracking for this account.
	logger.Infof("Sending account to transaction queue...")
	uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)

	return acc, nil
}

// determineStatus resolves the account status from input, defaulting to ACTIVE if not specified.
//
// If the input status is empty or has no code, this function will set the status to ACTIVE.
// This ensures all accounts have a valid operational status at creation time.
//
// Parameters:
//   - cai: The account creation input containing the desired status
//
// Returns:
//   - mmodel.Status: The resolved status with code and optional description
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

// applyAccountingValidations validates the account type against the chart of accounts.
//
// This function checks whether account type validation is enabled for the given
// organization/ledger combination via the ACCOUNT_TYPE_VALIDATION environment variable.
// The format is "orgID:ledgerID" to enable validation for specific ledgers.
//
// Validation ensures:
// 1. The account type exists in the AccountType repository (chart of accounts)
// 2. External account types bypass validation (system-managed)
//
// This is critical for financial accuracy as it enforces the configured chart of accounts,
// preventing the creation of accounts with undefined or invalid types.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The organization UUID to check validation settings for
//   - ledgerID: The ledger UUID to check validation settings for
//   - key: The account type key (e.g., "deposit", "liability", "current_assets")
//
// Returns:
//   - error: ErrInvalidAccountType if the type doesn't exist in the chart, or repository errors
func (uc *UseCase) applyAccountingValidations(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.apply_accounting_validations")
	defer span.End()

	// Check if accounting validation is enabled for this organization+ledger combination.
	// Format: "orgUUID:ledgerUUID" in the ACCOUNT_TYPE_VALIDATION env var
	accountingValidation := os.Getenv("ACCOUNT_TYPE_VALIDATION")
	if !strings.Contains(accountingValidation, organizationID.String()+":"+ledgerID.String()) {
		logger.Infof("Accounting validations are disabled")

		return nil
	}

	// External accounts are system-managed and bypass chart of accounts validation
	if strings.ToLower(key) == "external" {
		logger.Infof("External account type, skipping validation")

		return nil
	}

	// Verify the account type exists in the configured chart of accounts
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
