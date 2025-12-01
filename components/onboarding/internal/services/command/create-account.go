// Package command provides CQRS command handlers for the onboarding component.
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
	balanceproto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAccount creates an account with its default balance in a single synchronous operation.
//
// This command implements the complete account creation flow, ensuring that every account
// has a corresponding balance record. The operation uses gRPC to communicate with the
// transaction component for balance creation.
//
// Account Creation Process:
//
//	Step 1: Apply accounting validations (account type exists if validation enabled)
//	Step 2: Validate asset code exists in the ledger
//	Step 3: Resolve portfolio and entity relationships
//	Step 4: Validate parent account (if specified) has matching asset code
//	Step 5: Generate account ID and resolve alias
//	Step 6: Create account record in PostgreSQL
//	Step 7: Create default balance via gRPC to transaction component
//	Step 8: Compensate (delete account) if balance creation fails
//	Step 9: Create metadata in MongoDB (if provided)
//
// Why Synchronous Balance Creation:
//
// Accounts without balances are invalid in a double-entry ledger. Using synchronous
// gRPC ensures atomicity between account and balance creation. If balance creation
// fails, the account is deleted as compensation.
//
// Compensation Pattern:
//
// If the gRPC call to create the balance fails, the newly created account is deleted
// to maintain consistency. This implements a manual saga pattern without distributed
// transaction coordination.
//
// Parameters:
//   - ctx: Context with tracing and cancellation
//   - organizationID: Organization UUID (parent of ledger)
//   - ledgerID: Ledger UUID (parent of account)
//   - cai: Account creation input (name, alias, type, asset, etc.)
//   - token: JWT token for gRPC authentication
//
// Returns:
//   - *mmodel.Account: Created account with metadata
//   - error: Validation errors, database errors, or gRPC errors
//
// Error Scenarios:
//   - ErrAssetCodeNotFound: Asset code does not exist in the ledger
//   - ErrInvalidParentAccountID: Parent account not found
//   - ErrMismatchedAssetCode: Parent account has different asset code
//   - ErrAccountCreationFailed: Balance creation via gRPC failed
//
// Usage:
//
//	account, err := uc.CreateAccount(ctx, orgID, ledgerID, &mmodel.CreateAccountInput{
//	    Name:      "Savings Account",
//	    AssetCode: "USD",
//	    Type:      "asset",
//	}, jwtToken)
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account (sync): %v", cai)

	if err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type); err != nil {
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

	alias, err := uc.resolveAccountAlias(ctx, organizationID, ledgerID, cai, ID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)
		return nil, err
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
		Blocked:         &blocked,
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

	balanceReq := &balanceproto.BalanceRequest{
		RequestId:      requestID,
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      acc.ID,
		Alias:          *alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cai.AssetCode,
		AccountType:    cai.Type,
		AllowSending:   true,
		AllowReceiving: true,
	}

	_, err = uc.BalanceGRPCRepo.CreateBalance(ctx, token, balanceReq)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create default balance via gRPC", err)
		logger.Errorf("Failed to create default balance via gRPC: %v", err)

		delErr := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(acc.ID))
		if delErr != nil {
			logger.Errorf("Failed to delete account during compensation: %v", delErr)
		}

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return nil, err
		}

		return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
	}

	metadataDoc, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account metadata", err)

		logger.Errorf("Error creating account metadata: %v", err)

		return nil, err
	}

	acc.Metadata = metadataDoc

	logger.Infof("Account created synchronously with default balance")

	return acc, nil
}

// resolveAccountAlias resolves and validates the account alias.
//
// Alias Resolution Strategy:
//   - If alias provided and unique: Use the provided alias
//   - If alias provided but exists: Return conflict error
//   - If no alias provided: Use the generated account ID as alias
//
// Parameters:
//   - ctx: Context for tracing
//   - organizationID: Organization UUID
//   - ledgerID: Ledger UUID
//   - cai: Account creation input containing optional alias
//   - generatedID: The generated account UUID to use as fallback
//
// Returns:
//   - *string: The resolved alias (either provided or generated ID)
//   - error: Conflict error if alias already exists
func (uc *UseCase) resolveAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, generatedID string) (*string, error) {
	if !libCommons.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			return nil, err
		}

		return cai.Alias, nil
	}

	return &generatedID, nil
}

// determineStatus determines the initial status of a new account.
//
// Default Status: ACTIVE if not specified in the input.
//
// This allows accounts to be created in different states (e.g., PENDING for
// accounts requiring approval), while defaulting to operational state.
//
// Parameters:
//   - cai: Account creation input containing optional status
//
// Returns:
//   - mmodel.Status: The determined status (ACTIVE if not specified)
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

// applyAccountingValidations validates that the account type is valid for the ledger.
//
// Account Type Validation:
//
// When enabled via ACCOUNT_TYPE_VALIDATION environment variable, this ensures
// accounts use only predefined account types from the chart of accounts.
// This enforces accounting standards compliance.
//
// Environment Configuration:
//
//	ACCOUNT_TYPE_VALIDATION=<org_id>:<ledger_id>,<org_id>:<ledger_id>,...
//
// Bypass Conditions:
//   - Validation not enabled for the org:ledger pair
//   - Account type is "external" (external accounts bypass validation)
//
// Parameters:
//   - ctx: Context for tracing
//   - organizationID: Organization UUID
//   - ledgerID: Ledger UUID
//   - key: Account type key to validate
//
// Returns:
//   - error: ErrInvalidAccountType if type not found in chart of accounts
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
