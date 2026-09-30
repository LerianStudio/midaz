// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v7/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libObsOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// grpcStopTimeout bounds a graceful gRPC drain whose context carries no
// deadline; past it the remaining RPCs are cancelled.
const grpcStopTimeout = 10 * time.Second

// grpcServerComponent labels the gRPC server's goroutines in panic signals.
const grpcServerComponent = "grpc_server"

// GRPCServer serves the reservation seam over gRPC. It is a lib-commons Launcher
// App (Run mirrors HTTPServer), so it drains on SIGTERM through the same
// ServerManager graceful-shutdown path as the Fiber server. The otelgrpc stats
// handler gives the gRPC surface the same tracing parity as REST.
//
// Transport security depends on TRACER_TLS_MODE: in "mtls" mode a non-nil
// *tls.Config is passed in and the server requires+verifies a client cert whose
// identity is in TRACER_TLS_CLIENT_ALLOWED_NAMES when that allowlist is set
// (the reservation seam is unreachable without one); in "mesh" mode the config
// is nil and a sidecar terminates mTLS; an empty mode is plaintext and boots
// only in local deployments. Bootstrap always registers it, on
// TRACER_GRPC_PORT (default :4021).
type GRPCServer struct {
	server    *grpc.Server
	address   string
	logger    libObsLog.Logger
	telemetry libObsOtel.Telemetry
}

// NewGRPCServer builds the gRPC server, registers the reservation service, and
// returns the runnable. address is the listen address (e.g. ":4021"). When
// tlsConfig is non-nil the server enforces mutual TLS via grpc.Creds; nil means
// plaintext (mesh or local empty mode). When tenantInterceptor is non-nil it is chained as a
// unary interceptor so the trusted x-tenant-id resolves the per-tenant pool
// before the reservation handler runs (multi-tenant mode); nil leaves the
// single-tenant path untouched. Returns an error if any dependency is nil.
func NewGRPCServer(
	address string,
	reservationServer reservationv1.ReservationServiceServer,
	tlsConfig *tls.Config,
	tenantInterceptor grpc.UnaryServerInterceptor,
	logger libObsLog.Logger,
	telemetry *libObsOtel.Telemetry,
) (*GRPCServer, error) {
	if reservationServer == nil {
		return nil, fmt.Errorf("reservation server must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	if telemetry == nil {
		return nil, fmt.Errorf("telemetry must not be nil")
	}

	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	if tenantInterceptor != nil {
		opts = append(opts, grpc.ChainUnaryInterceptor(tenantInterceptor))
	}

	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	server := grpc.NewServer(opts...)

	reservationv1.RegisterReservationServiceServer(server, reservationServer)

	return &GRPCServer{
		server:    server,
		address:   address,
		logger:    logger,
		telemetry: *telemetry,
	}, nil
}

// Run starts the gRPC server via the lib-commons ServerManager, which installs
// the graceful-shutdown signal handler and stops the server on SIGTERM.
func (s *GRPCServer) Run(_ *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithGRPCServer(s.server, s.address).
		StartWithGracefulShutdown()

	return nil
}

// Stop drains in-flight RPCs and closes the listener. When ctx ends first it
// cancels the remaining RPCs, so shutdown never outlives its deadline; a ctx
// without a deadline is bounded by grpcStopTimeout. A nil receiver is a no-op.
func (s *GRPCServer) Stop(ctx context.Context) {
	s.stop(ctx, grpcStopTimeout)
}

// stop is Stop with the fallback bound applied when ctx has no deadline.
func (s *GRPCServer) stop(ctx context.Context, fallback time.Duration) {
	if s == nil || s.server == nil {
		return
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, fallback)
		defer cancel()
	}

	drained := make(chan struct{})

	libRuntime.SafeGoWithContextAndComponent(ctx, s.logger, grpcServerComponent, "grpc.graceful_stop",
		libRuntime.KeepRunning, func(context.Context) {
			defer close(drained)

			s.server.GracefulStop()
		})

	select {
	case <-drained:
	case <-ctx.Done():
		s.server.Stop()
		<-drained
	}
}
