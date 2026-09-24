// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"net/http"
	"reflect"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type ContextReservationCompletionInput struct {
	ID      string `path:"id"`
	RawBody []byte `contentType:"application/json"`
}

func (h *ContextReservationHandler) ConfirmByID(ctx context.Context, input *ContextReservationCompletionInput) (*ContextTransactionCompletionOutput, error) {
	return h.completeReservation(ctx, input, model.OperationConfirmed)
}

func (h *ContextReservationHandler) ReleaseByID(ctx context.Context, input *ContextReservationCompletionInput) (*ContextTransactionCompletionOutput, error) {
	return h.completeReservation(ctx, input, model.OperationReleased)
}

func (h *ContextReservationHandler) completeReservation(ctx context.Context, input *ContextReservationCompletionInput, outcome model.ReserveOperationStatus) (_ *ContextTransactionCompletionOutput, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.complete_reservation")
	defer span.End()
	defer func() {
		if retErr != nil {
			retErr = canonicalContextReservationError(retErr)
			if pkg.IsBusinessError(retErr) {
				libOtel.HandleSpanBusinessErrorEvent(span, "Reservation completion rejected", retErr)
			} else {
				libOtel.HandleSpanError(span, "Reservation completion failed", retErr)
			}

			retErr = humaProblem(retErr)
		}
	}()

	if input == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	id, err := h.validateCompletionAddress(ctx, input.ID, input.RawBody)
	if err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(input.RawBody)) == 0 {
		return h.completeLegacyReservation(ctx, input.ID, outcome)
	}

	if _, err := tracercontract.DecodeCompletionJSON(ctx, input.RawBody, h.maxBodyBytes); err != nil {
		return nil, err
	}

	result, err := h.completionByID.Execute(ctx, id, outcome)
	if err != nil {
		return nil, err
	}

	if result == nil || result.Validate() != nil || result.ReservationID != id || result.Status != string(outcome) {
		return nil, constant.ErrInternalServer
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reservation address completed its operation")

	return &ContextTransactionCompletionOutput{Body: result}, nil
}

func (h *ContextReservationHandler) validateCompletionAddress(ctx context.Context, rawID string, body []byte) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}

	if _, ok := contextutil.GetIntegrationIdentity(ctx); !ok {
		return uuid.Nil, constant.ErrInsufficientPrivileges
	}

	if len(body) > h.maxBodyBytes {
		return uuid.Nil, constant.ErrPayloadTooLarge
	}

	id, err := uuid.Parse(rawID)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, constant.ErrInvalidPathParameter
	}

	return id, nil
}

func (h *ContextReservationHandler) completeLegacyReservation(ctx context.Context, id string, outcome model.ReserveOperationStatus) (*ContextTransactionCompletionOutput, error) {
	if h.legacy == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	action := h.legacy.service.Confirm
	if outcome == model.OperationReleased {
		action = h.legacy.service.Release
	}

	result, err := h.legacy.terminate(ctx, id, "handler.legacy_reservation_completion", string(outcome), action)
	if err != nil {
		return nil, err
	}

	return &ContextTransactionCompletionOutput{Body: result}, nil
}

func registerContextReservationIDRoutes(api huma.API, h *ContextReservationHandler) {
	for _, op := range []struct {
		id, path, summary string
		handler           func(context.Context, *ContextReservationCompletionInput) (*ContextTransactionCompletionOutput, error)
	}{
		{"confirmReservation", "/reservations/{id}/confirm", "Confirm the entire operation addressed by a reservation", h.ConfirmByID},
		{"releaseReservation", "/reservations/{id}/release", "Release the entire operation addressed by a reservation", h.ReleaseByID},
	} {
		huma.Register(api, huma.Operation{OperationID: op.id, Method: http.MethodPost, Path: op.path, Summary: op.summary, Tags: []string{"Reservations"}, Security: contextReservationSecurity(api), SkipValidateBody: true, MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 404, 409, 413, 503}}, op.handler)
		request := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tracercontract.CompletionRequest](), true, "")
		api.OpenAPI().Paths[op.path].Post.RequestBody = &huma.RequestBody{Required: false, Description: "The revision body completes every reservation in the addressed coordinated operation atomically. An absent body addresses only a legacy reservation and requires legacy API-key/Bearer authorization.", Content: map[string]*huma.MediaType{"application/json": {Schema: request}}}
		response := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tracercontract.ReservationCompletionResult](), true, "")
		legacy := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[ReservationActionResponse](), true, "")
		api.OpenAPI().Paths[op.path].Post.Responses["200"].Content = map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{OneOf: []*huma.Schema{response, legacy}}}}
	}
}
