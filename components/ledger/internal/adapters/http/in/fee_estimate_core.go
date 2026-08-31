// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// FeeService defines the fee-estimate operation consumed by the fee handler. In
// the unified binary the fee calculation itself runs in-process via the
// transaction seam, so only the dry-run estimate is exposed over HTTP.
type FeeService interface {
	EstimateFeeCalculation(ctx context.Context, cf *model.FeeEstimate, organizationID, ledgerID uuid.UUID) (*model.FeeEstimateResult, error)
}

// FeeHandler exposes the fee-estimate (dry-run) endpoint over HTTP.
type FeeHandler struct {
	Service FeeService
}

// estimateFeeCalculation is the transport-agnostic core of the fee-estimate op,
// shared by the Fiber wrapper (EstimateFeeCalculation) and the Huma shell. It owns
// the span, service call, nil-result guard, and the applied-vs-no-rules envelope
// selection; the caller (Fiber/Huma) resolves the org and ledger ids from the path,
// decodes the payload, and renders the returned envelope/error.
func (handler *FeeHandler) estimateFeeCalculation(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *model.FeeEstimate) (model.FeeEstimateResponse, error) {
	if err := ctx.Err(); err != nil {
		return model.FeeEstimateResponse{}, err
	}

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.fee_estimate_calculation")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", payload.PackageID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	feeCalculate, errCreateFee := handler.Service.EstimateFeeCalculation(ctx, payload, organizationID, ledgerID)
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
