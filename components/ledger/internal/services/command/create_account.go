// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// accountCreateOptions groups internal-only knobs for account creation.
// Public callers always use CreateAccount (PendingCRMLink=false). The pending
// variant is reserved for the CRM/Ledger orchestration saga (see
// docs/plans/plan-mode-crm-ledger-abstraction-layer-*.md) and is not wired
// into any public HTTP route.
type accountCreateOptions struct {
	// PendingCRMLink, when true, forces the new account into PENDING_CRM_LINK
	// state with blocked=true and a default balance that disallows both
	// sending and receiving. Activation is handled by ActivateAccount once
	// the CRM alias is confirmed.
	PendingCRMLink bool
}

// CreateAccount creates an account and metadata, then synchronously creates the default balance.
// The balance is created via the BalancePort interface.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	return uc.createAccountWithOptions(ctx, organizationID, ledgerID, cai, token, accountCreateOptions{PendingCRMLink: false})
}

// createAccountWithOptions is the unexported entry point that carries the
// orchestration-internal accountCreateOptions. CreateAccount delegates here
// with PendingCRMLink=false so the default path is byte-identical to the
// previous behavior.
//
//nolint:unparam // token kept for API symmetry with public CreateAccount and saga caller; reserved for future authorization propagation.
func (uc *UseCase) createAccountWithOptions(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string, opts accountCreateOptions) (*mmodel.Account, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to create account organizationID=%s ledgerID=%s type=%s", organizationID.String(), ledgerID.String(), cai.Type))

	if err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting validations failed", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Accounting validations failed: %v", err))

		return nil, err
	}

	if libCommons.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	status := uc.determineStatus(cai, opts)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset", err)

		return nil, err
	}

	var portfolioUUID uuid.UUID

	if libCommons.IsNilOrEmpty(cai.EntityID) && !libCommons.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find portfolio", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error find portfolio to get Entity ID: %v", err))

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if err := uc.validateParentAccount(ctx, span, organizationID, ledgerID, portfolioUUID, cai); err != nil {
		return nil, err
	}

	accountID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate account ID", err)
		logger.Log(ctx, libLog.LevelError, "Error generating account ID")

		return nil, err
	}

	ID := accountID.String()

	alias, err := uc.resolveAccountAlias(ctx, organizationID, ledgerID, cai, ID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find account by alias", err)
		return nil, err
	}

	// Pending-CRM-link mode forces blocked=true regardless of caller input.
	// Defense-in-depth: transaction eligibility also checks Status.Code, but
	// setting Blocked=true here means every downstream check agrees.
	blocked := cai.Blocked != nil && *cai.Blocked
	if opts.PendingCRMLink {
		blocked = true
	}

	now := time.Now().UTC()

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
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating account: %v", err))

		return nil, err
	}

	// Pending-CRM-link accounts start with sending/receiving disabled; the
	// balance-level flags re-open in ActivateAccount once the CRM alias is
	// confirmed.
	allowSendingReceiving := !opts.PendingCRMLink

	balanceInput := mmodel.CreateBalanceInput{
		RequestID:      requestID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      uuid.MustParse(acc.ID),
		Alias:          *alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cai.AssetCode,
		AccountType:    cai.Type,
		AllowSending:   allowSendingReceiving,
		AllowReceiving: allowSendingReceiving,
	}

	_, err = uc.CreateBalanceSync(ctx, balanceInput)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create default balance: %v", err))

		delErr := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(acc.ID))
		if delErr != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete account during compensation: %v", delErr))
		}

		if isAuthorizationError(err) {
			return nil, err
		}

		return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
	}

	metadataDoc, err := uc.CreateOnboardingMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating account metadata: %v", err))

		return nil, err
	}

	acc.Metadata = metadataDoc

	logger.Log(ctx, libLog.LevelInfo, "Account created synchronously with default balance")

	return acc, nil
}

// validateParentAccount resolves the parent account (when specified) and
// verifies its asset code matches the new account's asset code. Span error
// attribution is preserved from the original inline block.
func (uc *UseCase) validateParentAccount(ctx context.Context, span trace.Span, organizationID, ledgerID, portfolioUUID uuid.UUID, cai *mmodel.CreateAccountInput) error {
	if libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		return nil
	}

	acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
	if err != nil {
		bizErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", bizErr)

		return bizErr
	}

	if acc.AssetCode != cai.AssetCode {
		bizErr := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", bizErr)

		return bizErr
	}

	return nil
}

// isAuthorizationError checks if the error is an authorization-related error.
func isAuthorizationError(err error) bool {
	var (
		unauthorized pkg.UnauthorizedError
		forbidden    pkg.ForbiddenError
	)

	return errors.As(err, &unauthorized) || errors.As(err, &forbidden)
}

// resolveAccountAlias resolves and validates the account alias.
// Returns provided alias when present and valid; otherwise falls back to generated ID.
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

// determineStatus determines the status of the account. When opts.PendingCRMLink
// is true, the status is forced to PENDING_CRM_LINK regardless of caller input
// (the saga never honors client-supplied status). Otherwise it defaults to
// ACTIVE when unset, preserving the existing behavior for all public callers.
func (uc *UseCase) determineStatus(cai *mmodel.CreateAccountInput, opts accountCreateOptions) mmodel.Status {
	if opts.PendingCRMLink {
		return mmodel.Status{
			Code:        constant.AccountStatusPendingCRMLink,
			Description: cai.Status.Description,
		}
	}

	var status mmodel.Status
	if cai.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cai.Status.Code) {
		status = mmodel.Status{
			Code: constant.AccountStatusActive,
		}
	} else {
		status = cai.Status
	}

	status.Description = cai.Status.Description

	return status
}

// applyAccountingValidations validates the account type against the registered
// account types for this ledger. Only runs when validateAccountType is enabled
// in the ledger settings. External accounts are always allowed.
func (uc *UseCase) applyAccountingValidations(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.apply_accounting_validations")
	defer span.End()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// External accounts bypass all accounting validations.
	if strings.ToLower(accountType) == "external" {
		return nil
	}

	rawSettings, err := uc.LedgerRepo.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings for account type validation", libLog.Err(err))

		return err
	}

	ledgerSettings := mmodel.ParseLedgerSettings(rawSettings)

	if !ledgerSettings.Accounting.ValidateAccountType {
		logger.Log(ctx, libLog.LevelDebug, "Account type validation disabled, skipping",
			libLog.String("ledger_id", ledgerID.String()))

		return nil
	}

	_, err = uc.AccountTypeRepo.FindByKey(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountType, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account type not registered", err)
			logger.Log(ctx, libLog.LevelWarn, "Account type not found in registered types",
				libLog.String("account_type", accountType))

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find account type", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find account type", libLog.Err(err))

		return err
	}

	return nil
}
