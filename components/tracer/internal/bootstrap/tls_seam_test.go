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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestBuildGRPCTLSConfig locks the gRPC listener contract: mesh/unset listens
// plaintext, mtls requires and verifies a client certificate, and missing
// material fails naming the absent knob.
func TestBuildGRPCTLSConfig(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := writeSeamCertFixture(t)

	tests := []struct {
		name       string
		cfg        *Config
		wantNil    bool
		wantErr    bool
		errMatches string
	}{
		{
			name:    "empty mode listens plaintext",
			cfg:     &Config{TracerTLSMode: ""},
			wantNil: true,
		},
		{
			name:    "mesh mode listens plaintext",
			cfg:     &Config{TracerTLSMode: "mesh"},
			wantNil: true,
		},
		{
			name:    "mesh mode ignores cert material",
			cfg:     &Config{TracerTLSMode: "mesh", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile, TracerTLSClientCAFile: caFile},
			wantNil: true,
		},
		{
			name: "mtls with full material builds verifying config",
			cfg: &Config{
				TracerTLSMode:         "mtls",
				TracerTLSCertFile:     certFile,
				TracerTLSKeyFile:      keyFile,
				TracerTLSClientCAFile: caFile,
			},
			wantNil: false,
		},
		{
			name: "mtls is case- and space-insensitive",
			cfg: &Config{
				TracerTLSMode:         "  MTLS ",
				TracerTLSCertFile:     certFile,
				TracerTLSKeyFile:      keyFile,
				TracerTLSClientCAFile: caFile,
			},
			wantNil: false,
		},
		{
			name:       "mtls missing cert file fails",
			cfg:        &Config{TracerTLSMode: "mtls", TracerTLSKeyFile: keyFile, TracerTLSClientCAFile: caFile},
			wantErr:    true,
			errMatches: "TRACER_TLS_CERT_FILE",
		},
		{
			name:       "mtls missing key file fails",
			cfg:        &Config{TracerTLSMode: "mtls", TracerTLSCertFile: certFile, TracerTLSClientCAFile: caFile},
			wantErr:    true,
			errMatches: "TRACER_TLS_KEY_FILE",
		},
		{
			name:       "mtls missing client CA fails",
			cfg:        &Config{TracerTLSMode: "mtls", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile},
			wantErr:    true,
			errMatches: "TRACER_TLS_CLIENT_CA_FILE",
		},
		{
			name:       "unknown mode fails fast",
			cfg:        &Config{TracerTLSMode: "insecure"},
			wantErr:    true,
			errMatches: "TRACER_TLS_MODE",
		},
		{
			name: "mtls with unreadable CA fails",
			cfg: &Config{
				TracerTLSMode:         "mtls",
				TracerTLSCertFile:     certFile,
				TracerTLSKeyFile:      keyFile,
				TracerTLSClientCAFile: filepath.Join(t.TempDir(), "missing-ca.pem"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildGRPCTLSConfig(tt.cfg)

			if tt.wantErr {
				require.Error(t, err)

				if tt.errMatches != "" {
					require.ErrorContains(t, err, tt.errMatches)
				}

				require.Nil(t, got)

				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, got)

				return
			}

			require.NotNil(t, got)
			require.Equal(t, tls.RequireAndVerifyClientCert, got.ClientAuth)
			require.NotNil(t, got.ClientCAs)
			// Either a static certificate or a hot-reload GetCertificate hook
			// must supply the server identity.
			require.True(t, got.GetCertificate != nil || len(got.Certificates) > 0,
				"tls.Config must provide a server certificate")
		})
	}
}

func TestBuildHTTPTLSConfig(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := writeSeamCertFixture(t)

	for _, mode := range []string{"", "mesh"} {
		got, err := buildHTTPTLSConfig(&Config{TracerTLSMode: mode, TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile, TracerTLSClientCAFile: caFile})
		require.NoError(t, err)
		require.Nil(t, got)
	}

	cfg := &Config{TracerTLSMode: "mtls", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile, TracerTLSClientCAFile: caFile}
	got, err := buildHTTPTLSConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, tls.NoClientCert, got.ClientAuth)
	require.Nil(t, got.ClientCAs)
	require.NotNil(t, got.GetCertificate)

	grpcTLS, err := buildGRPCTLSConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, grpcTLS.ClientAuth, "the HTTP clone must not relax the gRPC configuration")
	require.NotNil(t, grpcTLS.ClientCAs)

	_, err = buildHTTPTLSConfig(&Config{TracerTLSMode: "mtls", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile})
	require.ErrorContains(t, err, "TRACER_TLS_CLIENT_CA_FILE")

	_, err = buildHTTPTLSConfig(&Config{TracerTLSMode: "insecure"})
	require.ErrorContains(t, err, "TRACER_TLS_MODE")
}

// TestPerPortTLSHandshake performs real loopback handshakes with a client that
// presents no certificate: the HTTP listener accepts it and the gRPC listener
// refuses it.
func TestPerPortTLSHandshake(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := writeSeamCertFixture(t)
	caPEM, err := os.ReadFile(caFile) //#nosec G304 -- test-owned temp file
	require.NoError(t, err)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))

	cfg := &Config{TracerTLSMode: "mtls", TracerTLSCertFile: certFile, TracerTLSKeyFile: keyFile, TracerTLSClientCAFile: caFile}

	httpTLS, err := buildHTTPTLSConfig(cfg)
	require.NoError(t, err)

	grpcTLS, err := buildGRPCTLSConfig(cfg)
	require.NoError(t, err)

	clientTLS := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost"}

	require.NoError(t, serverHandshakeWithoutClientCert(t, httpTLS, clientTLS), "HTTP listener must accept a client without a certificate")
	require.Error(t, serverHandshakeWithoutClientCert(t, grpcTLS, clientTLS), "gRPC listener must refuse a client without a certificate")
}

// serverHandshakeWithoutClientCert dials a loopback listener secured by
// serverTLS with clientTLS and returns the server-side handshake outcome, which
// is where a missing client certificate is detected under every TLS version.
func serverHandshakeWithoutClientCert(t *testing.T, serverTLS, clientTLS *tls.Config) error {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	outcome := make(chan error, 1)

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			outcome <- acceptErr

			return
		}

		defer func() { _ = conn.Close() }()

		outcome <- tls.Server(conn, serverTLS).HandshakeContext(ctx)
	}()

	dialer := &tls.Dialer{Config: clientTLS}

	conn, dialErr := dialer.DialContext(ctx, "tcp", listener.Addr().String())
	if dialErr == nil {
		defer func() { _ = conn.Close() }()
	}

	select {
	case err := <-outcome:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// writeSeamCertFixture materializes a CA cert, a leaf server cert/key signed by
// it, and returns their file paths. Times are fixed (no time.Now) via the
// shared test fixture helper so the certs are deterministic.
func writeSeamCertFixture(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()

	mat := testutil.GenerateMTLSFixture(t)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "server-cert.pem")
	keyFile = filepath.Join(dir, "server-key.pem")
	caFile = filepath.Join(dir, "ca.pem")

	require.NoError(t, os.WriteFile(certFile, mat.ServerCertPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, mat.ServerKeyPEM, 0o600))
	require.NoError(t, os.WriteFile(caFile, mat.CACertPEM, 0o600))

	return certFile, keyFile, caFile
}
