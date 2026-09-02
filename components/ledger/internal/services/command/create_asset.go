// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateAsset creates an asset and metadata synchronously and ensures an external
// account exists for the asset. If a new external account is created, it also
// creates the default balance for that account.
// The balance is created via the BalancePort interface.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput, token string) (_ *mmodel.Asset, err error) {
	logger, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_asset", start, err)
	}()

	var status mmodel.Status
	if cii.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cii.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	status.Description = cii.Status.Description

	if err := utils.ValidateType(cii.Type); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, constant.EntityAsset)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset type", err)

		return nil, err
	}

	if err := uc.validateAssetCode(ctx, cii.Code); err != nil {
		return nil, err
	}

	if cii.Type == "currency" || cii.Type == "fiat" {
		if err := utils.ValidateCurrency(cii.Code); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset currency", err)

			return nil, err
		}
	}

	_, err = uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset by name or code", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset", libLog.Err(err))

		return nil, err
	}

	asset := &mmodel.Asset{
		Name:           cii.Name,
		Type:           cii.Type,
		Code:           cii.Code,
		Status:         status,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	inst, err := uc.AssetRepo.Create(ctx, asset)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset", libLog.Err(err))

		return nil, err
	}

	uc.emitAssetCreatedEvent(ctx, span, logger, inst)

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityAsset, inst.ID, cii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset metadata", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset metadata", libLog.Err(err))

		return nil, err
	}

	inst.Metadata = metadata

	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve asset external account", err)

		logger.Log(ctx, libLog.LevelError, "Error retrieving asset external account", libLog.Err(err))

		return nil, err
	}

	if len(account) == 0 {
		if err := uc.createAssetExternalAccount(ctx, span, logger, requestID, organizationID, ledgerID, cii.Code, aAlias, aStatusDescription); err != nil {
			return nil, err
		}
	}

	return inst, nil
}

// createAssetExternalAccount materialises the external account that anchors an
// asset and its default balance. It is split out of CreateAsset so the asset
// orchestration stays readable, and because the balance it creates carries the
// same account_blocked inheritance and post-INSERT re-verification as every
// other balance-creation site.
func (uc *UseCase) createAssetExternalAccount(ctx context.Context, span trace.Span, logger libLog.Logger, requestID string, organizationID, ledgerID uuid.UUID, assetCode, alias, statusDescription string) error {
	externalAccountID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate external account ID", err)
		logger.Log(ctx, libLog.LevelError, "Error generating asset external account ID")

		return err
	}

	now := time.Now()

	eAccount := &mmodel.Account{
		ID:              externalAccountID.String(),
		AssetCode:       assetCode,
		Alias:           &alias,
		Name:            "External " + assetCode,
		Type:            "external",
		OrganizationID:  organizationID.String(),
		LedgerID:        ledgerID.String(),
		ParentAccountID: nil,
		SegmentID:       nil,
		PortfolioID:     nil,
		EntityID:        nil,
		Status: mmodel.Status{
			Code:        "external",
			Description: &statusDescription,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	acc, err := uc.AccountRepo.Create(ctx, eAccount)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset external account", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset external account", libLog.Err(err))

		return err
	}

	// Parsed, not MustParse: the repository echoes back the id we generated, so a
	// malformed value means the adapter is broken — which is an error to report,
	// never a reason to take the process down.
	externalAccountUUID, err := uuid.Parse(acc.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse persisted external account id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to parse persisted external account id",
			libLog.String("account_id", acc.ID), libLog.Err(err))

		return err
	}

	// Derived from the account that was just persisted rather than hardcoded, so
	// all four balance-creation sites share one mechanism. External accounts
	// cannot be blocked (guard 0074), so this resolves to false in practice —
	// that is a property of the guard, not an assumption made here.
	externalAccountBlocked := acc.Blocked != nil && *acc.Blocked

	balanceInput := mmodel.CreateBalanceInput{
		RequestID:      requestID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      externalAccountUUID,
		Alias:          alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      assetCode,
		AccountType:    "external",
		AllowSending:   true,
		AllowReceiving: true,
		AccountBlocked: externalAccountBlocked,
	}

	if _, err := uc.CreateDefaultBalance(ctx, balanceInput); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create default balance", libLog.Err(err))

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, constant.EntityAccount)
	}

	// Same post-INSERT re-verification the account path runs. The external guard
	// makes the divergence unreachable here, but the mechanism is uniform across
	// the four creation sites so no site can drift.
	if err := uc.reconcileBalanceAccountBlocked(ctx, organizationID, ledgerID, externalAccountUUID, externalAccountBlocked); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to re-verify external default balance block projection", err)

		return err
	}

	return nil
}

// validateAssetCode checks the provided asset code and maps validation errors to business errors.
func (uc *UseCase) validateAssetCode(ctx context.Context, code string) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.validate_asset_code")
	defer span.End()

	if err := utils.ValidateCode(code); err != nil {
		switch err.Error() {
		case constant.ErrInvalidCodeFormat.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset code", mapped)

			return mapped
		}
	}

	return nil
}

// emitAssetCreatedEvent publishes the asset.created event for a
// successfully persisted asset. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// The persisted database mutation is durable; this helper does not make broker delivery transactional.
//
// Anchor: invoked immediately after AssetRepo.Create succeeds and
// before CreateOnboardingMetadata runs, so a downstream Mongo failure
// cannot mask the event. The implicit external account / default
// balance created later in this use case go through AccountRepo and
// BalancePort directly — NOT through UseCase.CreateAccount — so they
// produce no account.created event.
//
// Wire-format mapping lives in pkg/streaming/events/asset_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAssetCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, a *mmodel.Asset) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AssetCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAssetCreated(a).ToEmitRequest(tenantID, a.CreatedAt)
		})
}
