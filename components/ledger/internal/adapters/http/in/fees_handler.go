// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// FeeService defines the fee-estimate operation consumed by the fee handler. In
// the unified binary the fee calculation itself runs in-process via the
// transaction seam, so only the dry-run estimate is exposed over HTTP.
type FeeService interface {
	EstimateFeeCalculation(ctx context.Context, cf *model.FeeEstimate, organizationID uuid.UUID) (*model.FeeCalculate, error)
}

// FeeHandler exposes the fee-estimate (dry-run) endpoint over HTTP.
type FeeHandler struct {
	Service FeeService
}

// EstimateFeeCalculation is a method that creates a Fee estimate calculation.
//
//	@Summary		Create a fee estimate calculation
//	@Description	Performs a dry-run fee estimate for the given payload and returns the fees that would be applied, without creating or persisting any resource.
//	@Tags			Fees
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string				false	"Request ID for tracing"
//	@Param			organization_id		path		string				true	"Organization ID in UUID format"
//	@Param			fee					body		model.FeeEstimate	true	"Fee Input"
//	@Success		200					{object}	model.FeeEstimateResponse	"Successfully estimated fee"
//	@Failure		400					{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error				"Forbidden access"
//	@Failure		404					{object}	mmodel.Error				"Organization or package not found"
//	@Failure		422					{object}	mmodel.Error				"Business validation failed (e.g. invalid fee calculation configuration)"
//	@Failure		500					{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/estimates [post]
func (handler *FeeHandler) EstimateFeeCalculation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.fee_estimate_calculation")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.FeeEstimate)

	span.SetAttributes(
		attribute.String("app.request.package_id", payload.PackageID.String()),
		attribute.String("app.request.ledger_id", payload.LedgerID.String()),
	)

	feeCalculate, errCreateFee := handler.Service.EstimateFeeCalculation(ctx, payload, organizationID)
	if errCreateFee != nil {
		handleSpanByErrorClass(span, "Failed to estimate fee calculation", errCreateFee)

		return http.WithError(c, errCreateFee)
	}

	if feeCalculate == nil {
		return http.WithError(c, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "Fee"))
	}

	if feeCalculate.Transaction.Metadata["packageAppliedID"] == nil {
		return commonsHttp.Respond(c, fiber.StatusOK, model.FeeEstimateResponse{
			Message:     "No fee or gratuity rules were found for the given parameters.",
			FeesApplied: nil,
		})
	}

	// 200 OK is intentional: this is a compute/RPC-style endpoint that performs
	// a calculation without creating a persistent resource.
	return commonsHttp.Respond(c, fiber.StatusOK, model.FeeEstimateResponse{
		Message:     "Successfully estimated fee.",
		FeesApplied: feeCalculate,
	})
}
