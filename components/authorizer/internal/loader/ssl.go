// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
)

// ErrLoaderSSLDisableInProduction is returned when the caller passes a DSN with
// sslmode=disable while the environment is production-like. The authorizer's
// initial bootstrap path validates SSL mode at buildPostgresDSN() time, but the
// loader accepts raw DSNs via NewPostgresLoaderWithConfig too (tests, embedded
// callers, future wiring) — so enforcement is re-applied here as defense in
// depth. This mirrors the enforcePostgresSSLMode helper in the transaction
// bootstrap package but operates on the parsed URL rather than env vars.
var ErrLoaderSSLDisableInProduction = errors.New("postgres loader refuses sslmode=disable in production-like environments")

// enforceDSNSSLMode parses dsn and returns an error if sslmode=disable is
// specified while envName is production-like. Acceptable DSN formats:
//
//   - URI form: postgres://user:pass@host:port/db?sslmode=require
//   - URI form without sslmode (treated as require by libpq, so accepted)
//   - keyword=value form: "host=... sslmode=disable ..." (parsed by simple
//     token scan; any sslmode token is honored)
//
// Non-production environments (ENV_NAME in the brokersecurity allow-list)
// bypass this check — local dev and test fixtures routinely use sslmode=disable.
func enforceDSNSSLMode(dsn, envName string) error {
	if brokersecurity.IsNonProductionEnvironment(strings.TrimSpace(envName)) {
		return nil
	}

	mode := extractSSLMode(dsn)
	if strings.EqualFold(strings.TrimSpace(mode), "disable") {
		return fmt.Errorf("%w", ErrLoaderSSLDisableInProduction)
	}

	return nil
}

// extractSSLMode returns the sslmode value from either a URI-form DSN or a
// keyword=value DSN. Returns the empty string if absent or unparseable (libpq
// default is prefer, which is SSL-attempted — so absence is acceptable here).
func extractSSLMode(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return ""
	}

	// URI form: postgres://... or postgresql://...
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			if v := parsed.Query().Get("sslmode"); v != "" {
				return v
			}
		}

		return ""
	}

	// keyword=value form: scan tokens for sslmode=...
	for _, tok := range strings.Fields(trimmed) {
		if strings.HasPrefix(strings.ToLower(tok), "sslmode=") {
			return strings.TrimPrefix(tok, "sslmode=")
		}
	}

	return ""
}
