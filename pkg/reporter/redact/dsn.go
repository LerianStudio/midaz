// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redact provides utilities for redacting sensitive data from
// strings before logging or surfacing them in user-facing responses.
//
// The motivation is the operator-visibility / credential-hygiene tradeoff:
// /readyz responses, error messages, and structured logs need enough detail
// for operators to debug TLS failures, deadline-exceeded errors, and
// connection refusals — but they MUST NOT leak userinfo (user:password@host)
// or known-sensitive query parameters (token, api_key, secret, auth).
//
// This package is intentionally narrow:
//   - ConnectionString redacts a single URL-shaped string.
//   - Error redacts URL substrings embedded inside an error message.
//
// It deliberately does NOT touch TLS-relevant query parameters
// (tls=true, ssl=true, sslmode=require) — those are transport posture,
// not secrets, and operators rely on seeing them.
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

// redactedSentinel is returned (or substituted) when the input cannot be
// safely parsed. It MUST NOT echo any portion of the original input —
// a malformed URL might itself be an attack vector or hold a credential
// the parser failed to recognize.
const redactedSentinel = "[redacted: malformed URL]"

// redactedToken is the placeholder used in place of the original
// userinfo / sensitive query value. Single token used everywhere so log
// readers can grep for it.
const redactedToken = "REDACTED"

// sensitiveQueryKeys is the closed set of query parameter names that are
// treated as credentials and replaced with redactedToken. Entries are
// case-insensitive (we lower-case before comparison).
//
// TLS-relevant keys (tls, ssl, sslmode, authSource) are deliberately NOT
// included — they describe transport posture and operators MUST be able
// to see them.
var sensitiveQueryKeys = map[string]struct{}{
	"password":     {},
	"pwd":          {},
	"pass":         {},
	"token":        {},
	"access_token": {},
	"api_key":      {},
	"apikey":       {},
	"secret":       {},
	"auth":         {},
	"credentials":  {},
}

// urlPattern matches the typical net/http.Client error format —
// `Get "URL": underlying error` — and captures the quoted URL so we can
// redact it without disturbing the surrounding error context. Kept
// intentionally simple: a quoted token starting with a scheme followed by
// "://".
var urlPattern = regexp.MustCompile(`"([a-zA-Z][a-zA-Z0-9+.-]*://[^"]+)"`)

// ConnectionString returns a copy of the input with credentials masked.
//
// Behavior:
//   - Empty input → returns empty string.
//   - Malformed URL → returns redactedSentinel ("[redacted: malformed URL]").
//     Never returns the original input on parse failure — callers might
//     surface the result and we must not leak unrecognized secrets.
//   - Valid URL → strips userinfo (replaced with REDACTED) and replaces
//     known-sensitive query parameter values with REDACTED. TLS-relevant
//     query parameters are preserved verbatim.
//
// The function is idempotent: redacting an already-redacted string is
// safe and produces a structurally equivalent result.
func ConnectionString(s string) string {
	if s == "" {
		return ""
	}

	u, err := url.Parse(s)
	if err != nil {
		return redactedSentinel
	}

	// url.Parse is lenient — strings like "://no-scheme" succeed but yield
	// a URL with no scheme/host, which we treat as malformed for our
	// purposes (we cannot meaningfully redact something that doesn't look
	// like a URL).
	if u.Scheme == "" && u.Host == "" && u.Opaque == "" {
		return redactedSentinel
	}

	if u.User != nil {
		// url.User produces userinfo without the trailing colon —
		// "scheme://REDACTED@host" rather than "scheme://REDACTED:@host".
		// Either form would round-trip safely, but the no-colon variant is
		// the standard URL shape operators expect to see in logs.
		u.User = url.User(redactedToken)
	}

	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			if _, sensitive := sensitiveQueryKeys[strings.ToLower(key)]; sensitive {
				q.Set(key, redactedToken)
			}
		}

		u.RawQuery = q.Encode()
	}

	return u.String()
}

// Error redacts an error's message by extracting and redacting any
// connection-string-shaped tokens it contains.
//
// Behavior:
//   - nil error → returns "".
//   - Message contains a quoted "scheme://..." token (the standard
//     net/http.Client error format) → that URL is replaced with its
//     ConnectionString-redacted form, surrounding context preserved.
//   - Message has no URL-shaped substring → returned verbatim.
//
// The function is deliberately conservative: it only substitutes when it
// finds a recognizable URL pattern. False-positive substitution would make
// operator messages strictly worse, not better.
func Error(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()

	return urlPattern.ReplaceAllStringFunc(msg, func(match string) string {
		// match is `"scheme://..."` (with quotes). Strip the quotes,
		// redact, re-wrap.
		inner := strings.TrimPrefix(strings.TrimSuffix(match, `"`), `"`)
		return `"` + ConnectionString(inner) + `"`
	})
}
