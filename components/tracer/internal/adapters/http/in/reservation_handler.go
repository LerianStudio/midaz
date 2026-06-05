// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=reservation_handler.go -destination=mocks/reservation_handler_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v3/components/tracer/pkg/net/http"
)

// ReservationService defines the two-phase reservation operations the handler
// depends on. Interface defined locally per Ring pattern; satisfied by
// *services.ReservationService.
type ReservationService interface {
	Reserve(ctx context.Context, transactionID uuid.UUID, input *model.CheckLimitsInput) (*services.ReserveResult, error)
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
//	@Tags			reservations
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			request		body		ReserveRequest	true	"Reservation request"
//	@Success		201			{object}	ReserveResponse	"Capacity reserved"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		413			{object}	api.ErrorResponse	"Payload too large (exceeds 100KB)"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/reservations [post]
func (h *ReservationHandler) Reserve(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.reservations.reserve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if len(c.Body()) > maxPayloadSize {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.Int("payload_size", len(c.Body())),
			libLog.Int("max_size", maxPayloadSize),
		).Log(ctx, libLog.LevelWarn, "Payload too large")

		libOpentelemetry.HandleSpanError(span, "Payload exceeds size limit", constant.ErrPayloadTooLarge)

		return pkgHTTP.JSONResponse(c, http.StatusRequestEntityTooLarge, libCommons.Response{
			Code:    constant.CodePayloadTooLarge,
			Title:   "Payload Too Large",
			Message: "payload too large: exceeds 100KB limit",
		})
	}

	var request ReserveRequest
	if err := c.BodyParser(&request); err != nil {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to parse request body")

		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequestWithMessage(c, constant.CodeBadRequest, "Bad Request", "invalid request body")
	}

	now := h.clock.Now()
	if err := request.NormalizeAndReserveValidate(now); err != nil {
		logger.With(
			libLog.String("operation", "handler.reservations.reserve"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Request validation failed")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Request validation failed", err)

		return h.handleReservationInputError(c, err)
	}

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "reservation_request", map[string]any{
		"transaction_id":   request.TransactionID.String(),
		"transaction_type": string(request.TransactionType),
		"currency":         request.Currency,
	}, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	result, err := h.service.Reserve(ctx, request.TransactionID, request.ToReserveInput())
	if err != nil {
		return h.handleReservationServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.reservations.reserve"),
		libLog.String("transaction_id", request.TransactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.Int("reservations", len(result.ReservationIDs)),
	).Log(ctx, libLog.LevelInfo, "Reservation processed")

	return pkgHTTP.Created(c, ReserveResponse{
		TransactionID:  request.TransactionID,
		Denied:         result.Denied,
		ReservationIDs: reservationIDsOrEmpty(result.ReservationIDs),
	})
}

// Confirm godoc
//
//	@Summary		Confirm a reservation (phase two — commit)
//	@Description	Commits a held reservation: the amount moves from reserved to current usage. Idempotent — a retry against an already-terminal reservation returns 200.
//	@ID				confirmReservation
//	@Tags			reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Reservation ID (UUID)"	Format(uuid)
//	@Success		200			{object}	ReservationActionResponse	"Reservation confirmed"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/reservations/{id}/confirm [post]
func (h *ReservationHandler) Confirm(c *fiber.Ctx) error {
	return h.terminate(c, "handler.reservations.confirm", string(model.StatusConfirmed), h.service.Confirm)
}

// Release godoc
//
//	@Summary		Release a reservation (phase two — abort)
//	@Description	Returns a held reservation's capacity on an aborted ledger transaction. Idempotent — a retry against an already-terminal reservation returns 200.
//	@ID				releaseReservation
//	@Tags			reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Reservation ID (UUID)"	Format(uuid)
//	@Success		200			{object}	ReservationActionResponse	"Reservation released"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/reservations/{id}/release [post]
func (h *ReservationHandler) Release(c *fiber.Ctx) error {
	return h.terminate(c, "handler.reservations.release", string(model.StatusReleased), h.service.Release)
}

// ConfirmByTransaction godoc
//
//	@Summary		Confirm a transaction's reservations (phase two — commit by transaction)
//	@Description	Confirms EVERY held reservation a transaction holds, addressed by the ledger transaction id. The ledger /commit drives this with only the transaction id. Idempotent — flipped=0 (no reservations, or all already terminal) returns 200.
//	@ID				confirmReservationByTransaction
//	@Tags			reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			transaction_id	path		string	true	"Transaction ID (UUID)"	Format(uuid)
//	@Success		200				{object}	TransactionActionResponse	"Reservations confirmed"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/reservations/transaction/{transaction_id}/confirm [post]
func (h *ReservationHandler) ConfirmByTransaction(c *fiber.Ctx) error {
	return h.terminateByTransaction(c, "handler.reservations.confirm_by_transaction", string(model.StatusConfirmed), h.service.ConfirmByTransaction)
}

// ReleaseByTransaction godoc
//
//	@Summary		Release a transaction's reservations (phase two — abort by transaction)
//	@Description	Releases EVERY held reservation a transaction holds, addressed by the ledger transaction id. The ledger /cancel drives this with only the transaction id. Idempotent — flipped=0 returns 200.
//	@ID				releaseReservationByTransaction
//	@Tags			reservations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			transaction_id	path		string	true	"Transaction ID (UUID)"	Format(uuid)
//	@Success		200				{object}	TransactionActionResponse	"Reservations released"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid path parameter"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/reservations/transaction/{transaction_id}/release [post]
func (h *ReservationHandler) ReleaseByTransaction(c *fiber.Ctx) error {
	return h.terminateByTransaction(c, "handler.reservations.release_by_transaction", string(model.StatusReleased), h.service.ReleaseByTransaction)
}

// terminateByTransaction is the shared by-transaction confirm/release handler body:
// parse the transaction_id path param, invoke the service action, and respond 200
// with the terminal status and the flipped count. The service treats an absent or
// already-terminal transaction as an idempotent no-op (flipped=0), so a 200 here
// covers a fresh transition and a retried no-op alike.
func (h *ReservationHandler) terminateByTransaction(
	c *fiber.Ctx,
	operation string,
	terminalStatus string,
	action func(ctx context.Context, transactionID uuid.UUID) (int, error),
) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	transactionID, err := uuid.Parse(c.Params("transaction_id"))
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid transaction ID format")
	}

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	flipped, err := action(ctx, transactionID)
	if err != nil {
		return h.handleReservationServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("status", terminalStatus),
		libLog.Int("flipped", flipped),
	).Log(ctx, libLog.LevelInfo, "Reservations transitioned by transaction")

	return pkgHTTP.OK(c, TransactionActionResponse{
		TransactionID: transactionID,
		Status:        terminalStatus,
		Flipped:       flipped,
	})
}

// terminate is the shared confirm/release handler body: parse the reservation id
// path param, invoke the service action, and respond 200 with the terminal status.
// The service maps an already-terminal reservation to a nil error (idempotent
// retry), so a 200 here covers both a fresh transition and a retried no-op.
func (h *ReservationHandler) terminate(
	c *fiber.Ctx,
	operation string,
	terminalStatus string,
	action func(ctx context.Context, reservationID uuid.UUID) error,
) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid reservation ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid reservation ID format")
	}

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	if err := action(ctx, reservationID); err != nil {
		return h.handleReservationServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
		libLog.String("status", terminalStatus),
	).Log(ctx, libLog.LevelInfo, "Reservation transition processed")

	return pkgHTTP.OK(c, ReservationActionResponse{
		ReservationID: reservationID,
		Status:        terminalStatus,
	})
}

// reservationInputErrorMappings maps reserve input-validation sentinels to their
// TRC codes and human-readable messages.
var reservationInputErrorMappings = map[error]validationErrorMapping{
	constant.ErrReservationTransactionIDReq:      {code: "TRC-0371", message: "transactionId is required"},
	constant.ErrCheckLimitsInvalidAmount:         {code: "TRC-0222", message: "amount must be positive"},
	constant.ErrCheckLimitsInvalidCurrency:       {code: "TRC-0224", message: "currency must be valid ISO 4217 code (e.g., BRL, USD)"},
	constant.ErrCheckLimitsInvalidAccountID:      {code: "TRC-0227", message: "account is required"},
	constant.ErrValidationRequestIDRequired:      {code: "TRC-0220", message: "requestId is required"},
	constant.ErrValidationInvalidTransactionType: {code: "TRC-0221", message: "transactionType must be one of [CARD, WIRE, PIX, CRYPTO]"},
	constant.ErrValidationAmountNonPositive:      {code: "TRC-0222", message: "amount must be positive"},
	constant.ErrValidationCurrencyRequired:       {code: "TRC-0223", message: "currency is required"},
	constant.ErrValidationInvalidCurrency:        {code: "TRC-0224", message: "currency must be valid ISO 4217 code (e.g., BRL, USD)"},
	constant.ErrValidationTimestampRequired:      {code: "TRC-0225", message: "transactionTimestamp is required"},
	constant.ErrValidationTimestampFuture:        {code: "TRC-0226", message: "transactionTimestamp cannot be in the future"},
	constant.ErrValidationAccountRequired:        {code: "TRC-0227", message: "account is required"},
	constant.ErrValidationTimestampPast:          {code: "TRC-0228", message: "transactionTimestamp is too far in the past (max 24h)"},
}

// handleReservationInputError converts reserve input-validation errors to HTTP 400
// responses with field-aware messages.
func (h *ReservationHandler) handleReservationInputError(c *fiber.Ctx, err error) error {
	for knownErr, mapping := range reservationInputErrorMappings {
		if errors.Is(err, knownErr) {
			return pkgHTTP.BadRequestWithMessage(c, mapping.code, "Validation Error", mapping.message)
		}
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(c.UserContext()) //nolint:dogsled // only logger needed
	logger.With(libLog.String("error.message", err.Error())).Log(c.UserContext(), libLog.LevelWarn, "Unhandled reservation input error")

	return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", "invalid request")
}

// handleReservationServiceError converts reservation service errors to HTTP
// responses. ErrReservationNotFound (a confirm/release against a missing id) maps
// to 404; everything else is a technical failure mapped to 500.
func (h *ReservationHandler) handleReservationServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)

		return pkgHTTP.JSONResponse(c, http.StatusServiceUnavailable, libCommons.Response{
			Code:    constant.CodeContextCancelled,
			Title:   "Service Unavailable",
			Message: "request cancelled",
		})
	case errors.Is(err, constant.ErrReservationNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation not found", err)

		return pkgHTTP.NotFound(c, "TRC-0377", "Not Found", "reservation not found")
	default:
		libOpentelemetry.HandleSpanError(span, "Reservation processing failed", err)

		return pkgHTTP.InternalServerError(c, constant.CodeInternalServer, "Internal Server Error", "reservation processing failed")
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
