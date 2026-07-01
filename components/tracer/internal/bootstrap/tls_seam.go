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
// cert material.
const (
	tlsModeMTLS = "mtls"
	tlsModeMesh = "mesh"
)

// buildSeamTLSConfig builds the *tls.Config that secures BOTH reservation-seam
// listeners (the gRPC server and the Fiber REST listener). It is the single
// place the seam's mutual-TLS posture is decided, so the two transports cannot
// drift.
//
// Behavior contract (per the Seam Contract — identity is mutual TLS, no shared
// secret):
//
//   - mode "" / "mesh"  ⇒ (nil, nil). The app listens plaintext; a service-mesh
//     sidecar (Istio/Linkerd) terminates mTLS. No cert material is consulted.
//   - mode "mtls"       ⇒ (*tls.Config, nil) presenting the tracer's own server
//     certificate and enforcing tls.RequireAndVerifyClientCert against the
//     loaded client CA pool. The reservation seam is unreachable without a
//     verified client cert.
//   - mode "mtls" + missing/unreadable material ⇒ error naming the failing knob,
//     so a misconfigured deploy fails fast at boot rather than silently serving
//     an unverified seam.
//   - any other mode    ⇒ error (fail fast on a typo rather than guessing).
//
// The function is pure (Config in ⇒ tls.Config|error out) and does no logging —
// it runs at boot before listeners bind, and callers surface its error with
// context. Server cert loading goes through lib-commons certificate.Manager so
// the seam inherits its hot-reload/rotation support via GetCertificate.
func buildSeamTLSConfig(cfg *Config) (*tls.Config, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode))

	switch mode {
	case "", tlsModeMesh:
		return nil, nil
	case tlsModeMTLS:
		return buildMTLSConfig(cfg)
	default:
		return nil, fmt.Errorf("invalid TRACER_TLS_MODE %q: expected %q or %q", cfg.TracerTLSMode, tlsModeMTLS, tlsModeMesh)
	}
}

// buildMTLSConfig assembles the RequireAndVerifyClientCert config for mtls mode.
func buildMTLSConfig(cfg *Config) (*tls.Config, error) {
	if strings.TrimSpace(cfg.TracerTLSCertFile) == "" {
		return nil, fmt.Errorf("TRACER_TLS_MODE=mtls requires TRACER_TLS_CERT_FILE")
	}

	if strings.TrimSpace(cfg.TracerTLSKeyFile) == "" {
		return nil, fmt.Errorf("TRACER_TLS_MODE=mtls requires TRACER_TLS_KEY_FILE")
	}

	if strings.TrimSpace(cfg.TracerTLSClientCAFile) == "" {
		return nil, fmt.Errorf("TRACER_TLS_MODE=mtls requires TRACER_TLS_CLIENT_CA_FILE")
	}

	// Load the server cert/key through lib-commons so the seam can adopt cert
	// rotation (GetCertificate) without restart. NewManager validates the
	// key matches the cert and that the file pair is complete.
	certManager, err := libCert.NewManager(cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load tracer server certificate: %w", err)
	}

	clientCAs, err := loadCertPool(cfg.TracerTLSClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("load tracer client CA (TRACER_TLS_CLIENT_CA_FILE): %w", err)
	}

	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: certManager.GetCertificateFunc(),
		ClientAuth:     tls.RequireAndVerifyClientCert,
		ClientCAs:      clientCAs,
	}, nil
}

// loadCertPool reads a PEM bundle and returns a pool containing its
// certificate(s). Returns an error when the file is unreadable or contains no
// parseable certificate, so a misconfigured CA path fails fast instead of
// yielding an empty pool that rejects every client.
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
