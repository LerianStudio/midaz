// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	feeUtils "github.com/LerianStudio/midaz/v4/components/ledger/pkg/fee"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// EstimateFeeCalculation estimate a fee applied in transaction according a specific package
func (uc *UseCase) EstimateFeeCalculation(ctx context.Context, cf *model.FeeEstimate, organizationID uuid.UUID) (*model.FeeCalculate, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	logger.Log(ctx, libLog.LevelInfo, "Trying to estimate fee according a specific package")

	// Defensive nil check for the main input parameter
	if cf == nil {
		logger.Log(ctx, libLog.LevelError, "Invalid input: FeeEstimate is nil")
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidRequestBody, "")
	}

	ctx, span := tracer.Start(ctx, "service.estimate_fee_calculation")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", cf, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	// Validate the existence of a package
	packModel, err := uc.packageRepo.FindByID(ctx, cf.PackageID, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find package by organizationID and package", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to find package by organizationID %v and package %v, Error: %v", organizationID, cf.PackageID, err))

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", reflect.TypeOf(pack.Package{}).Name())
		}

		return nil, err
	}

	// Init process to make the calculation for a fee about a transaction
	logger.Log(ctx, libLog.LevelInfo, "Init the calculation estimate for a transaction")

	feeModel := &model.FeeCalculate{
		LedgerID:    cf.LedgerID,
		Transaction: cf.Transaction,
	}

	validationResult, errValidationSend := transaction.ValidateSendSourceAndDistribute(ctx, feeModel.Transaction, "")
	if errValidationSend != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send struct", errValidationSend)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("error to validate the send struct, Err: %v", errValidationSend))

		return nil, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "FeeEstimate", "transaction send source and distribute are invalid")
	}

	validationResultToSize := len(validationResult.To)
	validationResultFromSize := len(validationResult.From)

	if !feeModel.Transaction.Send.Value.GreaterThanOrEqual(packModel.MinimumAmount) || !feeModel.Transaction.Send.Value.LessThanOrEqual(packModel.MaximumAmount) {
		logMsg := "Transaction value is not between minimum and maximum amount package."

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, errors.New(strings.ToLower(logMsg)))

		logger.Log(ctx, libLog.LevelInfo, logMsg)

		return feeModel, nil
	}

	segCtx := &feeUtils.SegmentContext{
		Ctx:            ctx,
		Resolver:       uc.resolver,
		OrganizationID: organizationID,
		LedgerID:       cf.LedgerID,
	}

	errCalculateFee := feeUtils.CalculateFee(logger, feeModel, packModel, validationResult, uc.defaultCurrency, segCtx)
	if errCalculateFee != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to calculate fee", errCalculateFee)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error calculating fee: %v", errCalculateFee))

		return nil, errCalculateFee
	}

	if len(validationResult.From) == validationResultFromSize &&
		len(validationResult.To) == validationResultToSize {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("No fee is applied for this transaction %v", validationResult))
		return feeModel, nil
	}

	if feeModel.Transaction.Metadata == nil {
		feeModel.Transaction.Metadata = make(map[string]any)
	}

	feeModel.Transaction.Metadata["packageAppliedID"] = cf.PackageID.String()

	return feeModel, nil
}
