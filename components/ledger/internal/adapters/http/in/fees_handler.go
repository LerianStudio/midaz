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
	EstimateFeeCalculation(ctx context.Context, cf *model.FeeEstimate, organizationID uuid.UUID) (*model.FeeEstimateResult, error)
}

// FeeHandler exposes the fee-estimate (dry-run) endpoint over HTTP.
type FeeHandler struct {
	Service FeeService
}

// EstimateFeeCalculation is a method that creates a Fee estimate calculation.
func (handler *FeeHandler) EstimateFeeCalculation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*model.FeeEstimate)

	response, err := handler.estimateFeeCalculation(ctx, organizationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	// 200 OK is intentional: this is a compute/RPC-style endpoint that performs
	// a calculation without creating a persistent resource.
	return commonsHttp.Respond(c, fiber.StatusOK, response)
}

// estimateFeeCalculation is the transport-agnostic core of the fee-estimate op,
// shared by the Fiber wrapper (EstimateFeeCalculation) and the Huma shell. It owns
// the span, service call, nil-result guard, and the applied-vs-no-rules envelope
// selection; the caller (Fiber/Huma) resolves the org id, decodes the payload, and
// renders the returned envelope/error.
func (handler *FeeHandler) estimateFeeCalculation(ctx context.Context, organizationID uuid.UUID, payload *model.FeeEstimate) (model.FeeEstimateResponse, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.fee_estimate_calculation")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", payload.PackageID.String()),
		attribute.String("app.request.ledger_id", payload.LedgerID.String()),
	)

	feeCalculate, errCreateFee := handler.Service.EstimateFeeCalculation(ctx, payload, organizationID)
	if errCreateFee != nil {
		handleSpanByErrorClass(span, "Failed to estimate fee calculation", errCreateFee)

		return model.FeeEstimateResponse{}, errCreateFee
	}

	if feeCalculate == nil {
		return model.FeeEstimateResponse{}, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "Fee")
	}

	if feeCalculate.Transaction.Metadata["packageAppliedID"] == nil {
		return model.FeeEstimateResponse{
			Message:     "No fee or gratuity rules were found for the given parameters.",
			FeesApplied: nil,
		}, nil
	}

	return model.FeeEstimateResponse{
		Message:     "Successfully estimated fee.",
		FeesApplied: feeCalculate,
	}, nil
}
