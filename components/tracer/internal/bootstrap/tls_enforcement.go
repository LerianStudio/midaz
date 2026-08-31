// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strings"
)

// ValidateSaaSTLS enforces TLS posture on configured DSNs/URLs when
// DEPLOYMENT_MODE=saas. MUST be called from bootstrap BEFORE any connection
// opens. Centralizes a check that would otherwise drift across connection
// sites (anti-pattern N6: scattered inline checks). One function, one call
// site — `grep -rn 'DEPLOYMENT_MODE.*saas' internal/` should find this file
// and nothing else outside of tests/docs.
//
// Scope: Tracer's /readyz cycle is single-tenant, so this gate enforces
// Postgres TLS at boot when DEPLOYMENT_MODE=saas.
//
// Behavior contract:
//
//   - cfg == nil                              ⇒ error ("nil config")
//   - cfg.DeploymentMode != "saas"            ⇒ no-op (returns nil)
//   - dep not configured (empty DSN/host)     ⇒ skipped (returns nil)
//   - malformed DSN                           ⇒ wrapped parse error naming
//     the failing dep
//   - configured but non-TLS                  ⇒ error naming the failing dep
//
// NO logging from inside this function — it runs before logger/telemetry
// are wired, so structured logging is unavailable. Errors propagate up
// through fmt.Errorf wrapping and are surfaced by the caller (initCoreInfra)
// with full context.
func ValidateSaaSTLS(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("validate SaaS TLS: nil config")
	}

	// Normalize deployment mode (case + whitespace) so values like "SaaS" or
	// " saas " cannot bypass the gate by string-equality alone.
	if !isSaaSMode(cfg.DeploymentMode) {
		return nil
	}

	// Postgres: detection requires a non-empty DBHost. buildPostgresDSN
	// always produces a non-empty string (it concatenates field values
	// even when they are zero), so we cannot rely on connStr emptiness as
	// the "dep configured" signal. DBHost is the canonical signal —
	// initPostgresConnection itself fails fast when it is empty.
	if strings.TrimSpace(cfg.DBHost) != "" {
		dsn := buildPostgresDSN(cfg)

		tls, err := detectPostgresTLS(dsn)
		if err != nil {
			return fmt.Errorf("validate TLS for postgres: %w", err)
		}

		if !tls {
			return fmt.Errorf(
				"DEPLOYMENT_MODE=saas: TLS required for postgres but not configured (DB_SSL_MODE=%q)",
				cfg.DBSSLMode,
			)
		}
	}

	return nil
}

// ValidateSaaSStreamingTLS extends the SaaS TLS gate to the lib-streaming Kafka
// broker dial. It is separate from ValidateSaaSTLS because STREAMING_TLS_* is
// lib-streaming's own env contract and never lands on tracer's Config — binding
// it there is what left STREAMING_TLS_ENABLED with no reader at all in the
// pre-v3 tracer. The flag exists only once libStreaming.LoadConfig has run, so
// BuildStreamingEmitter resolves it and passes it in.
//
// The rule is the one Postgres already answers to above: in SaaS mode the
// transport is encrypted or the service does not boot. The check is reached only
// when streaming is ENABLED, because a disabled producer opens no broker
// connection at all. BYOC and local deployments keep their plaintext brokers.
func ValidateSaaSStreamingTLS(cfg *Config, streamingTLSEnabled bool) error {
	if cfg == nil {
		return fmt.Errorf("validate SaaS streaming TLS: nil config")
	}

	if !isSaaSMode(cfg.DeploymentMode) || streamingTLSEnabled {
		return nil
	}

	return fmt.Errorf(
		"DEPLOYMENT_MODE=saas: TLS required for streaming but not configured (set STREAMING_TLS_ENABLED=true)",
	)
}

// ValidateSaaSDeclarationTLS extends the SaaS TLS gate to the Responsibility-
// Inversion (RI) permission-declaration publisher's IdP dial. The publisher sends
// the M2M client_credentials grant and the resulting bearer token to IDP_HOST, so a
// Lerian-hosted deployment must not reach the IdP in cleartext — the same rule
// Postgres and the streaming broker already answer to.
//
// It is a no-op unless RI declaration is ENABLED: a disabled publisher opens no IdP
// connection at all. An empty or scheme-less IDP_HOST is likewise not this gate's
// concern — the publisher's own pre-flight and lib-auth's config validation already
// cover incomplete/malformed config and fail open (no dial happens); a missing host
// is not an insecure dial. Only an explicit http:// scheme trips the gate. BYOC and
// local deployments keep their plaintext IdP.
//
// Like the Postgres and streaming SaaS TLS gates, this does NOT honor the
// ALLOW_INSECURE_TLS escape hatch — that escape lives in the connection
// constructors, not in the SaaS TLS gate. IDP_HOST is a plain host URL, never a
// credential: the M2M secret is not part of this value and is never referenced here,
// so nothing sensitive can reach the error message.
func ValidateSaaSDeclarationTLS(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("validate SaaS declaration TLS: nil config")
	}

	if !cfg.DeclarationEnabled || !isSaaSMode(cfg.DeploymentMode) || !idpSchemeIsCleartext(cfg.IDPHost) {
		return nil
	}

	return fmt.Errorf(
		"DEPLOYMENT_MODE=saas: TLS required for IdP declaration but not configured (set IDP_HOST to an https:// URL)",
	)
}

// isSaaSMode normalizes the deployment mode (case + whitespace) so values like
// "SaaS" or " saas " cannot bypass a gate by string-equality alone. Shared by
// both SaaS gates in this file so the normalization can never drift between them.
func isSaaSMode(deploymentMode string) bool {
	return strings.EqualFold(strings.TrimSpace(deploymentMode), "saas")
}
