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
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TracerGRPCClient is the gRPC transport under ContextGRPCClient. The
// connection is persistent (one grpc.ClientConn for the client's lifetime) and
// instrumented with the otelgrpc client stats handler so the ledger
// transaction-create trace continues across the seam.
//
// Every failure without a recognized canonical code (a dial error, any status,
// a cancelled context) is mapped to ErrTracerUnavailable by mapGRPCError so the
// reserve anchor can apply tracer.failPosture, identically to the REST client.
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
	dialOptions = append(
		dialOptions,
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

// mapGRPCError normalises a gRPC RPC error to the seam's error vocabulary.
// Only a status whose message is a canonical code the seam recognizes is
// deterministic, whatever its status code: a refusal before evaluation (0043,
// 0487, 0527) wraps ErrTracerRequestRejected, as on the REST transport. Every
// other failure, including PermissionDenied, InvalidArgument or NotFound
// without a recognized code, is ErrTracerUnavailable: the answer did not come
// from a Tracer that evaluated the request, so the fail posture and the
// retrier decide.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	}

	if grpcStatus, ok := status.FromError(err); ok {
		if cause := seamCause(grpcStatus.Message()); cause != nil {
			return cause
		}
	}

	return fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
}
