// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TracerGRPCClient is the ledger-side gRPC client for the tracer reservation
// service. It implements the TracerReserver port the reserve anchor depends
// on. The connection is persistent (one grpc.ClientConn for the client's
// lifetime) and instrumented with the otelgrpc client stats handler so the
// ledger transaction-create trace continues across the seam.
//
// Transport / availability failures (a dial error, an Unavailable / DeadlineExceeded
// status, a cancelled context) are mapped to ErrTracerUnavailable so the reserve
// anchor can apply tracer.failPosture. A deadline or cancellation that struck a
// call already sent is additionally marked ErrTracerNoAnswer. A business DENIED
// decision is a successful Reserve return (ReserveResult.Denied=true), not an
// error.
type TracerGRPCClient struct {
	conn             *grpc.ClientConn
	client           reservationv1.ReservationServiceClient
	operationTimeout time.Duration
}

// TracerGRPCClientOption configures a TracerGRPCClient.
type TracerGRPCClientOption func(*tracerGRPCClientConfig)

// tracerGRPCClientConfig collects optional construction inputs before they are
// resolved into the persistent client. dialOptions injects mTLS
// transport credentials; today the client defaults to insecure transport.
type tracerGRPCClientConfig struct {
	operationTimeout time.Duration
	dialOptions      []grpc.DialOption
}

// WithGRPCOperationTimeout sets the per-operation context timeout from the
// ledger's tracer.timeoutMs setting. A non-positive value leaves the default in
// place.
func WithGRPCOperationTimeout(d time.Duration) TracerGRPCClientOption {
	return func(c *tracerGRPCClientConfig) {
		if d > 0 {
			c.operationTimeout = d
		}
	}
}

// WithGRPCDialOptions appends dial options to the persistent connection. It is
// the injection point for transport credentials (mTLS); when
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

	conf := &tracerGRPCClientConfig{operationTimeout: DefaultOperationTimeout}
	for _, opt := range opts {
		opt(conf)
	}

	dialOptions := make([]grpc.DialOption, 0, len(conf.dialOptions)+3)
	dialOptions = append(
		dialOptions,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(tenantUnaryInterceptor),
	)

	// Default to insecure transport ONLY when no dial options are injected
	// (mesh/empty mode). When mTLS credentials arrive via WithGRPCDialOptions
	// they carry their own transport credentials, and an unconditional
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
// comes back as a successful ReserveResult with Denied=true (not an error).
// Transport / availability failures return ErrTracerUnavailable; a tracer
// refusal of the request itself returns ErrTracerRejected. A request context
// already done is refused before send, so it is never marked ErrTracerNoAnswer.
func (c *TracerGRPCClient) Reserve(ctx context.Context, req ReserveRequest) (*ReserveResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.reserve")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", req.TransactionID.String()))

	if ctxErr := ctx.Err(); ctxErr != nil {
		notSent := fmt.Errorf("%w: reserve not sent: %w", ErrTracerUnavailable, ctxErr)
		recordRPCFailure(span, "Reserve not sent", notSent)

		return nil, notSent
	}

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	resp, err := c.client.Reserve(ctx, toProtoReserveRequest(req))
	if err != nil {
		mapped := mapGRPCError(err)
		recordRPCFailure(span, "Reserve failed", mapped)

		return nil, mapped
	}

	result, err := fromProtoReserveResult(resp)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to map reserve response", err)
		return nil, err
	}

	logger.Log(
		ctx, libLog.LevelDebug, "Reservation processed",
		libLog.String("transaction_id", req.TransactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.String("decision", result.Decision),
		libLog.Int("reservations", len(result.ReservationIDs)),
	)

	return result, nil
}

// Confirm commits a held reservation by id (phase two — commit).
func (c *TracerGRPCClient) Confirm(ctx context.Context, reservationID uuid.UUID) (ConfirmOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.confirm")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	resp, err := c.client.ConfirmById(ctx, &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		recordRPCFailure(span, "Reservation confirm failed", mapped)

		return ConfirmOutcome{}, mapped
	}

	if resp.GetAlreadyReleased() {
		return ConfirmOutcome{Confirmed: 0, AlreadyReleased: 1}, nil
	}

	return ConfirmOutcome{Confirmed: 1, AlreadyReleased: 0}, nil
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
		recordRPCFailure(span, "Reservation release failed", mapped)

		return mapped
	}

	return nil
}

// ConfirmByTransaction commits every reservation a transaction holds (phase two
// — commit by transaction).
func (c *TracerGRPCClient) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (ConfirmOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.grpc_client.confirm_by_transaction")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	resp, err := c.client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
	if err != nil {
		mapped := mapGRPCError(err)
		recordRPCFailure(span, "Reservation confirm-by-transaction failed", mapped)

		return ConfirmOutcome{}, mapped
	}

	return ConfirmOutcome{
		Confirmed:       int(resp.GetConfirmed()),
		AlreadyReleased: int(resp.GetAlreadyReleased()),
	}, nil
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
		recordRPCFailure(span, "Reservation release-by-transaction failed", mapped)

		return mapped
	}

	return nil
}

// recordRPCFailure records a failed RPC onto its span by failure class: a
// tracer rejection of the request (ErrTracerRejected) is a business outcome and
// keeps the span out of error; every other failure is technical and marks it.
func recordRPCFailure(span trace.Span, msg string, err error) {
	if errors.Is(err, ErrTracerRejected) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, msg, err)
		return
	}

	libOpentelemetry.HandleSpanError(span, msg, err)
}

// tenantUnaryInterceptor propagates the request's tenant to the tracer as the
// trusted x-tenant-id outgoing metadata on every RPC. The value is resolved
// from context via tmcore.GetTenantIDContext; in single-tenant mode it is empty
// and nothing is appended (the tracer then runs its single-tenant
// pass-through). The tenant value is never logged.
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

// toProtoReserveRequest maps the ReserveRequest onto the proto message
// field-for-field. The account is always sent as a populated message; an empty
// AccountID serializes to an empty account_id, which the tracer's relaxed reserve
// validation treats as an absent account.
func toProtoReserveRequest(req ReserveRequest) *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        req.TransactionID.String(),
		RequestId:            req.RequestID,
		Amount:               req.Amount,
		Asset:                req.Asset,
		Account:              &reservationv1.ReserveAccount{AccountId: req.Account.AccountID, Type: req.Account.Type},
		SegmentId:            req.SegmentID,
		PortfolioId:          req.PortfolioID,
		MerchantId:           req.MerchantID,
		TransactionType:      req.TransactionType,
		TransactionTimestamp: req.TransactionTimestamp,
		LongLived:            req.LongLived,
		Metadata:             req.Metadata,
		Revert:               req.Revert,
	}
}

// fromProtoReserveResult maps the proto reserve response back onto the
// ReserveResult the TracerReserver port speaks. Reservation and matched rule ids
// are parsed back to uuid.UUID; a malformed id from the tracer is a contract
// violation, surfaced as an error rather than silently dropped.
func fromProtoReserveResult(resp *reservationv1.ReserveResult) (*ReserveResult, error) {
	if resp == nil {
		return nil, errors.New("nil reserve result from tracer")
	}

	transactionID, err := uuid.Parse(resp.GetTransactionId())
	if err != nil {
		return nil, fmt.Errorf("parse reserve result transaction id: %w", err)
	}

	ids, err := parseProtoIDs(resp.GetReservationIds())
	if err != nil {
		return nil, fmt.Errorf("parse reservation id: %w", err)
	}

	ruleIDs, err := parseProtoIDs(resp.GetMatchedRuleIds())
	if err != nil {
		return nil, fmt.Errorf("parse matched rule id: %w", err)
	}

	return &ReserveResult{
		TransactionID:  transactionID,
		Denied:         resp.GetDenied(),
		Decision:       resp.GetDecision(),
		Reason:         resp.GetReason(),
		MatchedRuleIDs: ruleIDs,
		ReservationIDs: ids,
	}, nil
}

// parseProtoIDs parses a repeated proto id field. It always returns a non-nil
// slice so an absent field and an empty one read alike.
func parseProtoIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))

	for _, value := range raw {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// mapGRPCError normalises a gRPC RPC error to the seam's error vocabulary.
// Availability-class status codes (Unavailable, DeadlineExceeded, Canceled) and
// a context deadline / cancellation are folded into ErrTracerUnavailable so the
// reserve anchor's fail-posture branch handles them. A deadline or cancellation
// is also marked ErrTracerNoAnswer: the call may have reached the tracer and
// committed. Unavailable is not, because it is either the transport refusing to
// send or the tracer's own answer. InvalidArgument and FailedPrecondition mean
// the tracer refused the request itself and are wrapped in ErrTracerRejected.
// Other status codes (e.g. NotFound, Internal) are returned verbatim. Every wrap
// keeps the original status reachable through errors.As.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w: %w", ErrTracerUnavailable, ErrTracerNoAnswer, err)
	}

	switch status.Code(err) {
	case codes.DeadlineExceeded, codes.Canceled:
		return fmt.Errorf("%w: %w: %w", ErrTracerUnavailable, ErrTracerNoAnswer, err)
	case codes.Unavailable:
		return fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	case codes.InvalidArgument, codes.FailedPrecondition:
		return fmt.Errorf("%w: %w", ErrTracerRejected, err)
	default:
		return err
	}
}
