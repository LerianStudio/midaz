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
//
//	@Summary		Calculate billing
//	@Description	Calculate billing for a given organization, ledger, and period
//	@Tags			Billing Calculate
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string							false	"Request ID for tracing"
//	@Param			organization_id		path		string							true	"Organization ID in UUID format"
//	@Param			billingCalculate	body		model.BillingCalculateRequest	true	"Billing Calculation Input"
//	@Success		200					{object}	model.BillingCalculateResponse	"Billing calculation completed successfully"
//	@Failure		400					{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error					"Forbidden access"
//	@Failure		404					{object}	mmodel.Error					"Organization or ledger not found"
//	@Failure		422					{object}	mmodel.Error					"Business validation failed (e.g. invalid billing period or package type)"
//	@Failure		500					{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/billing/calculate [post]
func (handler *BillingCalculateHandler) CalculateBilling(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.calculate_billing")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.BillingCalculateRequest)
	payload.OrganizationID = organizationID.String()

	span.SetAttributes(
		attribute.String("app.request.ledger_id", payload.LedgerID),
		attribute.String("app.request.period", payload.Period),
		attribute.String("app.request.type", payload.Type),
	)

	if errValidation := validateBillingCalculateRequest(payload); errValidation != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing calculate request validation failed", errValidation)

		return http.WithError(c, errValidation)
	}

	result, errCalc := handler.Service.Calculate(ctx, *payload)
	if errCalc != nil {
		handleSpanByErrorClass(span, "Failed to calculate billing", errCalc)

		return http.WithError(c, errCalc)
	}

	if result == nil {
		return http.WithError(c, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "BillingCalculation"))
	}

	return commonsHttp.Respond(c, fiber.StatusOK, result)
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
