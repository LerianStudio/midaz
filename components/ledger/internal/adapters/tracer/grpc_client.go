// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TracerGRPCClient is the ledger-side gRPC client for the tracer reservation
// service. It implements the same TracerReserver port the REST client does, so
// the reserve anchor stays transport-agnostic; buildTracerReserver selects which
// concrete implementation to wire from cfg.TracerTransport. The connection is
// persistent (one grpc.ClientConn for the client's lifetime) and instrumented
// with the otelgrpc client stats handler so the ledger transaction-create trace
// continues across the seam.
//
// Transport / availability failures (a dial error, an Unavailable / DeadlineExceeded
// status, a cancelled context) are mapped to ErrTracerUnavailable so the reserve
// anchor can apply tracer.failPosture, identically to the REST client. A business
// DENIED decision is a successful Reserve return (ReserveResult.Denied=true), not
// an error.
type TracerGRPCClient struct {
	conn             *grpc.ClientConn
	client           reservationv1.ReservationServiceClient
	operationTimeout time.Duration
}

// TracerGRPCClientOption configures a TracerGRPCClient.
type TracerGRPCClientOption func(*tracerGRPCClientConfig)

// tracerGRPCClientConfig collects optional construction inputs before they are
// resolved into the persistent client. dialOptions lets Epic 1.3 inject mTLS
// transport credentials; today the client defaults to insecure transport.
type tracerGRPCClientConfig struct {
	operationTimeout time.Duration
	dialOptions      []grpc.DialOption
}

// WithGRPCOperationTimeout sets the per-operation context timeout from the
// ledger's tracer.timeoutMs setting. A non-positive value leaves the default in
// place. It mirrors WithOperationTimeout on the REST client.
func WithGRPCOperationTimeout(d time.Duration) TracerGRPCClientOption {
	return func(c *tracerGRPCClientConfig) {
		if d > 0 {
			c.operationTimeout = d
		}
	}
}

// WithGRPCDialOptions appends dial options to the persistent connection. It is
// the injection point for transport credentials (mTLS lands in Epic 1.3); when
// no credentials are supplied the client dials with insecure transport.
func WithGRPCDialOptions(opts ...grpc.DialOption) TracerGRPCClientOption {
	return func(c *tracerGRPCClientConfig) {
		c.dialOptions = append(c.dialOptions, opts...)
	}
}

// NewTracerGRPCClient builds a gRPC client for the tracer reservation service
// over a persistent connection to target. It returns an error when target is
// empty so a misconfigured composition root fails at boot rather than at the
// first transaction. grpc.NewClient is lazy — it does not dial until the first
// RPC — so this never blocks on tracer reachability at boot.
func NewTracerGRPCClient(target string, opts ...TracerGRPCClientOption) (*TracerGRPCClient, error) {
	if target == "" {
		return nil, errors.New("empty target passed to NewTracerGRPCClient")
	}

	conf := &tracerGRPCClientConfig{operationTimeout: defaultOperationTimeout}
	for _, opt := range opts {
		opt(conf)
	}

	dialOptions := make([]grpc.DialOption, 0, len(conf.dialOptions)+3)
	dialOptions = append(dialOptions,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(tenantUnaryInterceptor),
	)

	// Default to insecure transport ONLY when no dial options are injected
	// (mesh/empty mode). When mTLS credentials arrive via WithGRPCDialOptions
	// (Epic 1.3) they carry their own transport credentials, and an unconditional
	// insecure default appended afterwards would be the last WithTransportCredentials
	// and silently clobber them — dialing plaintext against the TLS server. Gating
	// the insecure default on the absence of injected options keeps mesh mode
	// working without overriding the secured transport.
	if len(conf.dialOptions) == 0 {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialOptions = append(dialOptions, conf.dialOptions...)

	conn, err := grpc.NewClient(target, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("dial tracer gRPC: %w", err)
	}

	return &TracerGRPCClient{
		conn:             conn,
		client:           reservationv1.NewReservationServiceClient(conn),
		operationTimeout: conf.operationTimeout,
	}, nil
}

// Close releases the persistent connection. Register it with the composition
// root so the connection drains on SIGTERM.
func (c *TracerGRPCClient) Close() error {
	return c.conn.Close()
}

// Reserve holds limit capacity for a transaction (phase one). A DENIED decision
// comes back as a successful ReserveResult with Denied=true (not an error); only
// transport / availability failures return ErrTracerUnavailable.
func (c *TracerGRPCClient) Reserve(ctx context.Context, req ReserveRequest) (*ReserveResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.reserve")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", req.TransactionID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	resp, err := c.client.Reserve(ctx, toProtoReserveRequest(req))
	if err != nil {
		mapped := mapGRPCError(err)
		libOpentelemetry.HandleSpanError(span, "Reserve transport failed", mapped)

		return nil, mapped
	}

	result, err := fromProtoReserveResult(resp)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to map reserve response", err)
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Reservation processed",
		libLog.String("transaction_id", req.TransactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.Int("reservations", len(result.ReservationIDs)),
	)

	return result, nil
}

// Confirm commits a held reservation by id (phase two — commit).
func (c *TracerGRPCClient) Confirm(ctx context.Context, reservationID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.confirm")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	_, err := c.client.ConfirmById(ctx, &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		libOpentelemetry.HandleSpanError(span, "Reservation confirm transport failed", mapped)

		return mapped
	}

	return nil
}

// Release returns a held reservation's capacity by id (phase two — abort).
func (c *TracerGRPCClient) Release(ctx context.Context, reservationID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.release")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	_, err := c.client.ReleaseById(ctx, &reservationv1.ReleaseByIdRequest{ReservationId: reservationID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		libOpentelemetry.HandleSpanError(span, "Reservation release transport failed", mapped)

		return mapped
	}

	return nil
}

// ConfirmByTransaction commits every reservation a transaction holds (phase two
// — commit by transaction).
func (c *TracerGRPCClient) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.confirm_by_transaction")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	_, err := c.client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		libOpentelemetry.HandleSpanError(span, "Reservation confirm-by-transaction transport failed", mapped)

		return mapped
	}

	return nil
}

// ReleaseByTransaction returns every reservation a transaction holds (phase two
// — abort by transaction).
func (c *TracerGRPCClient) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.release_by_transaction")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	_, err := c.client.ReleaseByTransaction(ctx, &reservationv1.ReleaseByTransactionRequest{TransactionId: transactionID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		libOpentelemetry.HandleSpanError(span, "Reservation release-by-transaction transport failed", mapped)

		return mapped
	}

	return nil
}

// tenantUnaryInterceptor propagates the request's tenant to the tracer as the
// trusted x-tenant-id outgoing metadata on every RPC, mirroring the REST
// client's TenantHeader injection. The value is resolved from context via
// tmcore.GetTenantIDContext; in single-tenant mode it is empty and nothing is
// appended (the tracer then runs its single-tenant pass-through). The tenant
// metadata key is the lower-cased TenantHeader (tenantMetadataKey) so the two
// transports cannot drift. The tenant value is never logged.
func tenantUnaryInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	if tenant := tmcore.GetTenantIDContext(ctx); tenant != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, tenantMetadataKey, tenant)
	}

	return invoker(ctx, method, req, reply, cc, opts...)
}

// toProtoReserveRequest mirrors the REST ReserveRequest onto the proto message
// field-for-field. The account is always sent as a populated message; an empty
// AccountID serializes to an empty account_id, which the tracer's relaxed reserve
// validation treats the same way the REST {} body is treated.
func toProtoReserveRequest(req ReserveRequest) *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        req.TransactionID.String(),
		RequestId:            req.RequestID,
		Amount:               req.Amount,
		Currency:             req.Currency,
		Account:              &reservationv1.ReserveAccount{AccountId: req.Account.AccountID},
		SegmentId:            req.SegmentID,
		PortfolioId:          req.PortfolioID,
		MerchantId:           req.MerchantID,
		TransactionType:      req.TransactionType,
		TransactionTimestamp: req.TransactionTimestamp,
		LongLived:            req.LongLived,
	}
}

// fromProtoReserveResult maps the proto reserve response back onto the REST
// result type the TracerReserver port speaks. Reservation ids are parsed back to
// uuid.UUID; a malformed id from the tracer is a contract violation, surfaced as
// an error rather than silently dropped.
func fromProtoReserveResult(resp *reservationv1.ReserveResult) (*ReserveResult, error) {
	if resp == nil {
		return nil, errors.New("nil reserve result from tracer")
	}

	transactionID, err := uuid.Parse(resp.GetTransactionId())
	if err != nil {
		return nil, fmt.Errorf("parse reserve result transaction id: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(resp.GetReservationIds()))

	for _, raw := range resp.GetReservationIds() {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse reservation id: %w", err)
		}

		ids = append(ids, id)
	}

	return &ReserveResult{
		TransactionID:  transactionID,
		Denied:         resp.GetDenied(),
		ReservationIDs: ids,
	}, nil
}

// mapGRPCError normalises a gRPC RPC error to the seam's error vocabulary.
// Availability-class status codes (Unavailable, DeadlineExceeded, Canceled) and
// a context deadline / cancellation are folded into ErrTracerUnavailable so the
// reserve anchor's fail-posture branch handles them, matching the REST client's
// transport-failure normalisation. Other status codes (e.g. NotFound, Internal,
// InvalidArgument) are returned verbatim — they are non-availability outcomes the
// caller surfaces as-is.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	}

	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	default:
		return err
	}
}
