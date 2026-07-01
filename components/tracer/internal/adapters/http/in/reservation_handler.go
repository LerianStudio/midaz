// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=reservation_handler.go -destination=mocks/reservation_handler_service_mock.go -package=mocks

import (
	"context"
	"encoding/json"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// ReservationService defines the two-phase reservation operations the handler
// depends on. Interface defined locally per Ring pattern; satisfied by
// *services.ReservationService.
type ReservationService interface {
	Reserve(ctx context.Context, transactionID uuid.UUID, input *model.CheckLimitsInput, longLived bool) (*services.ReserveResult, error)
	Confirm(ctx context.Context, reservationID uuid.UUID) error
	Release(ctx context.Context, reservationID uuid.UUID) error
	ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error)
	ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error)
}

// ReservationHandler handles HTTP requests for the two-phase reservation API.
type ReservationHandler struct {
	service ReservationService
	clock   clock.Clock
}

// NewReservationHandler creates a new reservation handler.
// Returns an error if service or clk is nil.
func NewReservationHandler(service ReservationService, clk clock.Clock) (*ReservationHandler, error) {
	if service == nil {
		return nil, errors.New("nil ReservationService passed to NewReservationHandler")
	}

	if clk == nil {
		return nil, errors.New("nil Clock passed to NewReservationHandler")
	}

	return &ReservationHandler{
		service: service,
		clock:   clk,
	}, nil
}

// Reserve godoc
//
//	@Summary		Reserve transaction capacity (phase one)
//	@Description	Holds limit capacity for a ledger transaction without committing it. Returns one reservation id per counter-backed limit, or denied=true when a limit would be exceeded.
//	@ID				createReservation
//	@Tags			Reservations
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			request		body		ReserveRequest	true	"Reservation request"
//	@Success		201			{object}	ReserveResponse	"Capacity reserved"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		413			{object}	api.ErrorResponse	"Payload too large (exceeds 100KB)"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503			{object}	api.ErrorResponse	"Service unavailable (context cancelled)"
//	@Router			/v1/reservations [post]
func (h *ReservationHandler) Reserve(c *fiber.Ctx) error {
	response, err := h.reserve(c.UserContext(), c.Body())
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	return pkgHTTP.Created(c, response)
}

// reserve is the transport-agnostic core of the reserve operation shared by the
// Fiber method (Reserve) and the Huma func (ReserveHuma). It owns the span, the
// payload-size guard, the imperative json.Unmarshal + NormalizeAndReserveValidate,
// the service call, and the success log — and CANONICALIZES every error before
// returning it (parse/validation errors are already canonical; the service error
// runs through classifyReservationServiceError here). So both callers render the
// returned error the same way — Fiber via pkgHTTP.WithError, Huma via humaProblem —
// and the two transports emit field/status/code/type-identical envelopes. The
// payload-size guard is preserved from the Fiber path: Huma has no Fiber-style body
// limit, so the check must live in the core.
func (h *ReservationHandler) reserve(ctx context.Context, rawBody []byte) (*ReserveResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.reservations.reserve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if len(rawBody) > maxPayloadSize {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.Int("payload_size", len(rawBody)),
			libLog.Int("max_size", maxPayloadSize),
		).Log(ctx, libLog.LevelWarn, "Payload too large")

		libOpentelemetry.HandleSpanError(span, "Payload exceeds size limit", constant.ErrPayloadTooLarge)

		return nil, pkg.ValidateBusinessError(constant.ErrPayloadTooLarge, constant.EntityReservation)
	}

	var request ReserveRequest
	if err := json.Unmarshal(rawBody, &request); err != nil {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to parse request body")

		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return nil, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."}
	}

	now := h.clock.Now()
	if err := request.NormalizeAndReserveValidate(now); err != nil {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Request validation failed")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Request validation failed", err)

		return nil, pkg.ValidateBusinessError(err, constant.EntityReservation)
	}

	span.SetAttributes(
		attribute.String("app.request.transaction_id", request.TransactionID.String()),
		attribute.String("app.request.transaction_type", string(request.TransactionType)),
		attribute.String("app.request.currency", request.Currency),
	)

	result, err := h.service.Reserve(ctx, request.TransactionID, request.ToReserveInput(), request.LongLived)
	if err != nil {
		return nil, classifyReservationServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.reservations.reserve"),
		libLog.String("transaction_id", request.TransactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.Int("reservations", len(result.ReservationIDs)),
	).Log(ctx, libLog.LevelDebug, "Reservation processed")

	return &ReserveResponse{
		TransactionID:  request.TransactionID,
		Denied:         result.Denied,
		ReservationIDs: reservationIDsOrEmpty(result.ReservationIDs),
	}, nil
}

// Confirm godoc
//
//	@Summary		Confirm a reservation (phase two — commit)
//	@Description	Commits a held reservation: the amount moves from reserved to current usage. Idempotent — a retry against an already-terminal reservation returns 200.
//	@ID				confirmReservation
//	@Tags			Reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Reservation ID (UUID)"	Format(uuid)
//	@Success		200			{object}	ReservationActionResponse	"Reservation confirmed"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Reservation not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503			{object}	api.ErrorResponse	"Service unavailable (context cancelled)"
//	@Router			/v1/reservations/{id}/confirm [post]
func (h *ReservationHandler) Confirm(c *fiber.Ctx) error {
	response, err := h.terminate(c.UserContext(), c.Params("id"), "handler.reservations.confirm", string(model.StatusConfirmed), h.service.Confirm)
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	return pkgHTTP.OK(c, response)
}

// Release godoc
//
//	@Summary		Release a reservation (phase two — abort)
//	@Description	Returns a held reservation's capacity on an aborted ledger transaction. Idempotent — a retry against an already-terminal reservation returns 200.
//	@ID				releaseReservation
//	@Tags			Reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Reservation ID (UUID)"	Format(uuid)
//	@Success		200			{object}	ReservationActionResponse	"Reservation released"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Reservation not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503			{object}	api.ErrorResponse	"Service unavailable (context cancelled)"
//	@Router			/v1/reservations/{id}/release [post]
func (h *ReservationHandler) Release(c *fiber.Ctx) error {
	response, err := h.terminate(c.UserContext(), c.Params("id"), "handler.reservations.release", string(model.StatusReleased), h.service.Release)
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	return pkgHTTP.OK(c, response)
}

// ConfirmByTransaction godoc
//
//	@Summary		Confirm a transaction's reservations (phase two — commit by transaction)
//	@Description	Confirms EVERY held reservation a transaction holds, addressed by the ledger transaction id. The ledger /commit drives this with only the transaction id. Idempotent — flipped=0 (no reservations, or all already terminal) returns 200.
//	@ID				confirmReservationByTransaction
//	@Tags			Reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			transaction_id	path		string	true	"Transaction ID (UUID)"	Format(uuid)
//	@Success		200				{object}	TransactionActionResponse	"Reservations confirmed"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503				{object}	api.ErrorResponse	"Service unavailable (context cancelled)"
//	@Router			/v1/reservations/transaction/{transaction_id}/confirm [post]
func (h *ReservationHandler) ConfirmByTransaction(c *fiber.Ctx) error {
	response, err := h.terminateByTransaction(c.UserContext(), c.Params("transaction_id"), "handler.reservations.confirm_by_transaction", string(model.StatusConfirmed), h.service.ConfirmByTransaction)
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	return pkgHTTP.OK(c, response)
}

// ReleaseByTransaction godoc
//
//	@Summary		Release a transaction's reservations (phase two — abort by transaction)
//	@Description	Releases EVERY held reservation a transaction holds, addressed by the ledger transaction id. The ledger /cancel drives this with only the transaction id. Idempotent — flipped=0 returns 200.
//	@ID				releaseReservationByTransaction
//	@Tags			Reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			transaction_id	path		string	true	"Transaction ID (UUID)"	Format(uuid)
//	@Success		200				{object}	TransactionActionResponse	"Reservations released"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503				{object}	api.ErrorResponse	"Service unavailable (context cancelled)"
//	@Router			/v1/reservations/transaction/{transaction_id}/release [post]
func (h *ReservationHandler) ReleaseByTransaction(c *fiber.Ctx) error {
	response, err := h.terminateByTransaction(c.UserContext(), c.Params("transaction_id"), "handler.reservations.release_by_transaction", string(model.StatusReleased), h.service.ReleaseByTransaction)
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	return pkgHTTP.OK(c, response)
}

// terminateByTransaction is the shared by-transaction confirm/release handler body:
// parse the transaction_id path param, invoke the service action, and respond 200
// with the terminal status and the flipped count. The service treats an absent or
// already-terminal transaction as an idempotent no-op (flipped=0), so a 200 here
// covers a fresh transition and a retried no-op alike.
func (h *ReservationHandler) terminateByTransaction(
	ctx context.Context,
	txIDParam string,
	operation string,
	terminalStatus string,
	action func(ctx context.Context, transactionID uuid.UUID) (int, error),
) (*TransactionActionResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	transactionID, err := uuid.Parse(txIDParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityReservation, "transaction_id")
	}

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	flipped, err := action(ctx, transactionID)
	if err != nil {
		return nil, classifyReservationServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("status", terminalStatus),
		libLog.Int("flipped", flipped),
	).Log(ctx, libLog.LevelDebug, "Reservations transitioned by transaction")

	return &TransactionActionResponse{
		TransactionID: transactionID,
		Status:        terminalStatus,
		Flipped:       flipped,
	}, nil
}

// terminate is the shared confirm/release handler body: parse the reservation id
// path param, invoke the service action, and respond 200 with the terminal status.
// The service maps an already-terminal reservation to a nil error (idempotent
// retry), so a 200 here covers both a fresh transition and a retried no-op.
func (h *ReservationHandler) terminate(
	ctx context.Context,
	idParam string,
	operation string,
	terminalStatus string,
	action func(ctx context.Context, reservationID uuid.UUID) error,
) (*ReservationActionResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservationID, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid reservation ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityReservation, "id")
	}

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	if err := action(ctx, reservationID); err != nil {
		return nil, classifyReservationServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
		libLog.String("status", terminalStatus),
	).Log(ctx, libLog.LevelDebug, "Reservation transition processed")

	return &ReservationActionResponse{
		ReservationID: reservationID,
		Status:        terminalStatus,
	}, nil
}

// classifyReservationServiceError maps a raw reservation service error to its
// canonical Midaz error, attributing the span, WITHOUT rendering. It is the single
// classification the Fiber wrappers (which render via pkgHTTP.WithError) and the
// Huma funcs (humaProblem -> *pkgHTTP.Detail) both consume, so both transports emit
// field/status/code/type-identical envelopes. ErrReservationNotFound (a
// confirm/release against a missing id) maps to 404; everything else is a technical
// failure mapped to 500.
func classifyReservationServiceError(span trace.Span, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)

		return pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityReservation)
	case errors.Is(err, constant.ErrReservationNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation not found", err)

		return pkg.ValidateBusinessError(constant.ErrReservationNotFound, constant.EntityReservation)
	default:
		libOpentelemetry.HandleSpanError(span, "Reservation processing failed", err)

		return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."}
	}
}

// reservationIDsOrEmpty returns a non-nil slice so the JSON body serializes
// reservationIds as [] rather than null on the denied / no-counter-limit paths.
func reservationIDsOrEmpty(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}

	return ids
}
