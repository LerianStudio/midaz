package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balanceproto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// validateAccountPrerequisites validates asset, portfolio, and parent account before account creation
func (uc *UseCase) validateAccountPrerequisites(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, span *trace.Span) (uuid.UUID, error) {
	logger := libCommons.NewLoggerFromContext(ctx)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset", err)

		return uuid.Nil, fmt.Errorf("validation failed: %w", err)
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

			return uuid.Nil, fmt.Errorf("failed to find: %w", err)
		}

		assert.NotNil(portfolio, "portfolio must exist after successful Find",
			"portfolio_id", portfolioUUID)

		cai.EntityID = &portfolio.EntityID
	}

	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		assert.That(assert.ValidUUID(*cai.ParentAccountID),
			"parent account ID must be valid UUID",
			"parent_account_id", *cai.ParentAccountID)

		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", err)

			return uuid.Nil, fmt.Errorf("validation failed: %w", err)
		}

		assert.NotNil(acc, "parent account must exist after successful Find",
			"parent_account_id", *cai.ParentAccountID)

		if acc.AssetCode != cai.AssetCode {
			err := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", err)

			return uuid.Nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	return portfolioUUID, nil
}

// createAccountBalance creates the default balance for an account via gRPC
func (uc *UseCase) createAccountBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, acc *mmodel.Account, cai *mmodel.CreateAccountInput, requestID, token string, span *trace.Span) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	assert.NotNil(acc.Alias, "account alias must not be nil before balance creation",
		"account_id", acc.ID)

	balanceReq := &balanceproto.BalanceRequest{
		RequestId:      requestID,
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      acc.ID,
		Alias:          *acc.Alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cai.AssetCode,
		AccountType:    cai.Type,
		AllowSending:   true,
		AllowReceiving: true,
	}

	_, err := uc.BalancePort.CreateBalance(ctx, token, balanceReq)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance via gRPC", err)
		logger.Errorf("Failed to create default balance via gRPC: %v", err)

		return fmt.Errorf("failed to create default balance via gRPC: %w", err)
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
		return fmt.Errorf("failed to delete: %w", err)
	}

	return fmt.Errorf("validation failed: %w", pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name()))
}

// CreateAccountSync creates an account and metadata, then synchronously creates the default balance via gRPC.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Infof("Trying to create account (sync): %v", cai)

	if err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting validations failed", err)
		logger.Errorf("Accounting validations failed: %v", err)

		return nil, fmt.Errorf("operation failed: %w", err)
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
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	account := uc.buildAccountModel(organizationID, ledgerID, cai, ID, alias, status)

	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
		logger.Errorf("Error creating account: %v", err)

		return nil, fmt.Errorf("failed to create: %w", err)
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

		return nil, fmt.Errorf("validation failed: %w", err)
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
			return nil, fmt.Errorf("failed to find: %w", err)
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
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountType, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Not found, invalid account type", err)

			logger.Warnf("Account type not found, invalid account type")

			return fmt.Errorf("validation failed: %w", err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account type", err)

		logger.Errorf("Error finding account type: %v", err)

		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}
