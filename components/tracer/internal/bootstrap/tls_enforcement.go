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
	if !strings.EqualFold(strings.TrimSpace(cfg.DeploymentMode), "saas") {
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
