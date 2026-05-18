// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (uc *UseCase) ResolveBankAccount(ctx context.Context, input *mmodel.ResolveBankAccountInput) (*mmodel.ResolveAliasResponse, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.resolve_bank_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.Bool("app.request.has_document", input != nil && input.Document != ""),
		attribute.Bool("app.request.has_banking_details", input != nil),
	)

	if input == nil || input.Document == "" || input.BankingDetails.BankID == "" || input.BankingDetails.Branch == "" || input.BankingDetails.Account == "" || input.BankingDetails.Type == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingFieldsInRequest, cn.EntityAlias, "document, bankingDetails.bankId, bankingDetails.branch, bankingDetails.account, bankingDetails.type")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Invalid bank account resolver input", err)

		return nil, err
	}

	aliases, err := uc.AliasRepo.ResolveBankAccount(ctx, input)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to resolve bank account", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve bank account", libLog.Err(err))

		return nil, err
	}

	return resolveOneAlias(ctx, span, aliases, true)
}

func (uc *UseCase) ResolveAccount(ctx context.Context, input *mmodel.ResolveAccountInput) (*mmodel.ResolveAliasResponse, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.resolve_account")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	if input == nil || input.AccountID == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingFieldsInRequest, cn.EntityAlias, "accountId")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account resolver input", err)

		return nil, err
	}

	accountID, err := uuid.Parse(input.AccountID)
	if err != nil || accountID == uuid.Nil {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidQueryParameter, cn.EntityAlias, "accountId")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account id", validationErr)

		return nil, validationErr
	}

	aliases, err := uc.AliasRepo.ResolveAccount(ctx, accountID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to resolve account", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve account", libLog.Err(err))

		return nil, err
	}

	return resolveOneAlias(ctx, span, aliases, false)
}

func resolveOneAlias(ctx context.Context, span trace.Span, aliases []*mmodel.Alias, requireBankingProof bool) (*mmodel.ResolveAliasResponse, error) {
	switch len(aliases) {
	case 0:
		err := pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Alias resolver match not found", err)

		return nil, err
	case 1:
		response := aliasToResolveResponse(aliases[0])
		if err := validateResolveAliasResponse(response, requireBankingProof); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Invalid resolver index row", err)

			return nil, err
		}

		return response, nil
	default:
		err := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
		_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
		_, duplicateSpan := tracer.Start(ctx, "service.resolve_alias.duplicate_active_matches")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(duplicateSpan, "Duplicate active alias resolver matches", err)
		duplicateSpan.End()

		return nil, err
	}
}

func validateResolveAliasResponse(response *mmodel.ResolveAliasResponse, requireBankingProof bool) error {
	if response == nil || response.ID == "" || response.OrganizationID == "" || response.LedgerID == "" || response.AccountID == "" || response.HolderID == "" || response.HolderDocument == "" {
		return pkg.ValidateBusinessError(cn.ErrInternalServer, cn.EntityAlias)
	}

	if requireBankingProof && (response.BankingDetails.BankID == "" || response.BankingDetails.Branch == "" || response.BankingDetails.Account == "" || response.BankingDetails.Type == "") {
		return pkg.ValidateBusinessError(cn.ErrInternalServer, cn.EntityAlias)
	}

	return nil
}

func aliasToResolveResponse(alias *mmodel.Alias) *mmodel.ResolveAliasResponse {
	if alias == nil {
		return nil
	}

	response := &mmodel.ResolveAliasResponse{
		ID:             stringValueFromUUID(alias.ID),
		OrganizationID: stringValue(alias.OrganizationID),
		LedgerID:       stringValue(alias.LedgerID),
		AccountID:      stringValue(alias.AccountID),
		HolderID:       stringValueFromUUID(alias.HolderID),
		HolderDocument: stringValue(alias.Document),
	}

	if alias.BankingDetails != nil {
		response.BankingDetails = mmodel.ResolveAliasBankingDetailsResponse{
			BankID:  stringValue(alias.BankingDetails.BankID),
			Branch:  stringValue(alias.BankingDetails.Branch),
			Account: stringValue(alias.BankingDetails.Account),
			Type:    stringValue(alias.BankingDetails.Type),
		}
	}

	return response
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func stringValueFromUUID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}

	return value.String()
}

func (uc *UseCase) BackfillBankAccountIndex(ctx context.Context, dryRun bool) (*mmodel.BankAccountIndexBackfillReport, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.backfill_bank_account_index")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.Bool("app.request.dry_run", dryRun),
	)

	report, err := uc.AliasRepo.BackfillBankAccountIndex(ctx, dryRun)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to backfill bank account index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to backfill bank account index", libLog.Err(err))

		return nil, err
	}

	return report, nil
}
