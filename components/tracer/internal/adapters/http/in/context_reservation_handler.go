// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"errors"
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

type ContextReserveOutput struct {
	Status int
	Body   *tracercontract.ReserveResult
}

type ContextTransactionCompletionInput struct {
	TransactionID string `path:"transaction_id"`
	RawBody       []byte `contentType:"application/json"`
}

type ContextTransactionCompletionOutput struct {
	Body any
}

// ContextReservationHandler replaces Reserve on the existing route. Legacy
// lifecycle bodies remain restricted to the legacy repository's reservations.
// Authentication and tenant resolution must precede all registered handlers.
type ContextReservationHandler struct {
	admission       ContextReserveAdmitter
	completion      ContextReserveCompleter
	completionByID  ContextReserveIDCompleter
	bounds          tracercontract.Limits
	maxBodyBytes    int
	maxReservations int
	legacy          *ReservationHandler
}

func NewContextReservationHandler(admission ContextReserveAdmitter, completion ContextReserveCompleter, completionByID ContextReserveIDCompleter, bounds tracercontract.Limits, maxBodyBytes, maxReservations int) (*ContextReservationHandler, error) {
	if admission == nil || completion == nil || completionByID == nil || maxBodyBytes <= 0 || maxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	return &ContextReservationHandler{admission: admission, completion: completion, completionByID: completionByID, bounds: bounds, maxBodyBytes: maxBodyBytes, maxReservations: maxReservations}, nil
}

func (h *ContextReservationHandler) Reserve(ctx context.Context, input *ReserveInputHuma) (_ *ContextReserveOutput, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.context_reserve")
	defer span.End()
	defer func() {
		if retErr != nil {
			retErr = canonicalContextReservationError(retErr)
			if pkg.IsBusinessError(retErr) {
				libOtel.HandleSpanBusinessErrorEvent(span, "Reserve request rejected", retErr)
			} else {
				libOtel.HandleSpanError(span, "Reserve admission failed", retErr)
			}

			retErr = humaProblem(retErr)
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	if input == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	if len(input.RawBody) > h.maxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	request, err := tracercontract.DecodeReserveJSON(ctx, input.RawBody, h.maxBodyBytes, h.bounds)
	if err != nil {
		return nil, err
	}

	if err := request.Validate(ctx, identity.AssetNamespace, h.bounds); err != nil {
		return nil, err
	}

	result, err := h.admission.Execute(ctx, request)
	if err != nil {
		return nil, err
	}

	if result == nil || result.TransactionID != request.TransactionID || result.Validate(h.maxReservations) != nil {
		return nil, constant.ErrInternalServer
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Context Reserve response ready")

	return &ContextReserveOutput{Status: http.StatusCreated, Body: result}, nil
}

func (h *ContextReservationHandler) ConfirmByTransaction(ctx context.Context, input *ContextTransactionCompletionInput) (*ContextTransactionCompletionOutput, error) {
	return h.complete(ctx, input, model.OperationConfirmed)
}

func (h *ContextReservationHandler) ReleaseByTransaction(ctx context.Context, input *ContextTransactionCompletionInput) (*ContextTransactionCompletionOutput, error) {
	return h.complete(ctx, input, model.OperationReleased)
}

func (h *ContextReservationHandler) complete(ctx context.Context, input *ContextTransactionCompletionInput, status model.ReserveOperationStatus) (_ *ContextTransactionCompletionOutput, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.context_reserve_completion")
	defer span.End()
	defer func() {
		if retErr != nil {
			retErr = canonicalContextReservationError(retErr)
			if pkg.IsBusinessError(retErr) {
				libOtel.HandleSpanBusinessErrorEvent(span, "Completion rejected", retErr)
			} else {
				libOtel.HandleSpanError(span, "Completion failed", retErr)
			}

			retErr = humaProblem(retErr)
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if _, ok := contextutil.GetIntegrationIdentity(ctx); !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	if input == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	if len(input.RawBody) > h.maxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	id, err := uuid.Parse(input.TransactionID)
	if err != nil || id == uuid.Nil {
		return nil, constant.ErrInvalidPathParameter
	}

	if len(bytes.TrimSpace(input.RawBody)) == 0 {
		return h.completeLegacy(ctx, input.TransactionID, status)
	}

	if _, err := tracercontract.DecodeCompletionJSON(ctx, input.RawBody, h.maxBodyBytes); err != nil {
		return nil, err
	}

	result, err := h.completion.ExecuteReport(ctx, id, status)
	if err != nil {
		return nil, err
	}

	if result == nil || result.TransactionID != id || result.Status != string(status) || result.Validate() != nil {
		return nil, constant.ErrInternalServer
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Context completion response ready")

	return &ContextTransactionCompletionOutput{Body: result}, nil
}

func (h *ContextReservationHandler) completeLegacy(ctx context.Context, id string, status model.ReserveOperationStatus) (*ContextTransactionCompletionOutput, error) {
	if h.legacy == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	action := h.legacy.service.ConfirmByTransaction
	if status == model.OperationReleased {
		action = h.legacy.service.ReleaseByTransaction
	}

	result, err := h.legacy.terminateByTransaction(ctx, id, "handler.legacy_reserve_completion", string(status), action)
	if err != nil {
		return nil, err
	}

	return &ContextTransactionCompletionOutput{Body: result}, nil
}

func canonicalContextReservationError(err error) error {
	if errors.Is(err, constant.ErrExpressionEvaluation) {
		return pkg.ValidateBusinessError(constant.ErrExpressionEvaluation, constant.EntityReservation)
	}

	if errors.Is(err, context.Canceled) {
		return pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityReservation)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return pkg.ValidateBusinessError(constant.ErrValidationTimeout, constant.EntityReservation)
	}

	if errors.Is(err, constant.ErrPayloadTooLarge) {
		return pkg.PayloadTooLargeError{Code: constant.ErrPayloadTooLarge.Error(), Title: "Payload Too Large", Message: "Reservation body exceeds the configured limit."}
	}

	if errors.Is(err, constant.ErrInvalidRequestBody) {
		return pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "Invalid reservation contract or context."}
	}

	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		mapped := pkg.ValidateBusinessError(cause, constant.EntityReservation)

		var unavailable pkg.ServiceUnavailableError
		if pkg.IsBusinessError(mapped) || errors.As(mapped, &unavailable) {
			return mapped
		}
	}

	return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "Reservation processing failed."}
}

// RegisterContextReservationRoutes mounts the replacement on the current URL.
// Call once during bootstrap; legacy is retained only for old completion calls.
func RegisterContextReservationRoutes(api huma.API, h *ContextReservationHandler, legacy *ReservationHandler) {
	installContextReservationSchemas(api.OpenAPI().Components.Schemas)

	h.legacy = legacy
	huma.Register(api, huma.Operation{OperationID: "createReservation", Method: http.MethodPost, Path: "/reservations", DefaultStatus: http.StatusCreated, Summary: "Evaluate rules and reserve account capacity", Tags: []string{"Reservations"}, Security: contextReservationSecurity(api), SkipValidateBody: true, MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 409, 413, 503}}, h.Reserve)
	reserveSchema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tracercontract.ReserveRequest](), true, "")

	api.OpenAPI().Paths["/reservations"].Post.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: reserveSchema}}}
	for _, op := range []struct {
		id, path, summary string
		handler           func(context.Context, *ContextTransactionCompletionInput) (*ContextTransactionCompletionOutput, error)
	}{
		{"confirmReservationByTransaction", "/reservations/transaction/{transaction_id}/confirm", "Confirm a known transaction outcome", h.ConfirmByTransaction},
		{"releaseReservationByTransaction", "/reservations/transaction/{transaction_id}/release", "Release a known aborted transaction", h.ReleaseByTransaction},
	} {
		huma.Register(api, huma.Operation{OperationID: op.id, Method: http.MethodPost, Path: op.path, Summary: op.summary, Tags: []string{"Reservations"}, Security: contextReservationSecurity(api), SkipValidateBody: true, MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 409, 413, 503}}, op.handler)
		schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tracercontract.CompletionRequest](), true, "")
		api.OpenAPI().Paths[op.path].Post.RequestBody = &huma.RequestBody{Required: false, Description: "A contractRevision body completes a coordinated operation. An absent body addresses only legacy reservations and additionally requires the configured legacy API-key/Bearer authorization.", Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
		response := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tracercontract.TransactionCompletionResult](), true, "")
		legacyResponse := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[TransactionActionResponse](), true, "")
		api.OpenAPI().Paths[op.path].Post.Responses["200"].Content = map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{OneOf: []*huma.Schema{response, legacyResponse}}}}
	}

	registerContextReservationIDRoutes(api, h)
}

func contextReservationSecurity(api huma.API) []map[string][]string {
	if api.OpenAPI().Components.SecuritySchemes == nil {
		api.OpenAPI().Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	api.OpenAPI().Components.SecuritySchemes["ProducerMTLS"] = &huma.SecurityScheme{Type: "mutualTLS", Description: "Verified producer certificate registered to the integration namespace."}

	return []map[string][]string{{"ProducerMTLS": {}}}
}
