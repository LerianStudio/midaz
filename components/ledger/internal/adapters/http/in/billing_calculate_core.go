// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// BillingCalculateUseCase defines the billing-calculation operation consumed by
// the billing-calculate handler. The ledger is taken from the URL path and threaded
// as an explicit parameter, mirroring organizationID on the sibling fee operations.
type BillingCalculateUseCase interface {
	Calculate(ctx context.Context, ledgerID uuid.UUID, request model.BillingCalculateRequest) (*model.BillingCalculateResponse, error)
}

// BillingCalculateHandler exposes the billing-calculation endpoint over HTTP.
type BillingCalculateHandler struct {
	Service BillingCalculateUseCase
}

// calculateBilling is the transport-agnostic core of the calculate op, shared by the
// Huma shell. It owns the span, stamps the path org onto the request, runs the
// handler-level validateBillingCalculateRequest, and calls the service with the path
// ledger; the caller resolves the org and ledger ids, decodes the payload, and renders
// the response/error.
func (handler *BillingCalculateHandler) calculateBilling(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *model.BillingCalculateRequest) (*model.BillingCalculateResponse, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.calculate_billing")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload.OrganizationID = organizationID.String()

	span.SetAttributes(
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.period", payload.Period),
		attribute.String("app.request.type", payload.Type),
	)

	if errValidation := validateBillingCalculateRequest(payload); errValidation != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing calculate request validation failed", errValidation)

		return nil, errValidation
	}

	result, errCalc := handler.Service.Calculate(ctx, ledgerID, *payload)
	if errCalc != nil {
		handleSpanByErrorClass(span, "Failed to calculate billing", errCalc)

		return nil, errCalc
	}

	if result == nil {
		return nil, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "BillingCalculation")
	}

	return result, nil
}

// validateBillingCalculateRequest validates the billing calculate request payload.
func validateBillingCalculateRequest(req *model.BillingCalculateRequest) error {
	if req.OrganizationID == "" {
		return feeerrors.ValidateBusinessError(feeconstant.ErrFeeInvalidHeaderParameter, "BillingCalculation", "organizationId")
	}

	if err := validateBillingPeriod(req.Period); err != nil {
		return err
	}

	if req.Type != "" && req.Type != model.BillingPackageTypeVolume && req.Type != model.BillingPackageTypeMaintenance {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidBillingPackageType, "BillingCalculation")
	}

	return nil
}

// validateBillingPeriod checks that the period is a valid YYYY-MM, YYYY-Www, or YYYY-MM-DD date.
func validateBillingPeriod(period string) error {
	if period == "" {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidBillingPeriod, "BillingCalculation",
			"period is required")
	}

	if _, err := time.Parse("2006-01-02", period); err == nil {
		return nil
	}

	if _, _, ok := model.ParseWeeklyPeriod(period); ok {
		return nil
	}

	if model.LooksLikeWeeklyPeriod(period) {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidBillingPeriod, "BillingCalculation",
			fmt.Sprintf("period %q is not a valid ISO week (week does not exist in that year)", period))
	}

	if _, err := time.Parse("2006-01", period); err == nil {
		return nil
	}

	return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidBillingPeriod, "BillingCalculation",
		"period must be a valid date in YYYY-MM, YYYY-Www, or YYYY-MM-DD format")
}
