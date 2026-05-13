// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateAccount creates an account and metadata, then synchronously creates the default balance.
// The balance is created via the BalancePort interface.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_type", cai.Type),
		attribute.String("app.request.asset_code", cai.AssetCode),
	)

	if err := uc.applyAccountingValidations(ctx, organizationID, ledgerID, cai.Type); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting validations failed", err)
		logger.Log(ctx, libLog.LevelError, "Accounting validations failed", libLog.Err(err))

		return nil, err
	}

	if libCommons.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	status := uc.determineStatus(cai)

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		err := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, constant.EntityAccount)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset", err)

		return nil, err
	}

	var portfolioUUID uuid.UUID

	if libCommons.IsNilOrEmpty(cai.EntityID) && !libCommons.IsNilOrEmpty(cai.PortfolioID) {
		portfolioUUID = uuid.MustParse(*cai.PortfolioID)

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find portfolio", err)
			logger.Log(ctx, libLog.LevelError, "Error finding portfolio to resolve entity ID", libLog.Err(err))

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, constant.EntityAccount)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", err)

			return nil, err
		}

		if acc.AssetCode != cai.AssetCode {
			err := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, constant.EntityAccount)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", err)

			return nil, err
		}
	}

	accountID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate account ID", err)
		logger.Log(ctx, libLog.LevelError, "Error generating account ID", libLog.Err(err))

		return nil, err
	}

	alias, err := uc.resolveAccountAlias(ctx, organizationID, ledgerID, cai, accountID.String())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find account by alias", err)
		return nil, err
	}

	blocked := cai.Blocked != nil && *cai.Blocked

	now := time.Now()

	account := &mmodel.Account{
		ID:              accountID.String(),
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
		logger.Log(ctx, libLog.LevelError, "Error creating account", libLog.Err(err))

		return nil, err
	}

	balanceInput := mmodel.CreateBalanceInput{
		RequestID:      requestID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          *alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cai.AssetCode,
		AccountType:    cai.Type,
		AllowSending:   true,
		AllowReceiving: true,
	}

	_, err = uc.CreateBalanceSync(ctx, balanceInput)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create default balance", libLog.Err(err))

		delErr := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, &portfolioUUID, accountID)
		if delErr != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to delete account during compensation", libLog.Err(delErr))
		}

		if isAuthorizationError(err) {
			return nil, err
		}

		return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, constant.EntityAccount)
	}

	uc.emitAccountCreated(ctx, span, logger, acc)

	metadataDoc, err := uc.CreateOnboardingMetadata(ctx, constant.EntityAccount, acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account metadata", err)
		logger.Log(ctx, libLog.LevelError, "Error creating account metadata", libLog.Err(err))

		return nil, err
	}

	acc.Metadata = metadataDoc

	logger.Log(ctx, libLog.LevelInfo, "Account created synchronously with default balance")

	return acc, nil
}

// emitAccountCreated publishes the account.created event for a
// successfully persisted account. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the default-balance success branch and the
// metadata-write call in CreateAccount, so a downstream Mongo failure
// cannot mask the event and a balance-create rollback cannot leak it.
//
// Wire-format mapping lives in pkg/streaming/events/account_created.go;
// changes to the payload contract belong there, not here. This function
// stays a thin emit-and-log adapter.
func (uc *UseCase) emitAccountCreated(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
	if uc.Streaming == nil {
		return
	}

	event, buildErr := events.NewAccountCreated(acc).ToEvent(
		pkgStreaming.ResolveTenantID(ctx),
		uc.StreamingSource,
		acc.CreatedAt,
	)
	if buildErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build account.created event", buildErr)
		logger.Log(ctx, libLog.LevelWarn, "Skipping account.created emit; build failed", libLog.Err(buildErr))

		return
	}

	if emitErr := uc.Streaming.Emit(ctx, event); emitErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to emit account.created", emitErr)
		logger.Log(ctx, libLog.LevelWarn, "Streaming emit failed for account.created", libLog.Err(emitErr))
	}
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
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountType, constant.EntityAccountType)

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
