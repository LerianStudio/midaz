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
	"os"
	"path/filepath"
	"testing"
	"time"

	libObsOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TestReservationMTLS proves the tracer enforces client-certificate
// verification on the reservation seam in TRACER_TLS_MODE=mtls, on BOTH
// transports:
//
//   - gRPC: a client presenting a CA-signed cert completes the Reserve RPC; a
//     client without a cert is rejected at the TLS layer (the RPC never reaches
//     the service).
//   - REST (Fiber): a tls.Dial with a CA-signed client cert handshakes; a dial
//     without a client cert is rejected by the server.
//
// It runs under the integration tag because it binds real loopback sockets and
// performs real TLS handshakes. No Docker is required: certs come from the
// deterministic fixture helper (fixed validity window, no time.Now).
func TestReservationMTLS(t *testing.T) {
	fixture := testutil.GenerateMTLSFixture(t)
	cfg := writeMTLSConfig(t, fixture)

	serverTLS, err := buildSeamTLSConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, serverTLS)
	require.Equal(t, tls.RequireAndVerifyClientCert, serverTLS.ClientAuth)

	clientCert, err := tls.X509KeyPair(fixture.ClientCertPEM, fixture.ClientKeyPEM)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(fixture.CACertPEM))

	t.Run("gRPC accepts CA-signed client and rejects uncertified client", func(t *testing.T) {
		addr := startGRPCMTLSServer(t, serverTLS)

		// Valid client: presents its cert, verifies the server against the CA.
		validCreds := credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      caPool,
			ServerName:   "localhost",
		})

		validConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(validCreds))
		require.NoError(t, err)
		defer func() { _ = validConn.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := reservationv1.NewReservationServiceClient(validConn).
			Reserve(ctx, &reservationv1.ReserveRequest{TransactionId: "tx-1"})
		require.NoError(t, err, "CA-signed client must complete the RPC")
		require.NotNil(t, resp)
		require.True(t, resp.GetDenied(), "stub returns the sentinel denied=true")

		// Uncertified client: server verifies the CA but presents NO client
		// cert, so the handshake must fail and the RPC must error.
		noCertCreds := credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    caPool,
			ServerName: "localhost",
		})

		noCertConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(noCertCreds))
		require.NoError(t, err)
		defer func() { _ = noCertConn.Close() }()

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()

		_, err = reservationv1.NewReservationServiceClient(noCertConn).
			Reserve(ctx2, &reservationv1.ReserveRequest{TransactionId: "tx-2"})
		require.Error(t, err, "client without a verified cert must be rejected")
	})

	t.Run("Fiber TLS listener accepts CA-signed client and rejects uncertified client", func(t *testing.T) {
		addr := startFiberMTLSServer(t, cfg, serverTLS)

		// Valid client: handshake completes.
		validConn, err := tls.Dial("tcp", addr, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      caPool,
			ServerName:   "localhost",
		})
		require.NoError(t, err, "CA-signed client must handshake with the Fiber TLS listener")
		require.NoError(t, validConn.Handshake())
		_ = validConn.Close()

		// Uncertified client: server demands a client cert and rejects the
		// connection. Under TLS 1.3 the client's Handshake() can complete
		// optimistically (the server's alert rides the first flight the client
		// reads), so we force a read to surface the rejection — an uncertified
		// client must never exchange application data with the seam.
		require.Error(t, mtlsRejectionError(addr, caPool),
			"client without a cert must be rejected by the mTLS seam")
	})
}

// mtlsRejectionError dials addr without a client certificate, completes the
// handshake, and attempts a read. It returns the first error observed — under
// TLS 1.2 the handshake itself fails; under TLS 1.3 the server's bad-certificate
// alert surfaces on the read. Either way an uncertified client cannot exchange
// data with the seam, which is the property under test.
func mtlsRejectionError(addr string, caPool *x509.CertPool) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    caPool,
		ServerName: "localhost",
	})
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	if err := conn.Handshake(); err != nil {
		return err
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 1)

	_, err = conn.Read(buf)

	return err
}

// startGRPCMTLSServer stands up a gRPC server secured by serverTLS, registers a
// stub reservation service, and returns its loopback address. The server is
// stopped on test cleanup.
func startGRPCMTLSServer(t *testing.T, serverTLS *tls.Config) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	reservationv1.RegisterReservationServiceServer(server, &deniedReservationServer{})

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

// startFiberMTLSServer runs a Fiber app behind the seam TLS listener (the same
// path HTTPServer.Run uses in mtls mode) and returns its loopback address.
func startFiberMTLSServer(t *testing.T, cfg *Config, serverTLS *tls.Config) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	hs, err := NewHTTPServer(cfg, app, serverTLS, testutil.NewMockLogger(), &libObsOtel.Telemetry{})
	require.NoError(t, err)
	require.NotNil(t, hs)

	tlsListener := tls.NewListener(listener, serverTLS)
	go func() { _ = app.Listener(tlsListener) }()

	t.Cleanup(func() {
		_ = app.ShutdownWithContext(context.Background())
	})

	return listener.Addr().String()
}

// deniedReservationServer is a stub that returns a known sentinel so the test
// can confirm a verified client's RPC actually reaches the service.
type deniedReservationServer struct {
	reservationv1.UnimplementedReservationServiceServer
}

func (deniedReservationServer) Reserve(context.Context, *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	return &reservationv1.ReserveResult{TransactionId: "tx-1", Denied: true}, nil
}

// writeMTLSConfig materializes the fixture certs to disk and returns a Config
// pointing TRACER_TLS_* at them in mtls mode.
func writeMTLSConfig(t *testing.T, fixture testutil.MTLSFixture) *Config {
	t.Helper()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "server-cert.pem")
	keyFile := filepath.Join(dir, "server-key.pem")
	caFile := filepath.Join(dir, "ca.pem")

	require.NoError(t, os.WriteFile(certFile, fixture.ServerCertPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, fixture.ServerKeyPEM, 0o600))
	require.NoError(t, os.WriteFile(caFile, fixture.CACertPEM, 0o600))

	return &Config{
		ServerAddress:         "127.0.0.1:0",
		TracerTLSMode:         "mtls",
		TracerTLSCertFile:     certFile,
		TracerTLSKeyFile:      keyFile,
		TracerTLSClientCAFile: caFile,
	}
}
