// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"fmt"
	"math"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// ContextClientConfig bounds both directions of the coordinated contract. The
// namespace comes from deployment configuration, never from request metadata.
type ContextClientConfig struct {
	Namespace       string
	Bounds          tracercontract.Limits
	MaxBodyBytes    int
	MaxReservations int
}

func (c ContextClientConfig) Validate() error {
	if c.Namespace == "" || c.MaxBodyBytes <= 0 || c.MaxBodyBytes > math.MaxInt32 || c.MaxReservations <= 0 || c.Bounds.MaxFractionDigits < tracercontract.MinimumResourceProfileFractionDigits {
		return constant.ErrInvalidRequestBody
	}

	return c.Bounds.Validate()
}

// ContextGRPCClient transports the complete shared contract. Legacy lifecycle
// operations stay on TracerGRPCClient and cannot silently complete new records.
type ContextGRPCClient struct {
	transport *TracerGRPCClient
	config    ContextClientConfig
}

func NewContextGRPCClient(target string, config ContextClientConfig, options ...TracerGRPCClientOption) (*ContextGRPCClient, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	transport, err := NewTracerGRPCClient(target, options...)
	if err != nil {
		return nil, err
	}

	return &ContextGRPCClient{transport: transport, config: config}, nil
}

func (c *ContextGRPCClient) Close() error { return c.transport.Close() }

func (c *ContextGRPCClient) Reserve(ctx context.Context, request tracercontract.ReserveRequest) (_ *tracercontract.ReserveResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.reserve_context")
	defer span.End()
	defer func() {
		if retErr != nil {
			libOtel.HandleSpanError(span, "Context reservation failed", retErr)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, c.transport.operationTimeout)
	defer cancel()

	input, err := contractpb.EncodeReserve(ctx, request, c.config.Namespace, c.config.Bounds)
	if err != nil {
		return nil, err
	}

	if proto.Size(input) > c.config.MaxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	wire, err := c.transport.client.Reserve(ctx, input, grpc.MaxCallRecvMsgSize(c.config.MaxBodyBytes), grpc.MaxCallSendMsgSize(c.config.MaxBodyBytes))
	if err != nil {
		return nil, mapGRPCError(err)
	}

	result, err := contractpb.DecodeResult(wire, c.config.MaxReservations)
	if err != nil {
		return nil, fmt.Errorf("invalid tracer decision: %w", err)
	}

	if err := result.ValidateFor(request, c.config.MaxReservations); err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Context reservation response validated")

	return result, nil
}

func (c *ContextGRPCClient) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	return c.complete(ctx, transactionID, false)
}

func (c *ContextGRPCClient) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	return c.complete(ctx, transactionID, true)
}

func (c *ContextGRPCClient) complete(ctx context.Context, transactionID uuid.UUID, release bool) (_ *tracercontract.TransactionCompletionResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.complete_context")
	defer span.End()
	defer func() {
		if retErr != nil {
			libOtel.HandleSpanError(span, "Context completion failed", retErr)
		}
	}()

	if transactionID == uuid.Nil {
		return nil, constant.ErrInvalidRequestBody
	}

	ctx, cancel := context.WithTimeout(ctx, c.transport.operationTimeout)
	defer cancel()

	options := []grpc.CallOption{grpc.MaxCallRecvMsgSize(c.config.MaxBodyBytes), grpc.MaxCallSendMsgSize(c.config.MaxBodyBytes)}

	var (
		result *tracercontract.TransactionCompletionResult
		err    error
	)

	if release {
		reply, callErr := c.transport.client.ReleaseByTransaction(ctx, &reservationv1.ReleaseByTransactionRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionId: transactionID.String()}, options...)
		if callErr != nil {
			return nil, mapGRPCError(callErr)
		}

		if reply == nil || len(reply.ProtoReflect().GetUnknown()) != 0 {
			return nil, constant.ErrInvalidRequestBody
		}

		result, err = decodeCompletionReport(transactionID, "RELEASED", reply.ContractRevision, reply.TransactionId, reply.Status, reply.Flipped, reply.EvaluationId)
	} else {
		reply, callErr := c.transport.client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionId: transactionID.String()}, options...)
		if callErr != nil {
			return nil, mapGRPCError(callErr)
		}

		if reply == nil || len(reply.ProtoReflect().GetUnknown()) != 0 {
			return nil, constant.ErrInvalidRequestBody
		}

		result, err = decodeCompletionReport(transactionID, "CONFIRMED", reply.ContractRevision, reply.TransactionId, reply.Status, reply.Flipped, reply.EvaluationId)
	}

	if err != nil {
		return nil, err
	}

	if result.Flipped > c.config.MaxReservations {
		return nil, constant.ErrInvalidRequestBody
	}

	logger.Log(ctx, libLog.LevelDebug, "Context completion response validated")

	return result, nil
}

func decodeCompletionReport(expectedID uuid.UUID, expectedStatus, revision, transaction, status string, flipped int32, evaluation *string) (*tracercontract.TransactionCompletionResult, error) {
	id, err := uuid.Parse(transaction)
	if err != nil || id != expectedID || status != expectedStatus {
		return nil, constant.ErrInvalidRequestBody
	}

	result := &tracercontract.TransactionCompletionResult{ContractRevision: revision, TransactionID: id, Status: status, Flipped: int(flipped)}

	if evaluation != nil {
		parsed, err := uuid.Parse(*evaluation)
		if err != nil {
			return nil, constant.ErrInvalidRequestBody
		}

		result.EvaluationID = &parsed
	}

	if err := result.Validate(); err != nil {
		return nil, err
	}

	return result, nil
}
