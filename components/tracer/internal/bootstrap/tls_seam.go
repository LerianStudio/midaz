// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	libCert "github.com/LerianStudio/lib-commons/v7/commons/certificate"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// TLS modes for the reservation seam (TRACER_TLS_MODE). Empty is treated as
// tlsModeMesh so local dev and the Phase-1 toggle default keep working without
// cert material.
const (
	tlsModeMTLS = "mtls"
	tlsModeMesh = "mesh"
)

// buildSeamTLSConfig builds the base *tls.Config of both seam listeners. The
// Fiber listener uses it as-is; the gRPC server uses it through
// buildGRPCSeamTLSConfig, which adds only the client identity allowlist, so the
// two transports cannot drift on the rest of the mutual-TLS posture.
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

// errClientNotAllowed refuses a verified client certificate whose identity is
// outside TRACER_TLS_CLIENT_ALLOWED_NAMES. It names no certificate contents.
var errClientNotAllowed = errors.New("client certificate identity is not in TRACER_TLS_CLIENT_ALLOWED_NAMES")

// grpcSeamAcceptsAnyClientMsg is the boot Warn logged when the gRPC seam runs
// mtls without a client identity allowlist.
const grpcSeamAcceptsAnyClientMsg = "gRPC seam accepts any client certificate signed by TRACER_TLS_CLIENT_CA_FILE; set TRACER_TLS_CLIENT_ALLOWED_NAMES"

// buildGRPCSeamTLSConfig builds the gRPC listener's *tls.Config. It returns
// nil in mesh/empty mode (the allowlist is ignored there). In mtls it returns a
// clone of buildSeamTLSConfig's result and, when TRACER_TLS_CLIENT_ALLOWED_NAMES
// names at least one identity, a VerifyConnection hook that refuses a verified
// client whose leaf matches none of them. The HTTP listener keeps
// buildSeamTLSConfig, so the allowlist never reaches it. Pure: no logging.
func buildGRPCSeamTLSConfig(cfg *Config) (*tls.Config, error) {
	base, err := buildSeamTLSConfig(cfg)
	if err != nil || base == nil {
		return base, err
	}

	grpcTLS := base.Clone()

	if allowed := parseClientAllowedNames(cfg.TracerTLSClientAllowedNames); len(allowed) > 0 {
		grpcTLS.VerifyConnection = verifyClientAllowedName(allowed)
	}

	return grpcTLS, nil
}

// warnGRPCSeamAcceptsAnyClient logs one Warn when the gRPC seam runs mtls with
// an empty allowlist, because any certificate the client CA signed is then
// accepted as the ledger.
func warnGRPCSeamAcceptsAnyClient(ctx context.Context, cfg *Config, logger libLog.Logger) {
	if strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode)) != tlsModeMTLS {
		return
	}

	if len(parseClientAllowedNames(cfg.TracerTLSClientAllowedNames)) > 0 {
		return
	}

	logger.Log(ctx, libLog.LevelWarn, grpcSeamAcceptsAnyClientMsg)
}

// parseClientAllowedNames splits the comma-separated allowlist into a set of
// trimmed, lowercased identities, dropping empty entries.
func parseClientAllowedNames(raw string) map[string]struct{} {
	allowed := make(map[string]struct{})

	for _, entry := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(entry))
		if name == "" {
			continue
		}

		allowed[name] = struct{}{}
	}

	return allowed
}

// verifyClientAllowedName returns a tls.Config.VerifyConnection hook that
// accepts the connection only when the verified leaf (PeerCertificates[0])
// carries an allowlisted DNS SAN, URI SAN or Subject CN.
func verifyClientAllowedName(allowed map[string]struct{}) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || !clientCertAllowed(cs.PeerCertificates[0], allowed) {
			return errClientNotAllowed
		}

		return nil
	}
}

// clientCertAllowed reports whether any identity field of cert is allowlisted.
func clientCertAllowed(cert *x509.Certificate, allowed map[string]struct{}) bool {
	if cert == nil {
		return false
	}

	isAllowed := func(name string) bool {
		_, ok := allowed[strings.ToLower(strings.TrimSpace(name))]

		return ok
	}

	for _, dnsName := range cert.DNSNames {
		if isAllowed(dnsName) {
			return true
		}
	}

	for _, uri := range cert.URIs {
		if uri != nil && isAllowed(uri.String()) {
			return true
		}
	}

	return cert.Subject.CommonName != "" && isAllowed(cert.Subject.CommonName)
}
