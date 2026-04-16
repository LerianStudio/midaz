// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package dbpool provides shared helpers for PostgreSQL connection pool
// management: DSN sanitisation for error/log output and bootstrap-time
// validation of the aggregate pool budget against server max_connections.
//
// This package exists because pgx and database/sql occasionally echo the
// raw DSN (including password) in error messages. Wrapping DSN-bearing
// errors without first scrubbing leaks credentials into logs, traces, and
// crash dumps. All DSN-bearing error paths in Midaz must route through
// ScrubDSN before formatting.
package dbpool

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// redacted is the placeholder written in place of a real password.
const redacted = "***"

// ErrPoolBudgetExceeded is returned by ValidatePoolBudget when the
// sum(MaxConns * expected_instances) across all declared pools exceeds
// the configured fraction of PostgreSQL max_connections.
var ErrPoolBudgetExceeded = errors.New("aggregate pool budget exceeds postgres max_connections")

// PoolBudget describes a single pool's contribution to the aggregate
// connection count. Name is used only for error messages.
type PoolBudget struct {
	Name              string
	MaxConns          int
	ExpectedInstances int
}

// ScrubDSN strips the password from a PostgreSQL DSN, regardless of
// whether the DSN is in URI form (postgres://user:pass@host/db) or
// keyword form (host=... user=... password=... dbname=...).
//
// The returned string is safe to include in error messages and logs.
// ScrubDSN is intentionally lenient: malformed input returns a best-effort
// redacted string rather than an error, because its purpose is to PREVENT
// leakage, not to validate DSN syntax.
func ScrubDSN(dsn string) string {
	if dsn == "" {
		return ""
	}

	trimmed := strings.TrimSpace(dsn)

	// URI form: postgres://user:password@host:port/db?...
	//
	// We avoid net/url's rebuild path because url.UserPassword
	// percent-encodes the redacted marker (`***` becomes `%2A%2A%2A`).
	// The substring rewrite preserves the exact sentinel readers
	// downstream expect to see.
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		u, err := url.Parse(trimmed)
		if err != nil || u.User == nil {
			return trimmed
		}

		pwd, hasPwd := u.User.Password()
		if !hasPwd {
			return trimmed
		}

		return replaceURIPassword(trimmed, u.User.Username(), pwd)
	}

	// Keyword form: key=value key='value with spaces' pairs.
	// We walk tokens separated by whitespace but respect single-quoted values
	// so password='weird value' is handled correctly.
	return scrubKeywordDSN(trimmed)
}

// replaceURIPassword rewrites the first `user:password@` segment of a
// URI-form DSN to `user:***@`. The net/url parser decodes the password
// percent-escapes for us, but we search for the encoded form in the raw
// input so the substring substitution doesn't misfire on a password
// that happens to match the decoded bytes elsewhere in the URI.
func replaceURIPassword(raw, user, decodedPassword string) string {
	// Locate the `user:` prefix after the scheme.
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 0 {
		return raw
	}

	tail := raw[schemeEnd+3:]

	at := strings.Index(tail, "@")
	if at < 0 {
		return raw
	}

	userinfo := tail[:at]
	colon := strings.Index(userinfo, ":")

	if colon < 0 {
		return raw
	}

	// Preserve the literal user token so we don't hide the fact that
	// the DSN had a username.
	gotUser := userinfo[:colon]
	if gotUser != user && url.QueryEscape(gotUser) != user {
		// Username mismatch — fall back to substring replacement of the
		// decoded password value in the raw URI. This path runs only on
		// highly unusual inputs.
		if decodedPassword != "" {
			return strings.Replace(raw, decodedPassword, redacted, 1)
		}

		return raw
	}

	rewritten := raw[:schemeEnd+3] + gotUser + ":" + redacted + "@" + tail[at+1:]

	return rewritten
}

// scrubKeywordDSN rewrites any `password=...` token to `password=***` while
// preserving token order and surrounding whitespace/quoting. Other tokens
// are untouched.
func scrubKeywordDSN(dsn string) string {
	var builder strings.Builder

	builder.Grow(len(dsn))

	i := 0
	for i < len(dsn) {
		i = copyWhitespace(dsn, i, &builder)
		if i >= len(dsn) {
			break
		}

		keyEnd := scanKey(dsn, i)

		key := dsn[i:keyEnd]
		builder.WriteString(key)

		i = keyEnd

		if i >= len(dsn) || dsn[i] != '=' {
			continue
		}

		builder.WriteByte('=')

		i++

		valStart := i
		i = scanValue(dsn, i)

		if strings.EqualFold(key, "password") {
			builder.WriteString(redacted)
		} else {
			builder.WriteString(dsn[valStart:i])
		}
	}

	return builder.String()
}

func copyWhitespace(dsn string, i int, builder *strings.Builder) int {
	for i < len(dsn) && (dsn[i] == ' ' || dsn[i] == '\t') {
		builder.WriteByte(dsn[i])
		i++
	}

	return i
}

func scanKey(dsn string, i int) int {
	for i < len(dsn) && dsn[i] != '=' && dsn[i] != ' ' && dsn[i] != '\t' {
		i++
	}

	return i
}

// scanValue returns the index past the end of the DSN value starting at i.
// Single-quoted values honour the standard ” escape; unquoted values run
// to the next whitespace.
func scanValue(dsn string, i int) int {
	if i < len(dsn) && dsn[i] == '\'' {
		return scanQuotedValue(dsn, i)
	}

	for i < len(dsn) && dsn[i] != ' ' && dsn[i] != '\t' {
		i++
	}

	return i
}

func scanQuotedValue(dsn string, i int) int {
	// Skip the opening quote.
	i++
	for i < len(dsn) {
		if dsn[i] != '\'' {
			i++

			continue
		}

		// Doubled single-quote is an embedded apostrophe.
		if i+1 < len(dsn) && dsn[i+1] == '\'' {
			i += 2

			continue
		}

		return i + 1
	}

	return i
}

// ScrubDSNInError returns err wrapped such that any occurrence of the
// raw DSN inside the error chain's Error() string is replaced by a
// scrubbed variant. Callers should use this when they suspect a
// downstream library may echo the DSN (pgx historically does).
//
// This is a defence-in-depth helper: it assumes the DSN itself may
// appear verbatim in the error text even when the call site didn't
// explicitly format it in. The function is a no-op when the DSN
// substring is not present in err.Error().
func ScrubDSNInError(err error, dsn string) error {
	if err == nil || dsn == "" {
		return err
	}

	scrubbed := ScrubDSN(dsn)
	if scrubbed == dsn {
		return err
	}

	msg := err.Error()
	if !strings.Contains(msg, dsn) {
		return err
	}

	return fmt.Errorf("%s", strings.ReplaceAll(msg, dsn, scrubbed)) //nolint:err113 // caller context already wrapped upstream
}
