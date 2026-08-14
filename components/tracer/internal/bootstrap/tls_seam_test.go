// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestBuildSeamTLSConfig exercises the reservation-seam TLS builder that both
// listeners (gRPC + Fiber) share. The contract per the Seam Contract:
//
//   - mode "" / "mesh"        ⇒ (nil, nil): app listens plaintext, sidecar (if
//     any) terminates mTLS. No cert material consulted.
//   - mode "mtls" + material  ⇒ (*tls.Config, nil) with
//     ClientAuth=RequireAndVerifyClientCert, a non-nil ClientCAs pool, and a
//     server certificate source.
//   - mode "mtls" + missing material ⇒ error naming the absent file knob.
//
// The function is pure (Config in ⇒ tls.Config|error out) so it is callable at
// boot before any listener binds.
func TestBuildSeamTLSConfig(t *testing.T) {
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

			got, err := buildSeamTLSConfig(tt.cfg)

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
