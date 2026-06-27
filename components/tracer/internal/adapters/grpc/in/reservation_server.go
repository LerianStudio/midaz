// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package in hosts the tracer's inbound gRPC adapters. The reservation server
// is the gRPC face of the SAME two-phase reservation use case the REST handler
// drives (components/tracer/internal/adapters/http/in/reservation_handler.go):
// it maps the generated proto messages to the domain inputs, delegates to the
// identical *services.ReservationService, and maps the results back. The
// business logic is never duplicated — both transports converge on one service.
package in

//go:generate mockgen -source=reservation_server.go -destination=mocks/reservation_server_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// ReservationService is the two-phase reservation use case the gRPC server
// delegates to. It is the SAME interface the REST handler depends on
// (reservation_handler.go), satisfied by *services.ReservationService, so the
// two transports cannot drift apart in behavior.
type ReservationService interface {
	Reserve(ctx context.Context, transactionID uuid.UUID, input *model.CheckLimitsInput, longLived bool) (*services.ReserveResult, error)
	Confirm(ctx context.Context, reservationID uuid.UUID) error
	Release(ctx context.Context, reservationID uuid.UUID) error
	ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error)
	ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error)
}

// ReservationServer is the gRPC ReservationService implementation. It embeds the
// generated UnimplementedReservationServiceServer for forward compatibility and
// delegates every RPC to the shared use case.
type ReservationServer struct {
	reservationv1.UnimplementedReservationServiceServer

	service ReservationService
	clock   clock.Clock
}

// NewReservationServer constructs a gRPC reservation server. clk drives the
// reserve timestamp-window check (injected for MOCK_TIME determinism in tests),
// mirroring the REST handler's clock dependency. Returns an error if service or
// clk is nil.
func NewReservationServer(service ReservationService, clk clock.Clock) (*ReservationServer, error) {
	if service == nil {
		return nil, errors.New("nil ReservationService passed to NewReservationServer")
	}

	if clk == nil {
		return nil, errors.New("nil Clock passed to NewReservationServer")
	}

	return &ReservationServer{
		service: service,
		clock:   clk,
	}, nil
}

// Reserve holds limit capacity for a ledger transaction (phase one). The proto
// request is mapped to the same model.ValidationRequest the REST path builds,
// normalized and validated with the relaxed reserve rules, then converted to the
// CheckLimitsInput the use case resolves against — so the gRPC and REST inputs
// are identical. A limit-exceeded decision comes back as a normal result with
// denied=true (NOT an error); only validation and technical failures map to a
// gRPC status error.
func (s *ReservationServer) Reserve(ctx context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.reservations.reserve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	transactionID, err := uuid.Parse(req.GetTransactionId())
	if err != nil || transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction id", constant.ErrReservationTransactionIDReq)
		return nil, status.Error(codes.InvalidArgument, constant.ErrReservationTransactionIDReq.Error())
	}

	validationReq, err := s.toValidationRequest(req)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid reserve request", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := validationReq.NormalizeAndValidateForReserve(s.clock.Now()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserve request validation failed", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	span.SetAttributes(
		attribute.String("app.request.transaction_id", transactionID.String()),
		attribute.String("app.request.transaction_type", string(validationReq.TransactionType)),
		attribute.String("app.request.currency", validationReq.Currency),
	)

	result, err := s.service.Reserve(ctx, transactionID, validationReq.ToCheckLimitsInput(), req.GetLongLived())
	if err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", "grpc.reservations.reserve"),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.Int("reservations", len(result.ReservationIDs)),
	).Log(ctx, libLog.LevelDebug, "Reservation processed")

	return &reservationv1.ReserveResult{
		TransactionId:  transactionID.String(),
		Denied:         result.Denied,
		ReservationIds: reservationIDStrings(result.ReservationIDs),
	}, nil
}

// ConfirmByTransaction commits every reservation a transaction holds (phase two,
// /commit-driven). Idempotent: a transaction with no RESERVED rows is a no-op
// success.
func (s *ReservationServer) ConfirmByTransaction(ctx context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	if err := s.terminateByTransaction(ctx, "grpc.reservations.confirm_by_transaction", string(model.StatusConfirmed), req.GetTransactionId(), s.service.ConfirmByTransaction); err != nil {
		return nil, err
	}

	return &reservationv1.ConfirmByTransactionResponse{}, nil
}

// ReleaseByTransaction returns the held capacity for every reservation a
// transaction holds (phase two, /cancel-driven). Idempotent like
// ConfirmByTransaction.
func (s *ReservationServer) ReleaseByTransaction(ctx context.Context, req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	if err := s.terminateByTransaction(ctx, "grpc.reservations.release_by_transaction", string(model.StatusReleased), req.GetTransactionId(), s.service.ReleaseByTransaction); err != nil {
		return nil, err
	}

	return &reservationv1.ReleaseByTransactionResponse{}, nil
}

// ConfirmById commits a single reservation addressed by its id (phase two).
// Idempotent: a retry against an already-terminal reservation succeeds.
func (s *ReservationServer) ConfirmById(ctx context.Context, req *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
	if err := s.terminateByID(ctx, "grpc.reservations.confirm", string(model.StatusConfirmed), req.GetReservationId(), s.service.Confirm); err != nil {
		return nil, err
	}

	return &reservationv1.ConfirmByIdResponse{}, nil
}

// ReleaseById returns a single reservation's held capacity addressed by its id
// (phase two). Idempotent like ConfirmById.
func (s *ReservationServer) ReleaseById(ctx context.Context, req *reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
	if err := s.terminateByID(ctx, "grpc.reservations.release", string(model.StatusReleased), req.GetReservationId(), s.service.Release); err != nil {
		return nil, err
	}

	return &reservationv1.ReleaseByIdResponse{}, nil
}

// terminateByTransaction is the shared by-transaction confirm/release body: parse
// the transaction id, invoke the use case, log the flipped count. The service
// treats an absent or already-terminal transaction as an idempotent no-op.
func (s *ReservationServer) terminateByTransaction(
	ctx context.Context,
	operation string,
	terminalStatus string,
	rawTransactionID string,
	action func(ctx context.Context, transactionID uuid.UUID) (int, error),
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	transactionID, err := uuid.Parse(rawTransactionID)
	if err != nil || transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction id", constant.ErrReservationTransactionIDReq)
		return status.Error(codes.InvalidArgument, constant.ErrReservationTransactionIDReq.Error())
	}

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	flipped, err := action(ctx, transactionID)
	if err != nil {
		return s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("status", terminalStatus),
		libLog.Int("flipped", flipped),
	).Log(ctx, libLog.LevelDebug, "Reservations transitioned by transaction")

	return nil
}

// terminateByID is the shared confirm/release-by-id body: parse the reservation
// id, invoke the use case. The service maps an already-terminal reservation to a
// nil error (idempotent retry).
func (s *ReservationServer) terminateByID(
	ctx context.Context,
	operation string,
	terminalStatus string,
	rawReservationID string,
	action func(ctx context.Context, reservationID uuid.UUID) error,
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservationID, err := uuid.Parse(rawReservationID)
	if err != nil || reservationID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid reservation id", constant.ErrInvalidPathParameter)
		return status.Error(codes.InvalidArgument, constant.ErrInvalidPathParameter.Error())
	}

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	if err := action(ctx, reservationID); err != nil {
		return s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
		libLog.String("status", terminalStatus),
	).Log(ctx, libLog.LevelDebug, "Reservation transition processed")

	return nil
}

// toValidationRequest builds the model.ValidationRequest the reserve path
// validates and converts, from the proto request. It mirrors the field set the
// REST DTO carries: requestId, amount (decimal-as-string), currency, account,
// optional segment/portfolio/merchant ids, transactionType, transactionTimestamp
// (RFC3339). Normalization and validation are delegated to the model so the
// gRPC path never forks the reserve input contract.
func (s *ReservationServer) toValidationRequest(req *reservationv1.ReserveRequest) (*model.ValidationRequest, error) {
	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return nil, constant.ErrValidationRequestIDRequired
	}

	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return nil, constant.ErrValidationAmountNonPositive
	}

	var transactionTimestamp time.Time
	if ts := req.GetTransactionTimestamp(); ts != "" {
		transactionTimestamp, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, constant.ErrValidationTimestampRequired
		}
	}

	var accountID uuid.UUID
	if acc := req.GetAccount(); acc != nil && acc.GetAccountId() != "" {
		accountID, err = uuid.Parse(acc.GetAccountId())
		if err != nil {
			return nil, constant.ErrInvalidPathParameter
		}
	}

	validationReq := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionType(req.GetTransactionType()),
		Amount:               amount,
		Currency:             req.GetCurrency(),
		TransactionTimestamp: transactionTimestamp,
		Account:              model.AccountContext{ID: accountID},
	}

	if segment, err := optionalContextID(req.GetSegmentId()); err != nil {
		return nil, err
	} else if segment != nil {
		validationReq.Segment = &model.SegmentContext{ID: *segment}
	}

	if portfolio, err := optionalContextID(req.GetPortfolioId()); err != nil {
		return nil, err
	} else if portfolio != nil {
		validationReq.Portfolio = &model.PortfolioContext{ID: *portfolio}
	}

	if merchant, err := optionalContextID(req.GetMerchantId()); err != nil {
		return nil, err
	} else if merchant != nil {
		validationReq.Merchant = &model.MerchantContext{ID: *merchant}
	}

	return validationReq, nil
}

// mapServiceError maps a reservation use-case error to a gRPC status error,
// recording it onto the span by error CLASS (T5): a not-found is a business
// outcome (span stays green), context cancellation is transport-side, and every
// other failure is technical (span flips red).
func (s *ReservationServer) mapServiceError(span trace.Span, msg string, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, constant.ErrReservationNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation not found", err)
		return status.Error(codes.NotFound, constant.ErrReservationNotFound.Error())
	default:
		libOpentelemetry.HandleSpanError(span, msg, err)
		return status.Error(codes.Internal, constant.ErrInternalServer.Error())
	}
}

// optionalContextID parses an optional uuid-bearing context id (segment /
// portfolio / merchant). An empty string means the field is absent (nil);
// a present-but-malformed value is rejected.
func optionalContextID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, constant.ErrInvalidPathParameter
	}

	return &id, nil
}

// reservationIDStrings renders the reservation ids as proto-friendly strings.
// A nil/empty input yields a nil slice — proto serializes a repeated field's
// absence and an empty slice identically, so no [] sentinel is needed (unlike
// the REST JSON path).
func reservationIDStrings(ids []uuid.UUID) []string {
	if len(ids) == 0 {
		return nil
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}

	return out
}
