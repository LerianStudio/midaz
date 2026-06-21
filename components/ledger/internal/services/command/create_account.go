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
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/skip"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateAccount creates an account and metadata, then synchronously creates the default balance.
// The balance is created via the BalancePort interface.
//
//nolint:gocyclo // Validation + creation + metadata + balance orchestration; refactor candidate.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, cai *mmodel.CreateAccountInput, token string) (_ *mmodel.Account, err error) {
	logger, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_account", start, err)
	}()

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

	requireHolder, allowHolderSkip, err := uc.resolveHolderRequirement(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve holder requirement", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve holder requirement", libLog.Err(err))

		return nil, err
	}

	honoredHolderSkip, err := skip.ResolveSkipFor("holder", cai.Skip != nil && cai.Skip.Holder, allowHolderSkip)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Holder skip not permitted", err)
		logger.Log(ctx, libLog.LevelWarn, "Holder skip not permitted", libLog.Err(err))

		return nil, err
	}

	requireHolder = requireHolder && !honoredHolderSkip

	// Record the honored skip as a system observation (not a request input): it
	// reflects what the two-key holder gate actually honored, and it is persisted
	// to the account row below for the durable audit trail.
	span.SetAttributes(attribute.Bool("app.account.holder_check_skipped", honoredHolderSkip))

	if err := uc.applyHolderValidation(ctx, organizationID, requireHolder, cai); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Holder validation failed", err)
		logger.Log(ctx, libLog.LevelWarn, "Holder validation failed", libLog.Err(err))

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
		parsed, parseErr := uuid.Parse(*cai.PortfolioID)
		if parseErr != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidRequestBody, constant.EntityAccount)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid portfolio ID", err)

			return nil, err
		}

		portfolioUUID = parsed

		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioUUID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find portfolio", err)
			logger.Log(ctx, libLog.LevelError, "Error finding portfolio to resolve entity ID", libLog.Err(err))

			return nil, err
		}

		cai.EntityID = &portfolio.EntityID

		logger.Log(ctx, libLog.LevelDebug, "Resolved entity ID from portfolio",
			libLog.String("portfolio_id", portfolioUUID.String()))
	}

	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		parentID, parseErr := uuid.Parse(*cai.ParentAccountID)
		if parseErr != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, constant.EntityAccount)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid parent account ID", err)

			return nil, err
		}

		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, parentID)
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

	holderID := uc.resolveHolderID(organizationID, cai)

	now := time.Now()

	account := &mmodel.Account{
		ID:                 accountID.String(),
		AssetCode:          cai.AssetCode,
		Alias:              alias,
		Name:               cai.Name,
		Type:               cai.Type,
		Blocked:            &blocked,
		ParentAccountID:    cai.ParentAccountID,
		SegmentID:          cai.SegmentID,
		OrganizationID:     organizationID.String(),
		PortfolioID:        cai.PortfolioID,
		LedgerID:           ledgerID.String(),
		EntityID:           cai.EntityID,
		HolderID:           holderID,
		Status:             status,
		HolderCheckSkipped: honoredHolderSkip,
		CreatedAt:          now,
		UpdatedAt:          now,
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

	_, err = uc.CreateDefaultBalance(ctx, balanceInput)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create default balance", libLog.Err(err))

		delErr := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, &portfolioUUID, accountID)
		if delErr != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to delete account during compensation", libLog.Err(delErr))
		}

		return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, constant.EntityAccount)
	}

	uc.emitAccountCreatedEvent(ctx, span, logger, acc)

	metadataDoc, err := uc.CreateOnboardingMetadata(ctx, constant.EntityAccount, acc.ID, cai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account metadata", err)
		logger.Log(ctx, libLog.LevelError, "Error creating account metadata", libLog.Err(err))

		return nil, err
	}

	acc.Metadata = metadataDoc

	return acc, nil
}

// emitAccountCreatedEvent publishes the account.created event for a
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
func (uc *UseCase) emitAccountCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountCreated(acc).ToEmitRequest(tenantID, acc.CreatedAt)
		})
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

// resolveHolderRequirement reads the cached holder gate keys for a ledger in a
// single settings read: whether holder validation is required and whether the
// per-call holder skip is opted in. It uses the cached settings reader port (not
// the uncached LedgerRepo.GetSettings).
//
// A nil reader is an unwired adapter (legitimate; production always wires it) and
// falls back to the permissive default (false, false). A settings-read error fails
// CLOSED — it is propagated so a transient PostgreSQL failure cannot silently
// disable the holder-integrity gate, mirroring applyAccountingValidations.
func (uc *UseCase) resolveHolderRequirement(ctx context.Context, organizationID, ledgerID uuid.UUID) (requireHolder, allowHolderSkip bool, err error) {
	if uc.SettingsReader == nil {
		return false, false, nil
	}

	settings, err := uc.SettingsReader.GetParsedLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		return false, false, err
	}

	return settings.Accounting.RequireHolder, settings.Overrides.AllowHolderSkip, nil
}

// applyHolderValidation enforces the requireHolder gate. When RequireHolder is
// true the account must name a real, existing holder: an absent HolderID is
// rejected with ErrHolderRequired (KYC semantics — the derived self-holder
// default is not an acceptable substitute), and a supplied HolderID must resolve
// to an existing holder or it maps to ErrHolderNotFound. When RequireHolder is
// false the gate is a no-op and the self-holder default applies.
func (uc *UseCase) applyHolderValidation(ctx context.Context, organizationID uuid.UUID, requireHolder bool, cai *mmodel.CreateAccountInput) error {
	if !requireHolder {
		return nil
	}

	if libCommons.IsNilOrEmpty(cai.HolderID) {
		return pkg.ValidateBusinessError(constant.ErrHolderRequired, constant.EntityAccount)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	holderID, err := uuid.Parse(*cai.HolderID)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidRequestBody, constant.EntityAccount)
	}

	exists, err := uc.HolderReader.Exists(ctx, organizationID.String(), holderID)
	if err != nil {
		return err
	}

	if !exists {
		return pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityHolder)
	}

	return nil
}

// resolveHolderID materialises the account's holder_id on the create path.
// When the input supplies a holder, that value wins. Otherwise non-external
// accounts default to the org's deterministic self-holder (derived, no I/O), and
// external accounts stay unowned (nil).
func (uc *UseCase) resolveHolderID(organizationID uuid.UUID, cai *mmodel.CreateAccountInput) *string {
	if !libCommons.IsNilOrEmpty(cai.HolderID) {
		return cai.HolderID
	}

	if strings.ToLower(cai.Type) == "external" {
		return nil
	}

	self := deriveSelfHolderID(organizationID).String()

	return &self
}

// applyAccountingValidations validates the account type against the registered
// account types for this ledger. Only runs when validateAccountType is enabled
// in the ledger settings. External accounts are always allowed.
func (uc *UseCase) applyAccountingValidations(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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
