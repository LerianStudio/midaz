// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v7/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libObsOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// grpcStopTimeout bounds a graceful gRPC drain whose context carries no
// deadline; past it the remaining RPCs are cancelled.
const grpcStopTimeout = 10 * time.Second

// grpcServerComponent labels the gRPC server's goroutines in panic signals.
const grpcServerComponent = "grpc_server"

// errSeamAuthClientNotAuthorizing refuses token identity on an AuthClient
// that is nil, disabled or has no address: lib-auth would pass every call
// through, leaving the principal guard to read unverified claims.
var errSeamAuthClientNotAuthorizing = errors.New("reservation seam: PLUGIN_AUTH_ENABLED=true requires an enabled Access Manager client with an address")

// GRPCServer serves the reservation seam over gRPC. It is a lib-commons Launcher
// App (Run mirrors HTTPServer), so it drains on SIGTERM through the same
// ServerManager graceful-shutdown path as the Fiber server. The otelgrpc stats
// handler gives the gRPC surface the same tracing parity as REST.
//
// Transport security depends on TRACER_TLS_MODE: in "mtls" mode a non-nil
// *tls.Config is passed in and the server requires+verifies a client cert whose
// identity is in TRACER_TLS_CLIENT_ALLOWED_NAMES when that allowlist is set;
// in "server" mode it presents its certificate and asks for none; in "mesh"
// mode the config is nil and a sidecar terminates mTLS; an empty mode is
// plaintext. The caller identity is enforced by the unary interceptor chain
// (seamUnaryInterceptors). Bootstrap always registers it, on TRACER_GRPC_PORT
// (default :4021).
type GRPCServer struct {
	server    *grpc.Server
	address   string
	logger    libObsLog.Logger
	telemetry libObsOtel.Telemetry
}

// NewGRPCServer builds the gRPC server, registers the reservation service, and
// returns the runnable. address is the listen address (e.g. ":4021"). When
// tlsConfig is non-nil the server serves TLS via grpc.Creds; nil means
// plaintext. interceptors are chained, in order, after the otelgrpc stats
// handler, so every identity and tenant decision is traced. The service has
// only unary RPCs, so no stream interceptor is installed. Returns an error if
// any dependency is nil.
func NewGRPCServer(
	address string,
	reservationServer reservationv1.ReservationServiceServer,
	tlsConfig *tls.Config,
	interceptors []grpc.UnaryServerInterceptor,
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

	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
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

// seamUnaryInterceptors returns the seam's unary chain for the identity
// ValidateSeamPosture admitted: identity interceptor(s), then the tenant
// interceptor when the resolver is active.
//
//   - token: lib-auth authorizes the token on tracer/reservations, then
//     SeamPrincipalInterceptor admits only the ledger's application principal,
//     then the tenant resolves from the token's tenantId claim.
//   - API key: SeamAPIKeyInterceptor, then the tenant resolves from
//     x-tenant-id.
//   - transport or none: the tenant resolves from x-tenant-id.
func seamUnaryInterceptors(
	cfg *Config,
	authClient *authMiddleware.AuthClient,
	resolver *seamtenant.Resolver,
	ensurer grpcin.WorkerEnsurer,
) ([]grpc.UnaryServerInterceptor, error) {
	chain := []grpc.UnaryServerInterceptor{}
	tenant := grpcin.TenantUnaryInterceptor(resolver, ensurer)

	switch resolveSeamIdentity(cfg) {
	case seamIdentityToken:
		if authClient == nil || !authClient.Enabled || strings.TrimSpace(authClient.Address) == "" {
			return nil, errSeamAuthClientNotAuthorizing
		}

		chain = append(
			chain,
			authMiddleware.NewGRPCAuthUnaryPolicy(authClient, grpcin.SeamAuthPolicyConfig()),
			grpcin.SeamPrincipalInterceptor(grpcin.SeamPrincipalConfig{
				MultiTenant:    cfg.MultiTenantEnabled,
				AllowedClients: parseSeamAllowedClients(cfg.TracerSeamAllowedClients),
			}),
		)

		tenant = grpcin.TokenTenantUnaryInterceptor(resolver, ensurer)
	case seamIdentityAPIKey:
		chain = append(chain, grpcin.SeamAPIKeyInterceptor(cfg.APIKey, cfg.APIKeyLabel))
	case seamIdentityTransport, seamIdentityNone:
	}

	if resolver.Active() {
		chain = append(chain, tenant)
	}

	return chain, nil
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
