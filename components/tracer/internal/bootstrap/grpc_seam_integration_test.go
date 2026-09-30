// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TestIntegration_GRPCSeamWiring_ClientAllowlist boots the reservation gRPC
// server through initGRPCServer in mtls mode on a real loopback listener, with
// an allowlist naming one client identity. Both clients present a certificate
// the configured CA signed; only the allowlisted one reaches the handler. The
// accepted RPC carries an empty transaction id, so the handler answers
// InvalidArgument before any service port is touched.
func TestIntegration_GRPCSeamWiring_ClientAllowlist(t *testing.T) {
	fixture := testutil.GenerateMTLSFixture(t)

	cfg := writeMTLSConfig(t, fixture)
	cfg.TracerTLSClientAllowedNames = "ledger-seam-client"
	cfg.TracerGRPCPort = "127.0.0.1:0"

	addr := startSeamThroughInitGRPCServer(t, cfg)

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(fixture.CACertPEM))

	t.Run("non-allowlisted client certificate is refused", func(t *testing.T) {
		err := confirmOverSeam(t, addr, clientTLSConfig(t, fixture.OtherClientCertPEM, fixture.OtherClientKeyPEM, caPool))
		require.Error(t, err)
		require.Equal(t, codes.Unavailable, status.Code(err),
			"a CA-signed client outside the allowlist must be refused at the transport, got %v", err)
	})

	t.Run("allowlisted client certificate reaches the handler", func(t *testing.T) {
		err := confirmOverSeam(t, addr, clientTLSConfig(t, fixture.ClientCertPEM, fixture.ClientKeyPEM, caPool))
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err),
			"the allowlisted client must reach the reservation handler, got %v", err)
	})
}

// startSeamThroughInitGRPCServer builds the gRPC seam via initGRPCServer over a
// ReservationService of doubles, serves it on a listener bound to
// cfg.TracerGRPCPort and returns the bound address, so a ":0" port is resolved
// by the same listener that serves it. The server is stopped on cleanup.
func startSeamThroughInitGRPCServer(t *testing.T, cfg *Config) string {
	t.Helper()

	ctrl := gomock.NewController(t)
	clk := testutil.NewMockClock(time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC))

	svc, err := services.NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		servicesMocks.NewMockLimitResolver(ctrl),
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
		servicesMocks.NewMockRuleEvaluator(ctrl),
		clk,
	)
	require.NoError(t, err)

	grpcServer, err := initGRPCServer(cfg, svc, nil, nil, clk, testutil.NewMockLogger(), &libOtel.Telemetry{})
	require.NoError(t, err)

	lis, err := net.Listen("tcp", cfg.TracerGRPCPort)
	require.NoError(t, err)

	served := make(chan struct{})

	go func() {
		defer close(served)

		_ = grpcServer.server.Serve(lis)
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		grpcServer.Stop(ctx)
		<-served
	})

	return lis.Addr().String()
}

// confirmOverSeam issues one ConfirmByTransaction with an empty transaction id
// and returns its error.
func confirmOverSeam(t *testing.T, addr string, clientTLS *tls.Config) error {
	t.Helper()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)

	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = reservationv1.NewReservationServiceClient(conn).
		ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{})

	return err
}

// clientTLSConfig builds a client TLS config presenting the given leaf and
// verifying the server against caPool.
func clientTLSConfig(t *testing.T, certPEM, keyPEM []byte, caPool *x509.CertPool) *tls.Config {
	t.Helper()

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   "localhost",
	}
}
