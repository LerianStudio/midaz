// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redact

import (
	"regexp"
	"strings"
)

// secretFieldReplacement is the placeholder substituted in place of any
// sensitive JSON value matched by SecretFields. Chosen so it cannot itself
// re-trigger the matcher (no embedded quotes) — guaranteeing idempotency.
const secretFieldReplacement = `"***REDACTED***"`

// truncationSuffix is appended to strings cut by TruncateForLog. Visually
// distinct from any realistic credential and easy to grep for in logs so
// operators can tell at a glance that the field was elided.
const truncationSuffix = "...[truncated]"

// SensitiveFieldNames is the closed allowlist of JSON keys whose VALUES are
// masked by SecretFields. Exported so operators can inspect, in code, the
// exact set of fields the redactor covers.
//
// Each name is matched case-insensitively against the JSON key as it
// appears on the wire. Exact key matching (not substring) is used so
// neighbours like "non_secret_field" or "my_custom_token_holder" are NOT
// false-positive-redacted.
//
// Coverage rationale (security-reviewer M1+M2):
//   - clientSecret / client_secret — OAuth2 client_credentials request and
//     any echo of it that some authorization servers include in error
//     payloads.
//   - accessToken / access_token / refreshToken / refresh_token — minted
//     JWTs that MUST NOT be persisted in plain text outside the calling
//     process.
//   - password — generic credential field name seen in legacy systems
//     bridged through the M2M path.
//   - secret / token — bare names used by some SDKs.
var SensitiveFieldNames = []string{
	"clientSecret",
	"client_secret",
	"accessToken",
	"access_token",
	"refreshToken",
	"refresh_token",
	"password",
	"secret",
	"token",
}

// secretFieldRegex matches `"<key>" : "<value>"` JSON pairs where <key> is
// any name in SensitiveFieldNames (case-insensitive) and <value> is a
// quoted string. The pattern is intentionally narrow:
//
//   - Key MUST be inside double quotes (so substring keys like
//     "my_custom_token_holder" do not match — the quote acts as a
//     word boundary).
//   - (?i) makes the key case-insensitive.
//   - \s* tolerates whitespace and tabs around the colon (pretty-printed JSON).
//   - The value capture group [^"\\]*(?:\\.[^"\\]*)* honours JSON string
//     escaping rules: it consumes characters that are NOT backslash or
//     quote, plus escaped sequences (\", \\, etc.). This is the standard
//     "match a JSON string content" idiom and avoids the common
//     vulnerability of terminating early on an escaped quote.
//
// NOT covered (by design):
//   - Numeric or null values (`"clientSecret":null`) — these are not
//     credentials by definition.
//   - Multi-line / pretty-printed values with embedded newlines — Go
//     regex `.` does not cross newlines by default; the [^"...] negation
//     does, but a value containing a literal newline is non-conformant
//     JSON. If we ever see one, the value will simply pass through
//     unredacted; we accept that (log-only path, downstream code does
//     not consume the redacted string).
//
// Compiled once at package init for hot-path safety.
var secretFieldRegex = regexp.MustCompile(buildSecretFieldPattern(SensitiveFieldNames))

// buildSecretFieldPattern assembles the regex source from SensitiveFieldNames.
// Kept as a function (not an inlined literal) so the allowlist is the single
// source of truth — adding a new field requires only an entry in the slice.
func buildSecretFieldPattern(names []string) string {
	// Escape the field names defensively in case future additions ever
	// contain regex metacharacters. None of the current entries do, but
	// this future-proofs the build step at negligible cost.
	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = regexp.QuoteMeta(n)
	}

	alternation := strings.Join(escaped, "|")

	// (?i)            case-insensitive
	// "(KEY)"         capture the matched key as group 1 (preserved verbatim in output)
	// \s*:\s*         tolerate whitespace and tabs around the colon
	// "(?:\\.|[^"\\])*"   match a properly-escaped JSON string value
	return `(?i)"(` + alternation + `)"\s*:\s*"(?:\\.|[^"\\])*"`
}

// SecretFields returns a copy of s with the VALUES of any sensitive JSON
// fields (per SensitiveFieldNames) replaced by "***REDACTED***". The keys
// are preserved verbatim — operators need to know the shape of the payload
// even when the values are scrubbed.
//
// Behavior:
//   - Empty input → returns empty string.
//   - Input that does not contain any sensitive field → returned unchanged.
//   - Input containing one or more sensitive fields → those values are
//     replaced; surrounding context is preserved byte-for-byte.
//   - Malformed input that contains no recognizable sensitive field
//     pattern → returned unchanged. The function never panics on
//     adversarial input.
//
// The function is idempotent: applying it twice produces the same result
// as applying it once. The replacement token "***REDACTED***" is
// deliberately chosen so it cannot itself re-match the pattern.
//
// Intended use: sanitising HTTP response bodies, request payloads, and
// arbitrary error strings before they are written to structured logs.
// NOT intended for transformations that downstream code consumes — the
// output is a one-way projection for human/log readers.
func SecretFields(s string) string {
	if s == "" {
		return ""
	}

	return secretFieldRegex.ReplaceAllStringFunc(s, func(match string) string {
		// Recover the key with original casing by re-running the regex on
		// the matched fragment (FindStringSubmatch returns submatches).
		// This preserves the on-the-wire field name (clientSecret vs
		// ClientSecret vs CLIENTSECRET) which matters for log readability.
		sub := secretFieldRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			// Should never happen: ReplaceAllStringFunc only invokes us on
			// matches. Be defensive anyway — return a safely-redacted shape.
			return `"REDACTED":` + secretFieldReplacement
		}

		key := sub[1]

		return `"` + key + `":` + secretFieldReplacement
	})
}

// TruncateForLog returns s if len(s) <= limit, otherwise the first `limit`
// bytes of s followed by "...[truncated]". Operates on bytes, not runes — log
// messages are inherently lossy, and a multi-byte boundary cut is
// acceptable in exchange for a simple, predictable length contract.
//
// limit == 0 yields the suffix alone for any non-empty input, which is the
// "log this field exists but show nothing" mode. Negative limit values are
// treated as 0.
//
// This helper exists alongside SecretFields because the M1 finding asked
// for "truncate + redact": both transforms are independently useful, and
// callers should be able to apply either or both. SecretFields keeps its
// pure-redaction contract; TruncateForLog owns the length concern.
//
// The parameter is named `limit` rather than `max` because the latter
// shadows Go 1.21+ builtin `max()`, which would trip the `predeclared`
// linter and confuse readers grepping for builtin overloads.
func TruncateForLog(s string, limit int) string {
	if s == "" {
		return ""
	}

	if limit < 0 {
		limit = 0
	}

	if len(s) <= limit {
		return s
	}

	return s[:limit] + truncationSuffix
}
