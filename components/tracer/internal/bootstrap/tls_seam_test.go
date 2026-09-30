// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestParseClientAllowedNames locks the allowlist parsing rule: entries are
// comma-separated, trimmed, lowercased, and empty entries are dropped.
func TestParseClientAllowedNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty string yields no entries", raw: "", want: nil},
		{name: "only separators and spaces yield no entries", raw: " , ,, ", want: nil},
		{name: "single entry", raw: "ledger", want: []string{"ledger"}},
		{name: "entries are trimmed", raw: "  ledger.svc , spiffe://lerian/ledger  ", want: []string{"ledger.svc", "spiffe://lerian/ledger"}},
		{name: "empty entries are dropped", raw: "ledger,,  ,ledger-2,", want: []string{"ledger", "ledger-2"}},
		{name: "entries are lowercased", raw: "Ledger.SVC,SPIFFE://Lerian/Ledger", want: []string{"ledger.svc", "spiffe://lerian/ledger"}},
		{name: "duplicates collapse", raw: "ledger,LEDGER, ledger ", want: []string{"ledger"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseClientAllowedNames(tt.raw)

			require.Len(t, got, len(tt.want))

			for _, name := range tt.want {
				_, ok := got[name]
				require.True(t, ok, "expected %q in the allowlist", name)
			}
		})
	}
}

// TestVerifyClientAllowedName exercises the gRPC seam identity check against
// generated certificates: a leaf matches when one of its DNS SANs, URI SANs or
// its Subject CN equals an allowlist entry (case-insensitive).
func TestVerifyClientAllowedName(t *testing.T) {
	t.Parallel()

	spiffe, err := url.Parse("spiffe://lerian.studio/ns/midaz/sa/ledger")
	require.NoError(t, err)

	dnsLeaf := generateIdentityCert(t, "unrelated-cn", []string{"ledger.midaz.svc.cluster.local"}, nil)
	uriLeaf := generateIdentityCert(t, "unrelated-cn", nil, []*url.URL{spiffe})
	cnLeaf := generateIdentityCert(t, "Ledger-Seam-Client", nil, nil)
	otherLeaf := generateIdentityCert(t, "intruder", []string{"intruder.svc"}, nil)
	cnBehindDNSLeaf := generateIdentityCert(t, "ledger-seam-client", []string{"intruder.svc"}, nil)
	intruderURI, err := url.Parse("spiffe://lerian.studio/ns/other/sa/intruder")
	require.NoError(t, err)
	cnBehindURILeaf := generateIdentityCert(t, "ledger-seam-client", nil, []*url.URL{intruderURI})

	allowed := parseClientAllowedNames("LEDGER.midaz.svc.cluster.local, spiffe://lerian.studio/ns/midaz/sa/ledger, ledger-seam-client")
	verify := verifyClientAllowedName(allowed)

	tests := []struct {
		name    string
		state   tls.ConnectionState
		wantErr bool
	}{
		{name: "DNS SAN match", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{dnsLeaf}}},
		{name: "URI SAN match", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{uriLeaf}}},
		{name: "Subject CN match", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cnLeaf}}},
		{name: "only the leaf is checked", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{otherLeaf, cnLeaf}}, wantErr: true},
		{name: "no match is refused", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{otherLeaf}}, wantErr: true},
		{name: "CN is ignored when a DNS SAN is present", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cnBehindDNSLeaf}}, wantErr: true},
		{name: "CN is ignored when a URI SAN is present", state: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cnBehindURILeaf}}, wantErr: true},
		{name: "no peer certificate is refused", state: tls.ConnectionState{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := verify(tt.state)
			if !tt.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			// The refusal names no certificate contents.
			require.NotContains(t, err.Error(), "intruder")
			require.NotContains(t, strings.ToLower(err.Error()), "ledger")
		})
	}
}

// TestBuildGRPCSeamTLSConfig locks the gRPC listener's TLS posture: nil in
// mesh/empty mode (the allowlist is ignored), a clone of the shared seam
// config in mtls, and a VerifyConnection hook only when the allowlist names
// at least one identity.
func TestBuildGRPCSeamTLSConfig(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := writeSeamCertFixture(t)

	mtls := func(allowed string) *Config {
		return &Config{
			TracerTLSMode:               "mtls",
			TracerTLSCertFile:           certFile,
			TracerTLSKeyFile:            keyFile,
			TracerTLSClientCAFile:       caFile,
			TracerTLSClientAllowedNames: allowed,
		}
	}

	t.Run("mesh and empty mode ignore the allowlist", func(t *testing.T) {
		t.Parallel()

		for _, mode := range []string{"", "mesh"} {
			got, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: mode, TracerTLSClientAllowedNames: "ledger"})
			require.NoError(t, err)
			require.Nil(t, got)
		}
	})

	t.Run("invalid mode propagates the shared builder error", func(t *testing.T) {
		t.Parallel()

		got, err := buildGRPCSeamTLSConfig(&Config{TracerTLSMode: "insecure", TracerTLSClientAllowedNames: "ledger"})
		require.ErrorContains(t, err, "TRACER_TLS_MODE")
		require.Nil(t, got)
	})

	t.Run("mtls with empty allowlist keeps any CA-signed client", func(t *testing.T) {
		t.Parallel()

		got, err := buildGRPCSeamTLSConfig(mtls(" , "))
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, tls.RequireAndVerifyClientCert, got.ClientAuth)
		require.NotNil(t, got.ClientCAs)
		require.Nil(t, got.VerifyConnection)
	})

	t.Run("mtls with allowlist installs the identity check", func(t *testing.T) {
		t.Parallel()

		got, err := buildGRPCSeamTLSConfig(mtls("ledger-seam-client"))
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, tls.RequireAndVerifyClientCert, got.ClientAuth)
		require.NotNil(t, got.VerifyConnection)

		allowedLeaf := generateIdentityCert(t, "ledger-seam-client", nil, nil)
		otherLeaf := generateIdentityCert(t, "someone-else", nil, nil)

		require.NoError(t, got.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{allowedLeaf}}))
		require.Error(t, got.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{otherLeaf}}))
	})

	t.Run("the HTTP seam config never carries the allowlist", func(t *testing.T) {
		t.Parallel()

		got, err := buildSeamTLSConfig(mtls("ledger-seam-client"))
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Nil(t, got.VerifyConnection)
	})
}

// TestWarnGRPCSeamAcceptsAnyClient locks the boot Warn: exactly one Warn in
// mtls with an empty allowlist, nothing otherwise.
func TestWarnGRPCSeamAcceptsAnyClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       *Config
		wantWarns int
	}{
		{name: "mtls with empty allowlist warns once", cfg: &Config{TracerTLSMode: " MTLS "}, wantWarns: 1},
		{name: "mtls with blank allowlist warns once", cfg: &Config{TracerTLSMode: "mtls", TracerTLSClientAllowedNames: " , "}, wantWarns: 1},
		{name: "mtls with allowlist is silent", cfg: &Config{TracerTLSMode: "mtls", TracerTLSClientAllowedNames: "ledger"}},
		{name: "mesh mode is silent", cfg: &Config{TracerTLSMode: "mesh"}},
		{name: "empty mode is silent", cfg: &Config{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.NewMockLogger()

			warnGRPCSeamAcceptsAnyClient(context.Background(), tt.cfg, logger)

			require.Len(t, logger.Calls, tt.wantWarns)

			for _, call := range logger.Calls {
				require.Equal(t, "warn", call.Level)
				require.Equal(t,
					"gRPC seam accepts any client certificate signed by TRACER_TLS_CLIENT_CA_FILE; set TRACER_TLS_CLIENT_ALLOWED_NAMES",
					call.Message)
			}
		})
	}
}

// generateIdentityCert returns a self-signed leaf carrying the given identity
// fields. The verifier inspects only identity fields, so a self-signed cert
// with a fixed validity window is sufficient.
func generateIdentityCert(t *testing.T, commonName string, dnsNames []string, uris []*url.URL) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     dnsNames,
		URIs:         uris,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert
}
