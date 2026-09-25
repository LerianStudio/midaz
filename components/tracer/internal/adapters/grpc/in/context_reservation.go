// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"math"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=context_reservation.go -destination=mocks/context_reservation_mock.go -package=mocks

type (
	ContextReserveAdmitter interface {
		Execute(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error)
	}
	ContextReserveCompleter interface {
		ExecuteReport(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error)
	}
)

type ContextReservationConfig struct {
	Bounds          tracercontract.Limits
	MaxBodyBytes    int
	MaxReservations int
}

func NewContextReservationServer(legacy ReservationService, clk clock.Clock, admission ContextReserveAdmitter, completion ContextReserveCompleter, completionByID ContextReserveIDCompleter, config ContextReservationConfig) (*ReservationServer, error) {
	if admission == nil || completion == nil || completionByID == nil || config.MaxBodyBytes <= 0 || config.MaxReservations <= 0 || config.MaxReservations > math.MaxInt32 {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := config.Bounds.Validate(); err != nil {
		return nil, err
	}

	server, err := NewReservationServer(legacy, clk)
	if err != nil {
		return nil, err
	}

	server.admission = admission
	server.completion = completion
	server.completionByID = completionByID
	server.contextConfig = config

	return server, nil
}

func (s *ReservationServer) Reserve(ctx context.Context, input *reservationv1.ReserveRequest) (_ *reservationv1.ReserveResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.context_reserve")
	defer span.End()
	defer func() {
		if retErr != nil {
			if status.Code(retErr) == codes.InvalidArgument || status.Code(retErr) == codes.PermissionDenied || status.Code(retErr) == codes.AlreadyExists || status.Code(retErr) == codes.FailedPrecondition {
				libOtel.HandleSpanBusinessErrorEvent(span, "Reserve rejected", retErr)
			} else {
				libOtel.HandleSpanError(span, "Reserve failed", retErr)
			}
		}
	}()

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, contextReservationError(constant.ErrInsufficientPrivileges)
	}

	if s.admission == nil {
		return nil, contextReservationError(constant.ErrContextPolicyUnavailable)
	}

	if input != nil && proto.Size(input) > s.contextConfig.MaxBodyBytes {
		return nil, status.Error(codes.ResourceExhausted, constant.ErrInvalidRequestBody.Error())
	}

	request, err := contractpb.DecodeReserve(ctx, input, identity.AssetNamespace, s.contextConfig.Bounds, s.contextConfig.MaxBodyBytes)
	if err != nil {
		return nil, contextReservationError(err)
	}

	result, err := s.admission.Execute(ctx, request)
	if err != nil {
		return nil, contextReservationError(err)
	}

	if result == nil || result.TransactionID != request.TransactionID {
		return nil, contextReservationError(constant.ErrInternalServer)
	}

	encoded, err := contractpb.EncodeResult(result, s.contextConfig.MaxReservations)
	if err != nil {
		return nil, contextReservationError(constant.ErrInternalServer)
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Context Reserve response ready")

	return encoded, nil
}

func (s *ReservationServer) completeContext(ctx context.Context, revision, id string, outcome model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
	if _, ok := contextutil.GetIntegrationIdentity(ctx); !ok {
		return nil, contextReservationError(constant.ErrInsufficientPrivileges)
	}

	if revision != tracercontract.ReserveContractRevision {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if s.completion == nil {
		return nil, contextReservationError(constant.ErrContextPolicyUnavailable)
	}

	transaction, err := uuid.Parse(id)
	if err != nil || transaction == uuid.Nil {
		return nil, contextReservationError(constant.ErrInvalidPathParameter)
	}

	result, err := s.completion.ExecuteReport(ctx, transaction, outcome)
	if err != nil {
		return nil, contextReservationError(err)
	}

	if result == nil || result.TransactionID != transaction || result.Status != string(outcome) || result.Flipped > math.MaxInt32 || result.Validate() != nil {
		return nil, contextReservationError(constant.ErrInternalServer)
	}

	return result, nil
}

func completionEvaluationID(result *tracercontract.TransactionCompletionResult) *string {
	if result.EvaluationID == nil {
		return nil
	}

	id := result.EvaluationID.String()

	return &id
}

func contextReservationError(err error) error {
	mappings := []struct {
		cause error
		code  codes.Code
	}{
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{constant.ErrInvalidRequestBody, codes.InvalidArgument},
		{constant.ErrInvalidPathParameter, codes.InvalidArgument},
		{constant.ErrValidationTimestampFuture, codes.InvalidArgument},
		{constant.ErrValidationTimestampPast, codes.InvalidArgument},
		{constant.ErrInsufficientPrivileges, codes.PermissionDenied},
		{constant.ErrReservationTenantRequired, codes.InvalidArgument},
		{constant.ErrReserveDecisionConflict, codes.AlreadyExists},
		{constant.ErrReserveOperationConflict, codes.FailedPrecondition},
		{constant.ErrReservationNotFound, codes.NotFound},
		{constant.ErrContextPolicyUnavailable, codes.FailedPrecondition},
		{constant.ErrContextLimitsUnavailable, codes.FailedPrecondition},
		{constant.ErrExpressionCostExceeded, codes.FailedPrecondition},
		{constant.ErrExpressionEvaluation, codes.FailedPrecondition},
	}
	for _, mapping := range mappings {
		if errors.Is(err, mapping.cause) {
			return status.Error(mapping.code, mapping.cause.Error())
		}
	}

	return status.Error(codes.Internal, constant.ErrInternalServer.Error())
}

func completionMovementCount(count int) (int32, error) {
	if count < 0 || count > math.MaxInt32 {
		return 0, contextReservationError(constant.ErrInternalServer)
	}

	return int32(count), nil
}

// ContextReserveIDCompleter treats a reservation ID as an address for its whole operation.
type ContextReserveIDCompleter interface {
	Execute(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.ReservationCompletionResult, error)
}

func (s *ReservationServer) completeContextReservation(ctx context.Context, revision, id string, outcome model.ReserveOperationStatus) (*tracercontract.ReservationCompletionResult, error) {
	if _, ok := contextutil.GetIntegrationIdentity(ctx); !ok {
		return nil, contextReservationError(constant.ErrInsufficientPrivileges)
	}

	if revision != tracercontract.ReserveContractRevision {
		return nil, contextReservationError(constant.ErrInvalidRequestBody)
	}

	if s.completionByID == nil {
		return nil, contextReservationError(constant.ErrContextPolicyUnavailable)
	}

	reservation, err := uuid.Parse(id)
	if err != nil || reservation == uuid.Nil {
		return nil, contextReservationError(constant.ErrInvalidPathParameter)
	}

	result, err := s.completionByID.Execute(ctx, reservation, outcome)
	if err != nil {
		return nil, contextReservationError(err)
	}

	if result == nil || result.Validate() != nil || result.ReservationID != reservation || result.Status != string(outcome) {
		return nil, contextReservationError(constant.ErrInternalServer)
	}

	return result, nil
}
