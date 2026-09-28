// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/tls"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v7/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/v4/log"
	libObsOtel "github.com/LerianStudio/lib-observability/v4/tracing"
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
// The server always requires and verifies a client certificate: the gRPC seam
// identifies producers only by that certificate, so bootstrap registers it only
// when TRACER_GRPC_PORT is set under TRACER_TLS_MODE=mtls.
type GRPCServer struct {
	server    *grpc.Server
	address   string
	logger    libObsLog.Logger
	telemetry libObsOtel.Telemetry
}

// NewGRPCServer builds the gRPC server, registers the reservation service, and
// returns the runnable. address is the listen address (e.g. ":4021").
// tlsConfig must require and verify client certificates, and interceptor must
// authenticate the producer and authorize its tenant before any RPC runs;
// both are mandatory, since without either the seam would serve unidentified
// callers. Returns an error if any dependency is nil.
func NewGRPCServer(
	address string,
	reservationServer reservationv1.ReservationServiceServer,
	tlsConfig *tls.Config,
	interceptor grpc.UnaryServerInterceptor,
	logger libObsLog.Logger,
	telemetry *libObsOtel.Telemetry,
	additionalOptions ...grpc.ServerOption,
) (*GRPCServer, error) {
	if reservationServer == nil {
		return nil, fmt.Errorf("reservation server must not be nil")
	}

	if tlsConfig == nil || tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, fmt.Errorf("TLS config must require and verify client certificates")
	}

	if interceptor == nil {
		return nil, fmt.Errorf("reservation interceptor must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	if telemetry == nil {
		return nil, fmt.Errorf("telemetry must not be nil")
	}

	opts := make([]grpc.ServerOption, 0, 3+len(additionalOptions))
	opts = append(
		opts,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(interceptor),
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	)
	opts = append(opts, additionalOptions...)
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
