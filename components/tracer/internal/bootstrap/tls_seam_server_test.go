// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestBuildGRPCSeamTLSConfig_ServerMode(t *testing.T) {
	t.Parallel()

	certFile, keyFile, _ := writeSeamCertFixture(t)

	t.Run("server mode presents a certificate and asks for no client certificate", func(t *testing.T) {
		t.Parallel()

		got, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: " Server ", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, tls.NoClientCert, got.ClientAuth)
		assert.Nil(t, got.ClientCAs)
		assert.Nil(t, got.VerifyConnection, "the client certificate allowlist belongs to mtls only")
		assert.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
		assert.True(t, got.GetCertificate != nil || len(got.Certificates) > 0)
	})

	t.Run("server mode ignores a client CA and an allowlist", func(t *testing.T) {
		t.Parallel()

		got, err := buildGRPCSeamTLSConfig(&Config{
			TracerTLSMode:               "server",
			TracerTLSCertFile:           certFile,
			TracerTLSKeyFile:            keyFile,
			TracerTLSClientAllowedNames: "ledger",
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, tls.NoClientCert, got.ClientAuth)
		assert.Nil(t, got.VerifyConnection)
	})

	t.Run("server mode without a certificate fails naming the knob", func(t *testing.T) {
		t.Parallel()

		_, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: "server", TracerTLSKeyFile: keyFile})
		require.ErrorContains(t, err, "TRACER_TLS_MODE=server requires TRACER_TLS_CERT_FILE")

		_, err = buildGRPCSeamTLSConfig(&Config{TracerTLSMode: "server", TracerTLSCertFile: certFile})
		require.ErrorContains(t, err, "TRACER_TLS_MODE=server requires TRACER_TLS_KEY_FILE")
	})

	t.Run("the HTTP listener stays plaintext under server mode", func(t *testing.T) {
		t.Parallel()

		got, err := buildSeamTLSConfig(&Config{TracerTLSMode: "server", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile})
		require.NoError(t, err)
		assert.Nil(t, got, "server mode secures the gRPC listener only")
	})

	t.Run("an unknown mode names every accepted value", func(t *testing.T) {
		t.Parallel()

		_, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: "insecure"})
		require.ErrorContains(t, err, `expected "mtls", "mesh" or "server"`)
	})
}

// TestGRPCSeamServerMode_Handshake serves gRPC on loopback under server mode:
// a client trusting the CA reaches the handler without presenting any
// certificate, and a client that does not trust the CA fails the handshake.
func TestGRPCSeamServerMode_Handshake(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := writeSeamCertFixture(t)

	caPEM, err := os.ReadFile(caFile)
	require.NoError(t, err)

	serverTLS, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: "server", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile})
	require.NoError(t, err)

	reached := func(any, grpc.ServerStream) error { return status.Error(codes.FailedPrecondition, "reached") }

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.UnknownServiceHandler(reached))

	go func() { _ = server.Serve(lis) }()

	t.Cleanup(server.Stop)

	invoke := func(clientTLS *tls.Config) error {
		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
		require.NoError(t, err)

		defer func() { _ = conn.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return conn.Invoke(ctx, "/test.Seam/Call", &emptypb.Empty{}, &emptypb.Empty{})
	}

	t.Run("client trusting the CA connects without a certificate", func(t *testing.T) {
		t.Parallel()

		pool := x509.NewCertPool()
		require.True(t, pool.AppendCertsFromPEM(caPEM))

		err := invoke(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: "localhost"})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err), "the handler must be reached, got %v", err)
	})

	t.Run("client without the CA fails the handshake", func(t *testing.T) {
		t.Parallel()

		err := invoke(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool(), ServerName: "localhost"})
		assert.Equal(t, codes.Unavailable, status.Code(err), "an untrusted server must be refused, got %v", err)
	})
}
