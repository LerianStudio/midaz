// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
)

// TLS detection helpers used by the /readyz handler to populate the per-dep
// `tls` field per the canonical Lerian /readyz contract.
//
// Scope: Tracer's /readyz cycle is single-tenant, so only Postgres TLS
// detection lives here.
//
// Contract:
//
//   - empty connection string ⇒ (false, nil) — dep is not configured.
//   - malformed DSN ⇒ (false, err) — the parse error is propagated so the
//     caller can log + emit the error string into the readyz response.
//   - parse succeeds ⇒ (bool, nil) — bool reflects the protocol-specific
//     posture (sslmode parsing only).
//
// CRITICAL: TLS detection is structural — it relies exclusively on
// `net/url.Parse` (and `url.Values.Get` for query params). Substring matching
// against the raw connection string (e.g. `strings.Contains(uri, "tls=true")`)
// is FORBIDDEN per anti-pattern N4 of the dev-readyz skill: it produces
// false-positives on user data (database names, paths, hostnames) that
// happen to contain the literal "tls=true". Live-connection inspection
// (e.g. reflection on TLSConfig fields, `conn.TLSConnectionState()`) is also
// forbidden per anti-pattern N5: the readyz path must not depend on a live
// connection just to report posture.

// detectPostgresTLS reads the libpq connection string and returns whether
// TLS is configured. PostgreSQL accepts two DSN formats — both are handled:
//
//  1. URL form: `postgres://user:pass@host/db?sslmode=require`
//  2. keyword/value form: `host=... user=... sslmode=require`
//
// Per the libpq spec, `sslmode` defaults to `prefer` when omitted, but tracer
// always sets it explicitly (see internal/bootstrap/config.go:initPostgresConnection).
// We treat any non-empty `sslmode` other than `disable` as TLS=true to align
// with operator expectations.
//
// Tokenization note: the keyword/value branch splits the DSN on whitespace
// (`strings.Fields`) and inspects each token for a `sslmode=` prefix. This is
// STRUCTURAL parsing of a defined grammar (libpq keyword/value DSN), NOT
// substring matching against arbitrary user content — anti-pattern N4
// targets the latter. The distinction is critical: a substring search for
// `"sslmode="` against the WHOLE DSN would match values like
// `dbname=my_sslmode_test`. Token-by-token prefix matching does not.
func detectPostgresTLS(dsn string) (bool, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return false, nil
	}

	// URL form is identified by an explicit scheme prefix. Anything else falls
	// back to keyword/value parsing. Match scheme case-insensitively per
	// RFC 3986 (scheme is case-insensitive); libpq accepts any case.
	lowerDSN := strings.ToLower(dsn)
	if strings.HasPrefix(lowerDSN, "postgres://") || strings.HasPrefix(lowerDSN, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return false, fmt.Errorf("postgres tls detect: parse url: %w", err)
		}

		mode := u.Query().Get("sslmode")

		return postgresSSLModeIsTLS(mode), nil
	}

	// Keyword/value form. Tokenize by whitespace and parse each `k=v` pair.
	// NOTE: this does NOT handle libpq's quoted values with embedded spaces
	// (those would be split by strings.Fields), but tracer's buildPostgresDSN
	// only ever quotes individual whitespace-free values, so this is safe in
	// practice. We strip surrounding single quotes from the value to support
	// the libpq escaping introduced in buildPostgresDSN.
	for _, tok := range strings.Fields(dsn) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok {
			continue
		}

		// libpq treats keyword names as lower-case; normalize to be safe.
		if strings.ToLower(strings.TrimSpace(k)) == "sslmode" {
			v = strings.TrimSpace(v)
			// Strip outer libpq quoting if present.
			if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
				v = v[1 : len(v)-1]
			}

			return postgresSSLModeIsTLS(v), nil
		}
	}

	// No sslmode token observed in keyword/value form ⇒ default to false. The
	// libpq default is `prefer` (which would attempt TLS), but tracer always
	// sets the field explicitly, so reaching this branch in production
	// indicates a misconfiguration the operator should fix.
	return false, nil
}

// postgresSSLModeIsTLS centralizes the rule that ANY non-empty sslmode other
// than "disable" is TLS-enabled. libpq accepts: disable, allow, prefer,
// require, verify-ca, verify-full. Only "disable" is plaintext.
//
// Normalization: case- and whitespace-insensitive — `" DISABLE "` matches
// `"disable"`. This prevents bypass via stray surrounding whitespace or
// uppercase env values.
func postgresSSLModeIsTLS(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))

	return mode != "" && mode != "disable"
}
