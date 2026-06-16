// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	libCert "github.com/LerianStudio/lib-commons/v5/commons/certificate"
)

// TLS modes for the reservation seam (TRACER_TLS_MODE). Empty is treated as
// tlsModeMesh so local dev and the Phase-1 toggle default keep working without
// cert material. These mirror the tracer-side constants so both ends of the
// seam agree on the vocabulary.
const (
	tlsModeMTLS = "mtls"
	tlsModeMesh = "mesh"
)

// buildSeamClientTLSConfig builds the *tls.Config the ledger uses to dial the
// tracer reservation seam over mutual TLS. It is the single place the ledger's
// client-side mTLS posture is decided, so the gRPC and REST transports cannot
// drift (both consume the returned config).
//
// Behavior contract (per the Seam Contract — identity is mutual TLS, no shared
// secret):
//
//   - mode "" / "mesh"  ⇒ (nil, nil). The ledger dials plaintext; a local
//     service-mesh sidecar (Istio/Linkerd) originates mTLS. No cert material is
//     consulted — the operator's assertion is trusted.
//   - mode "mtls"       ⇒ (*tls.Config, nil) presenting the ledger's client
//     certificate (GetClientCertificate) and verifying the tracer's server
//     certificate against the loaded CA pool (RootCAs). serverName pins the
//     name the leaf is verified against.
//   - mode "mtls" + missing/unreadable material ⇒ error naming the failing knob,
//     so a misconfigured deploy fails fast at boot rather than silently dialing
//     an unverified seam.
//   - any other mode    ⇒ error (fail fast on a typo rather than guessing).
//
// The function is pure (Config + serverName in ⇒ tls.Config|error out) and does
// no logging — it runs at boot before clients dial, and callers surface its
// error with context. The client cert loads through lib-commons
// certificate.Manager so the seam inherits its hot-reload/rotation support via
// GetClientCertificate.
func buildSeamClientTLSConfig(cfg *Config, serverName string) (*tls.Config, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode))

	switch mode {
	case "", tlsModeMesh:
		return nil, nil
	case tlsModeMTLS:
		return buildClientMTLSConfig(cfg, serverName)
	default:
		return nil, fmt.Errorf("invalid TRACER_TLS_MODE %q: expected %q or %q", cfg.TracerTLSMode, tlsModeMTLS, tlsModeMesh)
	}
}

// buildClientMTLSConfig assembles the client-side mutual-TLS config for mtls
// mode: present the client leaf, verify the tracer's server leaf against the CA.
func buildClientMTLSConfig(cfg *Config, serverName string) (*tls.Config, error) {
	if strings.TrimSpace(cfg.TracerTLSCertFile) == "" {
		return nil, fmt.Errorf("reservation seam requires mTLS material: TRACER_TLS_MODE=mtls requires TRACER_TLS_CERT_FILE")
	}

	if strings.TrimSpace(cfg.TracerTLSKeyFile) == "" {
		return nil, fmt.Errorf("reservation seam requires mTLS material: TRACER_TLS_MODE=mtls requires TRACER_TLS_KEY_FILE")
	}

	if strings.TrimSpace(cfg.TracerTLSCAFile) == "" {
		return nil, fmt.Errorf("reservation seam requires mTLS material: TRACER_TLS_MODE=mtls requires TRACER_TLS_CA_FILE")
	}

	// Load the client cert/key through lib-commons so the seam can adopt cert
	// rotation without restart. NewManager validates the key matches the cert
	// and that the file pair is complete.
	certManager, err := libCert.NewManager(cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load ledger client certificate: %w", err)
	}

	rootCAs, err := loadCertPool(cfg.TracerTLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("load tracer server CA (TRACER_TLS_CA_FILE): %w", err)
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
		RootCAs:    rootCAs,
		// GetClientCertificate always serves the most recently loaded cert via the
		// Manager's TLSCertificate snapshot, so rotation is transparent. It ignores
		// the CertificateRequestInfo (the seam presents a single leaf).
		GetClientCertificate: func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert := certManager.TLSCertificate()
			return &cert, nil
		},
	}, nil
}

// loadCertPool reads a PEM bundle and returns a pool containing its
// certificate(s). Returns an error when the file is unreadable or contains no
// parseable certificate, so a misconfigured CA path fails fast instead of
// yielding an empty pool that rejects every server.
func loadCertPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path) //#nosec G304 -- operator-supplied trusted CA path
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no PEM certificates found in %q", path)
	}

	return pool, nil
}
