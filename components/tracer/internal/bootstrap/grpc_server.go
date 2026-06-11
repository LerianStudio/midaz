// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/tls"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v5/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/log"
	libObsOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// GRPCServer serves the reservation seam over gRPC. It is a lib-commons Launcher
// App (Run mirrors HTTPServer), so it drains on SIGTERM through the same
// ServerManager graceful-shutdown path as the Fiber server. The otelgrpc stats
// handler gives the gRPC surface the same tracing parity as REST.
//
// Transport security depends on TRACER_TLS_MODE (Epic 1.3): in "mtls" mode a
// non-nil *tls.Config is passed in and the server requires+verifies a client
// cert (the reservation seam is unreachable without one); in "mesh" mode the
// config is nil and a sidecar terminates mTLS. The server is opt-in: bootstrap
// only registers it when TRACER_GRPC_PORT is set.
type GRPCServer struct {
	server    *grpc.Server
	address   string
	logger    libObsLog.Logger
	telemetry libObsOtel.Telemetry
}

// NewGRPCServer builds the gRPC server, registers the reservation service, and
// returns the runnable. address is the listen address (e.g. ":4021"). When
// tlsConfig is non-nil the server enforces mutual TLS via grpc.Creds; nil means
// plaintext (mesh mode). When tenantInterceptor is non-nil it is chained as a
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
