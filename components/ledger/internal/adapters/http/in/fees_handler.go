// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
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
//	@Description	Create a fee estimate calculation with input payload
//	@Tags			Fees
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string				false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string				true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			fee					body		model.FeeEstimate	true	"Fee Input"
//	@Success		200					{object}	model.FeeEstimateResponse
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		409					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/estimates [post]
func (handler *FeeHandler) EstimateFeeCalculation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.fee_estimate_calculation")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.FeeEstimate)
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to create a fee estimate with details: %#v", payload))

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", payload, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	feeCalculate, errCreateFee := handler.Service.EstimateFeeCalculation(ctx, payload, organizationID)
	if errCreateFee != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to estimate fee calculation", errCreateFee)

		return feehttp.WithError(c, errCreateFee)
	}

	if feeCalculate.Transaction.Metadata["packageAppliedID"] == nil {
		return commonsHttp.Respond(c, fiber.StatusOK, model.FeeEstimateResponse{
			Message:     "No fee or gratuity rules were found for the given parameters.",
			FeesApplied: nil,
		})
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully estimated fee: %v", feeCalculate))

	// 200 OK is intentional: this is a compute/RPC-style endpoint that performs
	// a calculation without creating a persistent resource.
	return commonsHttp.Respond(c, fiber.StatusOK, model.FeeEstimateResponse{
		Message:     "Successfully estimated fee.",
		FeesApplied: feeCalculate,
	})
}
