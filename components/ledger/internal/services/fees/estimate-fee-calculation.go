// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	feeUtils "github.com/LerianStudio/midaz/v4/components/ledger/pkg/fee"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// EstimateFeeCalculation estimate a fee applied in transaction according a specific package
func (uc *UseCase) EstimateFeeCalculation(ctx context.Context, cf *model.FeeEstimate, organizationID uuid.UUID) (*model.FeeCalculate, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	// Defensive nil check for the main input parameter
	if cf == nil {
		return nil, pkg.ValidationError{
			Code:    constant.ErrInvalidRequestBody.Error(),
			Title:   "Invalid Request Body",
			Message: "The request body is required. Please check the documentation and try again.",
		}
	}

	ctx, span := tracer.Start(ctx, "service.estimate_fee_calculation")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	span.SetAttributes(
		attribute.String("app.request.package_id", cf.PackageID.String()),
		attribute.String("app.request.ledger_id", cf.LedgerID.String()),
	)

	// Validate the existence of a package
	packModel, err := uc.packageRepo.FindByID(ctx, cf.PackageID, organizationID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Package not found", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find package by organizationID and package", err)

		return nil, err
	}

	feeModel := &model.FeeCalculate{
		LedgerID:    cf.LedgerID,
		Transaction: cf.Transaction,
	}

	validationResult, errValidationSend := transaction.ValidateSendSourceAndDistribute(ctx, feeModel.Transaction, "")
	if errValidationSend != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send struct", errValidationSend)

		return nil, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "FeeEstimate", "transaction send source and distribute are invalid")
	}

	validationResultToSize := len(validationResult.To)
	validationResultFromSize := len(validationResult.From)

	if !feeModel.Transaction.Send.Value.GreaterThanOrEqual(packModel.MinimumAmount) || !feeModel.Transaction.Send.Value.LessThanOrEqual(packModel.MaximumAmount) {
		const outOfRangeMsg = "Transaction value is not between minimum and maximum amount package."

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, outOfRangeMsg, errors.New(strings.ToLower(outOfRangeMsg)))

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

		return nil, errCalculateFee
	}

	if len(validationResult.From) == validationResultFromSize &&
		len(validationResult.To) == validationResultToSize {
		return feeModel, nil
	}

	if feeModel.Transaction.Metadata == nil {
		feeModel.Transaction.Metadata = make(map[string]any)
	}

	feeModel.Transaction.Metadata["packageAppliedID"] = cf.PackageID.String()

	return feeModel, nil
}
