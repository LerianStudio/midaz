// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// BillingCalculateUseCase defines the billing-calculation operation consumed by
// the billing-calculate handler.
type BillingCalculateUseCase interface {
	Calculate(ctx context.Context, request model.BillingCalculateRequest) (*model.BillingCalculateResponse, error)
}

// BillingCalculateHandler exposes the billing-calculation endpoint over HTTP.
type BillingCalculateHandler struct {
	Service BillingCalculateUseCase
}

// CalculateBilling performs a billing calculation for the given request.
func (handler *BillingCalculateHandler) CalculateBilling(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*model.BillingCalculateRequest)
	if !ok || payload == nil {
		return http.WithError(c, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "BillingCalculation"))
	}

	result, err := handler.calculateBilling(ctx, organizationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// calculateBilling is the transport-agnostic core of the calculate op, shared by the
// Fiber wrapper (CalculateBilling) and the Huma shell. It owns the span, stamps the
// path org onto the request, runs the handler-level validateBillingCalculateRequest,
// and calls the service; the caller resolves the org id, decodes the payload, and
// renders the response/error.
func (handler *BillingCalculateHandler) calculateBilling(ctx context.Context, organizationID uuid.UUID, payload *model.BillingCalculateRequest) (*model.BillingCalculateResponse, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.calculate_billing")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload.OrganizationID = organizationID.String()

	span.SetAttributes(
		attribute.String("app.request.ledger_id", payload.LedgerID),
		attribute.String("app.request.period", payload.Period),
		attribute.String("app.request.type", payload.Type),
	)

	if errValidation := validateBillingCalculateRequest(payload); errValidation != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing calculate request validation failed", errValidation)

		return nil, errValidation
	}

	result, errCalc := handler.Service.Calculate(ctx, *payload)
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

	if req.LedgerID == "" {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, "BillingCalculation", "ledgerId")
	}

	if _, err := uuid.Parse(req.LedgerID); err != nil {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, "BillingCalculation", "ledgerId")
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
