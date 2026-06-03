// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	feeUtils "github.com/LerianStudio/midaz/v3/components/ledger/pkg/fee"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CalculateFee creates a new pack persists data in the repository.
func (uc *UseCase) CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	logger.Log(ctx, libLog.LevelInfo, "Trying to create a fee")

	// Defensive nil check for the main input parameter
	if cf == nil {
		logger.Log(ctx, libLog.LevelError, "Invalid input: FeeCalculate is nil")
		return pkg.ValidateBusinessError(constant.ErrCalculateFee, "")
	}

	ctx, span := tracer.Start(ctx, "service.calculate_fee")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", cf, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	packages, err := uc.packageRepo.FindByOrganizationIDAndLedgerID(ctx, organizationID, cf.LedgerID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to find packages by organizationID %v and ledgerId %v, Error: %v", organizationID, cf.LedgerID, err))
		return err
	}

	if len(packages) == 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Fee is not applied. There is no package associate with organizationID %v and ledgerID %v.", organizationID, cf.LedgerID))
		return nil
	}

	logger.Log(ctx, libLog.LevelInfo, "Init the calculation for a transaction")

	validationResult, errValidationSend := transaction.ValidateSendSourceAndDistribute(ctx, cf.Transaction, "")
	if errValidationSend != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("error to validate the send stuct, Err: %v", errValidationSend))
		return pkg.ValidateBusinessError(constant.ErrValidateDistributeTransactionValue, "")
	}

	sendModel := cf.Transaction.Send
	validationResultToSize := len(validationResult.To)
	validationResultFromSize := len(validationResult.From)

	if len(packages) == 1 {
		return uc.calculateFeeForSinglePackage(ctx, logger, cf, packages[0], sendModel, validationResult, validationResultFromSize, validationResultToSize, organizationID)
	}

	return uc.calculateFeeForMultiplePackages(ctx, logger, cf, packages, sendModel, validationResult, validationResultFromSize, validationResultToSize, organizationID)
}

// calculateFeeForSinglePackage calculate the fee for a single package
func (uc *UseCase) calculateFeeForSinglePackage(
	ctx context.Context,
	logger libLog.Logger,
	cf *model.FeeCalculate,
	feePackage *pack.Package,
	sendModel transaction.Send,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	organizationID uuid.UUID,
) error {
	if !sendModel.Value.GreaterThanOrEqual(feePackage.MinimumAmount) || !sendModel.Value.LessThanOrEqual(feePackage.MaximumAmount) {
		logger.Log(ctx, libLog.LevelInfo, "Only one package found. Fee is not applied because Transaction value is not between maximum and minimum package value.")
		return nil
	}

	segCtx := &feeUtils.SegmentContext{
		Ctx:            ctx,
		MidazClient:    uc.midazClient,
		OrganizationID: organizationID.String(),
		LedgerID:       cf.LedgerID.String(),
	}

	errCalculateFee := feeUtils.CalculateFee(logger, cf, feePackage, validationResult, uc.defaultCurrency, segCtx)
	if errCalculateFee != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error calculating fee: %v", errCalculateFee))
		return errCalculateFee
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, validationResultFromSize, validationResultToSize, feePackage.ID)

	return nil
}

func (uc *UseCase) calculateFeeForMultiplePackages(
	ctx context.Context,
	logger libLog.Logger,
	cf *model.FeeCalculate,
	packages []*pack.Package,
	sendModel transaction.Send,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	organizationID uuid.UUID,
) error {
	packFilter, errFilterPack := feeUtils.FindPackageToCalculateFee(packages, cf.Transaction.Route, cf.SegmentID, sendModel.Value)
	if errFilterPack != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to filter package %v", errFilterPack))
		return pkg.ValidateBusinessError(constant.ErrFilterPackage, "")
	}

	if packFilter == nil {
		logger.Log(ctx, libLog.LevelInfo, "Fee is not applied. The package rules not apply to transaction.")
		return nil
	}

	if !sendModel.Value.GreaterThanOrEqual(packFilter.MinimumAmount) || !sendModel.Value.LessThanOrEqual(packFilter.MaximumAmount) {
		logger.Log(ctx, libLog.LevelInfo, "Fee is not applied. The package rules not apply to transaction.")
		return nil
	}

	segCtx := &feeUtils.SegmentContext{
		Ctx:            ctx,
		MidazClient:    uc.midazClient,
		OrganizationID: organizationID.String(),
		LedgerID:       cf.LedgerID.String(),
	}

	errCalculateFee := feeUtils.CalculateFee(logger, cf, packFilter, validationResult, uc.defaultCurrency, segCtx)
	if errCalculateFee != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error calculating fee: %v", errCalculateFee))
		return errCalculateFee
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, validationResultFromSize, validationResultToSize, packFilter.ID)

	return nil
}

func (uc *UseCase) updateFeeMetadataIfNeeded(
	cf *model.FeeCalculate,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	packageID uuid.UUID,
) {
	feeApplied := len(validationResult.From) != validationResultFromSize ||
		len(validationResult.To) != validationResultToSize
	_, hasExemption := cf.Transaction.Metadata["feeExemption"]

	if feeApplied || hasExemption {
		if cf.Transaction.Metadata == nil {
			cf.Transaction.Metadata = make(map[string]any)
		}

		cf.Transaction.Metadata["packageAppliedID"] = packageID.String()
	}
}
