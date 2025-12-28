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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// validateAccountPrerequisites validates asset, portfolio, and parent account before account creation
func (uc *UseCase) validateAccountPrerequisites(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, span *trace.Span) (uuid.UUID, error) {
	logger := libCommons.NewLoggerFromContext(ctx)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset", err)

		return uuid.Nil, err
	}

	var portfolioUUID uuid.UUID

	if libCommons.IsNilOrEmpty(cai.EntityID) && !libCommons.IsNilOrEmpty(cai.PortfolioID) {
		assert.That(assert.ValidUUID(*cai.PortfolioID),
			"portfolio ID must be valid UUID",
			"portfolio_id", *cai.PortfolioID)
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find portfolio", err)
			logger.Errorf("Error find portfolio to get Entity ID: %v", err)

			return uuid.Nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		assert.NotNil(portfolio, "portfolio must exist after successful Find",
			"portfolio_id", portfolioUUID)

		cai.EntityID = &portfolio.EntityID
	}

	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		if err := uc.validateParentAccount(ctx, organizationID, ledgerID, &portfolioUUID, cai, span); err != nil {
			return uuid.Nil, err
		}
	}

	return portfolioUUID, nil
}

// validateParentAccount validates the parent account exists, has matching asset code, and no circular hierarchy.
func (uc *UseCase) validateParentAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioUUID *uuid.UUID, cai *mmodel.CreateAccountInput, span *trace.Span) error {
	// Safe UUID parsing - avoids panic on malformed input
	parsedParentID, parseErr := uuid.Parse(*cai.ParentAccountID)
	if parseErr != nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid parent account ID format", businessErr)

		return businessErr
	}

	acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioUUID, parsedParentID)
	if err != nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", businessErr)

		return businessErr
	}

	assert.NotNil(acc, "parent account must exist after successful Find",
		"parent_account_id", parsedParentID.String())

	if acc.AssetCode != cai.AssetCode {
		businessErr := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", businessErr)

		return businessErr
	}

	// Check for circular hierarchy before allowing account creation (CREATE only).
	// Note: This check is not atomic with account creation. For strict prevention,
	// consider adding a database trigger or constraint. See TOCTOU note in docs.
	if err := uc.detectCycleInHierarchy(ctx, organizationID, ledgerID, parsedParentID.String()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Circular hierarchy check failed: "+err.Error(), err)
		return err
	}

	return nil
}

// createAccountBalance creates the default balance for an account via BalancePort
func (uc *UseCase) createAccountBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, acc *mmodel.Account, cai *mmodel.CreateAccountInput, _, _ string, span *trace.Span) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	assert.NotNil(acc.Alias, "account alias must not be nil before balance creation",
		"account_id", acc.ID)

	balanceInput := mmodel.CreateBalanceInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      uuid.MustParse(acc.ID),
		Alias:          *acc.Alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cai.AssetCode,
		AccountType:    cai.Type,
		AllowSending:   true,
		AllowReceiving: true,
	}

	_, err := uc.BalancePort.CreateBalanceSync(ctx, balanceInput)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)
		logger.Errorf("Failed to create default balance: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return nil
}

// buildAccountModel builds an account model from input parameters
func (uc *UseCase) buildAccountModel(organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, id string, alias *string, status mmodel.Status) *mmodel.Account {
	blocked := false
	if cai.Blocked != nil {
		blocked = *cai.Blocked
	}

	return &mmodel.Account{
		ID:              id,
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
}

// handleBalanceCreationError handles balance creation failure with compensation
func (uc *UseCase) handleBalanceCreationError(ctx context.Context, err error, organizationID, ledgerID uuid.UUID, portfolioUUID uuid.UUID, accountID string) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	assert.That(assert.ValidUUID(accountID),
		"account ID must be valid UUID",
		"account_id", accountID)

	delErr := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(accountID))
	if delErr != nil {
		logger.Errorf("Failed to delete account during compensation: %v", delErr)
	}

	var (
		unauthorized pkg.UnauthorizedError
		forbidden    pkg.ForbiddenError
	)

	if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
		return err
	}

	return pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
}

// CreateAccount creates an account and metadata, then synchronously creates the default balance via gRPC.
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

	portfolioUUID, err := uc.validateAccountPrerequisites(ctx, organizationID, ledgerID, cai, &span)
	if err != nil {
		return nil, err
	}

	ID := libCommons.GenerateUUIDv7().String()

	assert.That(assert.ValidUUID(ID),
		"generated account ID must be valid UUID",
		"account_id", ID)

	alias, err := uc.resolveAccountAlias(ctx, organizationID, ledgerID, cai, ID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)
		return nil, err
	}

	account := uc.buildAccountModel(organizationID, ledgerID, cai, ID, alias, status)

	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
		logger.Errorf("Error creating account: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	assert.NotNil(acc, "repository Create must return non-nil account on success",
		"account_id", account.ID)

	if err := uc.createAccountBalance(ctx, organizationID, ledgerID, acc, cai, requestID, token, &span); err != nil {
		return nil, uc.handleBalanceCreationError(ctx, err, organizationID, ledgerID, portfolioUUID, acc.ID)
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
// Returns provided alias when present and valid; otherwise falls back to generated ID.
func (uc *UseCase) resolveAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, generatedID string) (*string, error) {
	if !libCommons.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *cai.Alias)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return cai.Alias, nil
	}

	return &generatedID, nil
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
			businessErr := pkg.ValidateBusinessError(constant.ErrInvalidAccountType, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Not found, invalid account type", businessErr)

			logger.Warnf("Account type not found, invalid account type")

			return businessErr
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account type", err)

		logger.Errorf("Error finding account type: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	return nil
}

// detectCycleInHierarchy checks if setting parentAccountID as parent of a new account
// would create a circular reference. It traverses up the hierarchy from parentAccountID
// with a depth limit to prevent DoS attacks.
//
// This function is used during CREATE operations only - the new account doesn't exist yet,
// so we only need to check if the existing parent chain already contains a cycle.
//
// Returns:
// - nil: No cycle detected, safe to proceed
// - ErrCircularAccountHierarchy: Cycle detected in existing hierarchy
// - ErrAccountHierarchyTooDeep: Hierarchy exceeds max depth
// - ErrCorruptedParentAccountUUID: Invalid UUID detected (strict mode only)
// - Other errors: Database or validation errors
func (uc *UseCase) detectCycleInHierarchy(ctx context.Context, organizationID, ledgerID uuid.UUID, parentAccountID string) error {
	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.detect_cycle_in_hierarchy")
	defer span.End()

	visited := make(map[string]bool)
	currentID := parentAccountID
	depth := 0

	for currentID != "" {
		depth++

		// Depth limit check - prevents DoS via deep hierarchies
		if depth > constant.MaxAccountHierarchyDepth {
			err := pkg.ValidateBusinessError(constant.ErrAccountHierarchyTooDeep, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Hierarchy depth limit exceeded", err)
			logger.Warnf("Account hierarchy depth limit exceeded: %d levels", depth)

			return err
		}

		// Cycle detection - check if we've visited this node
		if visited[currentID] {
			err := pkg.ValidateBusinessError(constant.ErrCircularAccountHierarchy, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Circular hierarchy detected in existing data", err)
			logger.Warnf("Circular hierarchy detected: already visited account %s", currentID)

			return err
		}

		visited[currentID] = true

		// Safe UUID parsing - handles corrupted database data based on strictMode configuration.
		// When strictMode is enabled (STRICT_PARENT_UUID_VALIDATION=true), parse failures
		// return an error to fail the operation fast.
		// When strictMode is disabled (default), we treat corrupted UUIDs as end-of-chain
		// to prevent cascading failures from bad data while still allowing the operation.
		// In both cases, a metric is emitted and the error is logged for incident tracking.
		parsedID, parseErr := uuid.Parse(currentID)
		if parseErr != nil {
			// Always emit metric for observability regardless of mode
			if metricFactory != nil {
				metricFactory.Counter(utils.ParentUUIDCorruption).
					WithLabels(map[string]string{
						"organization_id": organizationID.String(),
						"ledger_id":       ledgerID.String(),
					}).
					AddOne(ctx)
			}

			// Always log with full error details for incident tracking
			logger.Warnf("DATA_CORRUPTION: Invalid UUID in parent account chain: %s, parse_error: %v, organization_id: %s, ledger_id: %s",
				currentID, parseErr, organizationID.String(), ledgerID.String())

			// Check strict mode configuration
			strictMode := strings.EqualFold(os.Getenv("STRICT_PARENT_UUID_VALIDATION"), "true")
			if strictMode {
				err := pkg.ValidateBusinessError(constant.ErrCorruptedParentAccountUUID, reflect.TypeOf(mmodel.Account{}).Name())
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Corrupted UUID detected in parent account chain (strict mode)", err)

				return err
			}

			// Lenient mode: defensive termination - treat as end of chain
			return nil
		}

		// Fetch the current account to get its parent
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, parsedID)
		if err != nil {
			// Check if it's a "not found" error using the correct error type
			// AccountRepo.Find returns pkg.EntityNotFoundError, not services.ErrDatabaseItemNotFound
			var entityNotFoundErr pkg.EntityNotFoundError
			if errors.As(err, &entityNotFoundErr) {
				logger.Infof("Account %s not found, end of hierarchy chain", currentID)
				return nil
			}
			// Propagate real database errors
			libOpentelemetry.HandleSpanError(&span, "Database error during hierarchy check", err)
			logger.Errorf("Error fetching account %s during hierarchy check: %v", currentID, err)

			return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
		}

		// Move to parent
		if acc.ParentAccountID == nil || *acc.ParentAccountID == "" {
			// Reached root, no cycle
			return nil
		}

		currentID = *acc.ParentAccountID
	}

	return nil
}
