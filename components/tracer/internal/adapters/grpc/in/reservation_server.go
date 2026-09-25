// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package in hosts Tracer's inbound gRPC adapters. Coordinated Reserve and
// completion use the same admission and completion commands as HTTP. Empty
// lifecycle revisions remain restricted to the legacy reservation service.
package in

//go:generate mockgen -source=reservation_server.go -destination=mocks/reservation_server_service_mock.go -package=mocks

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
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

	service        ReservationService
	admission      ContextReserveAdmitter
	completion     ContextReserveCompleter
	completionByID ContextReserveIDCompleter
	contextConfig  ContextReservationConfig
	clock          clock.Clock
}

// NewReservationServer constructs the legacy lifecycle service. Reserve requires
// NewContextReservationServer; it never reconstructs missing context from the
// removed protobuf fields. Returns an error if service or clk is nil.
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

// ConfirmByTransaction commits every reservation a transaction holds (phase two,
// /commit-driven). Idempotent: a transaction with no RESERVED rows is a no-op
// success.
func (s *ReservationServer) ConfirmByTransaction(ctx context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	if req == nil || len(req.ProtoReflect().GetUnknown()) != 0 {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if req.ContractRevision != "" {
		result, err := s.completeContext(ctx, req.ContractRevision, req.TransactionId, model.OperationConfirmed)
		if err != nil {
			return nil, err
		}

		flipped, err := completionMovementCount(result.Flipped)
		if err != nil {
			return nil, err
		}

		return &reservationv1.ConfirmByTransactionResponse{ContractRevision: result.ContractRevision, TransactionId: result.TransactionID.String(), Status: result.Status, Flipped: flipped, EvaluationId: completionEvaluationID(result)}, nil
	}

	if s.admission != nil {
		return nil, contextReservationError(constant.ErrTracerContractUnavailable)
	}

	if err := s.terminateByTransaction(ctx, "grpc.reservations.confirm_by_transaction", string(model.StatusConfirmed), req.GetTransactionId(), s.service.ConfirmByTransaction); err != nil {
		return nil, err
	}

	return &reservationv1.ConfirmByTransactionResponse{}, nil
}

// ReleaseByTransaction returns the held capacity for every reservation a
// transaction holds (phase two, /cancel-driven). Idempotent like
// ConfirmByTransaction.
func (s *ReservationServer) ReleaseByTransaction(ctx context.Context, req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	if req == nil || len(req.ProtoReflect().GetUnknown()) != 0 {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if req.ContractRevision != "" {
		result, err := s.completeContext(ctx, req.ContractRevision, req.TransactionId, model.OperationReleased)
		if err != nil {
			return nil, err
		}

		flipped, err := completionMovementCount(result.Flipped)
		if err != nil {
			return nil, err
		}

		return &reservationv1.ReleaseByTransactionResponse{ContractRevision: result.ContractRevision, TransactionId: result.TransactionID.String(), Status: result.Status, Flipped: flipped, EvaluationId: completionEvaluationID(result)}, nil
	}

	if s.admission != nil {
		return nil, contextReservationError(constant.ErrTracerContractUnavailable)
	}

	if err := s.terminateByTransaction(ctx, "grpc.reservations.release_by_transaction", string(model.StatusReleased), req.GetTransactionId(), s.service.ReleaseByTransaction); err != nil {
		return nil, err
	}

	return &reservationv1.ReleaseByTransactionResponse{}, nil
}

// ConfirmById commits a single reservation addressed by its id (phase two).
// Idempotent: a retry against an already-terminal reservation succeeds.
func (s *ReservationServer) ConfirmById(ctx context.Context, req *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
	if req == nil || len(req.ProtoReflect().GetUnknown()) != 0 {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if req.ContractRevision != "" {
		result, err := s.completeContextReservation(ctx, req.ContractRevision, req.ReservationId, model.OperationConfirmed)
		if err != nil {
			return nil, err
		}

		evaluation := result.EvaluationID.String()

		return &reservationv1.ConfirmByIdResponse{ContractRevision: result.ContractRevision, TransactionId: result.TransactionID.String(), ReservationId: result.ReservationID.String(), Status: result.Status, EvaluationId: &evaluation}, nil
	}

	if s.admission != nil {
		return nil, contextReservationError(constant.ErrTracerContractUnavailable)
	}

	if err := s.terminateByID(ctx, "grpc.reservations.confirm", string(model.StatusConfirmed), req.GetReservationId(), s.service.Confirm); err != nil {
		return nil, err
	}

	return &reservationv1.ConfirmByIdResponse{}, nil
}

// ReleaseById addresses a whole coordinated operation, or a single legacy
// reservation without a revision. Idempotent like ConfirmById.
func (s *ReservationServer) ReleaseById(ctx context.Context, req *reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
	if req == nil || len(req.ProtoReflect().GetUnknown()) != 0 {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if req.ContractRevision != "" {
		result, err := s.completeContextReservation(ctx, req.ContractRevision, req.ReservationId, model.OperationReleased)
		if err != nil {
			return nil, err
		}

		evaluation := result.EvaluationID.String()

		return &reservationv1.ReleaseByIdResponse{ContractRevision: result.ContractRevision, TransactionId: result.TransactionID.String(), ReservationId: result.ReservationID.String(), Status: result.Status, EvaluationId: &evaluation}, nil
	}

	if s.admission != nil {
		return nil, contextReservationError(constant.ErrTracerContractUnavailable)
	}

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
