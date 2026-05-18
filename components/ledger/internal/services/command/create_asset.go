// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// CreateAsset creates an asset and metadata synchronously and ensures an external
// account exists for the asset. If a new external account is created, it also
// creates the default balance for that account.
// The balance is created via the BalancePort interface.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput, token string) (*mmodel.Asset, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to create asset organizationID=%s ledgerID=%s code=%s", organizationID.String(), ledgerID.String(), cii.Code))

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
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset type", err)

		return nil, err
	}

	if err := uc.validateAssetCode(ctx, cii.Code); err != nil {
		return nil, err
	}

	if cii.Type == "currency" {
		if err := utils.ValidateCurrency(cii.Code); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset currency", err)

			return nil, err
		}
	}

	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset by name or code", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating asset: %v", err))

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

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating asset: %v", err))

		return nil, err
	}

	uc.emitAssetCreatedEvent(ctx, span, logger, inst)

	metadata, err := uc.CreateOnboardingMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating asset metadata: %v", err))

		return nil, err
	}

	inst.Metadata = metadata

	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve asset external account", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error retrieving asset external account: %v", err))

		return nil, err
	}

	if len(account) == 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Creating external account for asset: %s", cii.Code))

		externalAccountID, err := libCommons.GenerateUUIDv7()
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate external account ID", err)
			logger.Log(ctx, libLog.LevelError, "Error generating asset external account ID")

			return nil, err
		}

		eAccount := &mmodel.Account{
			ID:              externalAccountID.String(),
			AssetCode:       cii.Code,
			Alias:           &aAlias,
			Name:            "External " + cii.Code,
			Type:            "external",
			OrganizationID:  organizationID.String(),
			LedgerID:        ledgerID.String(),
			ParentAccountID: nil,
			SegmentID:       nil,
			PortfolioID:     nil,
			EntityID:        nil,
			Status: mmodel.Status{
				Code:        "external",
				Description: &aStatusDescription,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		acc, err := uc.AccountRepo.Create(ctx, eAccount)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset external account", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating asset external account: %v", err))

			return nil, err
		}

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("External account created for asset %s with alias %s", cii.Code, aAlias))

		balanceInput := mmodel.CreateBalanceInput{
			RequestID:      requestID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(acc.ID),
			Alias:          aAlias,
			Key:            constant.DefaultBalanceKey,
			AssetCode:      cii.Code,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
		}

		_, err = uc.CreateDefaultBalance(ctx, balanceInput)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create default balance: %v", err))

			var (
				unauthorized pkg.UnauthorizedError
				forbidden    pkg.ForbiddenError
			)

			if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
				return nil, err
			}

			return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
		}

		logger.Log(ctx, libLog.LevelInfo, "External account default balance created")
	}

	return inst, nil
}

// validateAssetCode checks the provided asset code and maps validation errors to business errors.
func (uc *UseCase) validateAssetCode(ctx context.Context, code string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.validate_asset_code")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Validating asset code: %s", code))

	if err := utils.ValidateCode(code); err != nil {
		switch err.Error() {
		case constant.ErrInvalidCodeFormat.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset code", mapped)

			return mapped
		}
	}

	return nil
}

// emitAssetCreatedEvent publishes the asset.created event for a
// successfully persisted asset. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
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
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AssetCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAssetCreated(a).ToEmitRequest(tenantID, a.CreatedAt)
		})
}
