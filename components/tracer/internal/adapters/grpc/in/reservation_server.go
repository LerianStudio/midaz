// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package in hosts the tracer's inbound gRPC adapters. The reservation server
// is the only transport of the two-phase reservation use case: it maps the
// generated proto messages to the domain inputs, delegates to the
// identical *services.ReservationService, and maps the results back. The
// business logic is never duplicated — both transports converge on one service.
package in

//go:generate mockgen -source=reservation_server.go -destination=mocks/reservation_server_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
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
// delegates to, satisfied by *services.ReservationService.
type ReservationService interface {
	Reserve(ctx context.Context, transactionID uuid.UUID, req *model.ValidationRequest, opts services.ReserveOptions) (*services.ReserveResult, error)
	Confirm(ctx context.Context, reservationID uuid.UUID) (services.ConfirmOutcome, error)
	Release(ctx context.Context, reservationID uuid.UUID) error
	ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (services.ConfirmOutcome, error)
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
// reserve timestamp-window check (injected for MOCK_TIME determinism in tests).
// Returns an error if service or clk is nil.
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
// request is mapped to a model.ValidationRequest, normalized and validated with
// the relaxed reserve rules, then converted to the
// CheckLimitsInput the use case resolves against. A limit-exceeded decision comes back as a normal result with
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
		attribute.String("app.request.asset", validationReq.Asset),
		attribute.Bool("app.request.revert", req.GetRevert()),
	)

	result, err := s.service.Reserve(ctx, transactionID, validationReq, services.ReserveOptions{
		LongLived: req.GetLongLived(),
		Revert:    req.GetRevert(),
	})
	if err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", "grpc.reservations.reserve"),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.String("decision", string(result.EffectiveDecision())),
		libLog.Int("reservations", len(result.ReservationIDs)),
	).Log(ctx, libLog.LevelDebug, "Reservation processed")

	return &reservationv1.ReserveResult{
		TransactionId:  transactionID.String(),
		Denied:         result.Denied,
		ReservationIds: reservationIDStrings(result.ReservationIDs),
		Decision:       string(result.EffectiveDecision()),
		Reason:         result.Reason,
		MatchedRuleIds: reservationIDStrings(result.MatchedRuleIDs),
	}, nil
}

// ConfirmByTransaction commits every reservation a transaction holds (phase two,
// /commit-driven). Idempotent: a transaction with no RESERVED rows is a no-op
// success. The response carries the rows this call confirmed and the rows it
// found already RELEASED.
func (s *ReservationServer) ConfirmByTransaction(ctx context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.reservations.confirm_by_transaction")
	defer span.End()

	transactionID, err := parseTransactionID(span, req.GetTransactionId())
	if err != nil {
		return nil, err
	}

	outcome, err := s.service.ConfirmByTransaction(ctx, transactionID)
	if err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	return &reservationv1.ConfirmByTransactionResponse{
		Confirmed:       countToUint32(outcome.Confirmed),
		AlreadyReleased: countToUint32(outcome.AlreadyReleased),
	}, nil
}

// ReleaseByTransaction returns the held capacity for every reservation a
// transaction holds (phase two, /cancel-driven). Idempotent like
// ConfirmByTransaction: the service treats an absent or already-terminal
// transaction as a no-op.
func (s *ReservationServer) ReleaseByTransaction(ctx context.Context, req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	const operation = "grpc.reservations.release_by_transaction"

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	transactionID, err := parseTransactionID(span, req.GetTransactionId())
	if err != nil {
		return nil, err
	}

	released, err := s.service.ReleaseByTransaction(ctx, transactionID)
	if err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Int("released", released),
	).Log(ctx, libLog.LevelDebug, "Reservations released by transaction")

	return &reservationv1.ReleaseByTransactionResponse{}, nil
}

// ConfirmById commits a single reservation addressed by its id (phase two).
// Idempotent: a retry against an already-terminal reservation succeeds, and the
// response says whether the row was already RELEASED.
func (s *ReservationServer) ConfirmById(ctx context.Context, req *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
	const operation = "grpc.reservations.confirm"

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservationID, err := parseReservationID(span, req.GetReservationId())
	if err != nil {
		return nil, err
	}

	outcome, err := s.service.Confirm(ctx, reservationID)
	if err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
		libLog.Int("confirmed", outcome.Confirmed),
		libLog.Int("already_released", outcome.AlreadyReleased),
	).Log(ctx, libLog.LevelDebug, "Reservation confirm processed")

	return &reservationv1.ConfirmByIdResponse{
		AlreadyReleased: outcome.AlreadyReleased > 0,
	}, nil
}

// ReleaseById returns a single reservation's held capacity addressed by its id
// (phase two). Idempotent like ConfirmById: the service maps an already-terminal
// reservation to success.
func (s *ReservationServer) ReleaseById(ctx context.Context, req *reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
	const operation = "grpc.reservations.release"

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservationID, err := parseReservationID(span, req.GetReservationId())
	if err != nil {
		return nil, err
	}

	if err := s.service.Release(ctx, reservationID); err != nil {
		return nil, s.mapServiceError(span, "Reservation processing failed", err)
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
	).Log(ctx, libLog.LevelDebug, "Reservation release processed")

	return &reservationv1.ReleaseByIdResponse{}, nil
}

// toValidationRequest builds the model.ValidationRequest the reserve path
// validates and converts, from the proto request: requestId, amount
// (decimal-as-string), asset, account id and type, optional
// segment/portfolio/merchant ids, transactionType, transactionTimestamp
// (RFC3339) and flat metadata. Normalization and validation are delegated to
// the model so the reserve input contract is the one POST /v1/validations
// applies.
func (s *ReservationServer) toValidationRequest(req *reservationv1.ReserveRequest) (*model.ValidationRequest, error) {
	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return nil, constant.ErrValidationRequestIDRequired
	}

	amount, err := safe.ParseDecimal(req.GetAmount())
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
		Asset:                req.GetAsset(),
		TransactionTimestamp: transactionTimestamp,
		Account:              model.AccountContext{ID: accountID, Type: req.GetAccount().GetType()},
		Metadata:             metadataFromProto(req.GetMetadata()),
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

// parseTransactionID parses the ledger transaction id of a by-transaction RPC
// and records it on the span. An absent or malformed id is InvalidArgument.
func parseTransactionID(span trace.Span, raw string) (uuid.UUID, error) {
	transactionID, err := uuid.Parse(raw)
	if err != nil || transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction id", constant.ErrReservationTransactionIDReq)
		return uuid.Nil, status.Error(codes.InvalidArgument, constant.ErrReservationTransactionIDReq.Error())
	}

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	return transactionID, nil
}

// parseReservationID parses the reservation id of a by-id RPC and records it on
// the span. An absent or malformed id is InvalidArgument.
func parseReservationID(span trace.Span, raw string) (uuid.UUID, error) {
	reservationID, err := uuid.Parse(raw)
	if err != nil || reservationID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid reservation id", constant.ErrInvalidPathParameter)
		return uuid.Nil, status.Error(codes.InvalidArgument, constant.ErrInvalidPathParameter.Error())
	}

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	return reservationID, nil
}

// countToUint32 renders a row count on the proto's uint32 field. A count is never
// negative and never reaches the ceiling in practice; both bounds are clamped so
// the conversion cannot wrap.
func countToUint32(n int) uint32 {
	if n < 0 {
		return 0
	}

	if n > math.MaxUint32 {
		return math.MaxUint32
	}

	return uint32(n)
}

// mapServiceError maps a reservation use-case error to a gRPC status error,
// recording it onto the span by error CLASS (T5): a not-found and a replay onto
// a settled reservation are business outcomes (span stays green), context
// cancellation is transport-side, a rule cache that is not ready yet is
// Unavailable so the caller treats the tracer as temporarily unavailable, and
// every other failure is technical (span flips red). An inactive tenant never
// reaches this mapping: the tenant interceptor answers it before the handler
// runs. A sentinel maps with its code string as the message so the ledger can
// parse it.
func (s *ReservationServer) mapServiceError(span trace.Span, msg string, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, constant.ErrReservationNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation not found", err)
		return status.Error(codes.NotFound, constant.ErrReservationNotFound.Error())
	case errors.Is(err, constant.ErrReservationAlreadySettled):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation already settled", err)
		return status.Error(codes.FailedPrecondition, constant.ErrReservationAlreadySettled.Error())
	case errors.Is(err, constant.ErrRuleCacheNotReady):
		libOpentelemetry.HandleSpanError(span, "Rule cache not ready", err)
		return status.Error(codes.Unavailable, constant.ErrRuleCacheNotReady.Error())
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

// metadataFromProto widens the proto string map into the model's metadata map.
// An empty map yields nil so an absent field and an empty one validate alike.
func metadataFromProto(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}

	return out
}

// reservationIDStrings renders reservation or rule ids as proto-friendly strings.
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
