// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/redact"
)

// ============================================================================
// TLS detection helpers — Gate 3 implementation.
//
// These helpers expose the function signatures that checks.go uses to populate
// the DependencyCheck.TLS field. The contract is:
//
//   - Distinguish URL schemes (mongodb+srv, amqps, rediss, https) as a
//     reliable TLS signal.
//   - Parse query parameters (tls=true, ssl=true) using url.Parse +
//     url.Values, NEVER strings.Contains. Substring TLS detection is forbidden
//     because URL-encoded params (%3D for "=") and ambiguous keys (e.g.,
//     `xtlsenabled=` matching the substring `tls=`) make it unsafe.
//   - Treat empty input as "not configured, no TLS to detect" → (false, nil)
//     so callers can probe optional dependencies without special-casing nil.
//   - Return an error only when the input is non-empty and malformed, or when
//     the input is missing the scheme that the protocol requires (S3, HTTP).
//   - Errors MUST NOT leak credentials: net/url parse errors include the
//     original URL verbatim (with userinfo) and any direct interpolation of
//     the input string is a credential leak path. The helpers below funnel
//     all such errors through pkg/redact so userinfo and known-sensitive
//     query parameters are masked before the message escapes the package.
// ============================================================================

// wrapParseError converts a url.Parse failure into an error whose message is
// safe to surface in /readyz responses, structured logs, or bootstrap
// fail-fast paths. It strips the original *url.Error wrapper (which embeds
// the raw input) and substitutes a redacted copy of the input alongside the
// underlying error reason, preserving the parser's diagnostic ("invalid
// port", "invalid control character", etc.) without echoing credentials.
//
// Inputs:
//   - kind: short label for the URI shape (e.g. "mongo URI", "amqp URI",
//     "s3 endpoint"), used as a prefix so operators can locate the failure.
//   - uri: the original input. ConnectionString redacts userinfo and known
//     sensitive query params before returning the formatted message.
//   - err: the error returned by url.Parse. The wrapped *url.Error.Err is
//     unwrapped so the message does not double-print the input.
func wrapParseError(kind, uri string, err error) error {
	cause := err

	var ue *url.Error
	if errors.As(err, &ue) {
		cause = ue.Err
	}

	return fmt.Errorf("parse %s %s: %w", kind, redact.ConnectionString(uri), cause)
}

// isQueryParamTrue reports whether the given query map contains key=value
// where value is the literal string "true" (case-insensitive). Any other value
// (including empty, "false", "1", "0") returns false.
//
// Both the key and the value are matched case-insensitively. URIs encountered
// in the wild use a mix of casings (`?tls=true`, `?TLS=True`, `?Ssl=TRUE`,
// etc.) and url.Values.Get is case-sensitive; iterating the map manually
// preserves the documented case-insensitive semantics for ALL key forms,
// not just the canonical lowercase variant.
func isQueryParamTrue(values url.Values, key string) bool {
	for k, vs := range values {
		if !strings.EqualFold(k, key) {
			continue
		}

		for _, v := range vs {
			if strings.EqualFold(v, "true") {
				return true
			}
		}
	}

	return false
}

// DetectMongoTLS reports whether the given MongoDB URI is configured with TLS.
//
// Detection rules:
//   - "mongodb+srv://..." → TLS implicit (always true). The +srv form forces
//     TLS by spec.
//   - "mongodb://..." with query param tls=true OR ssl=true (case-insensitive)
//     → true. Any other value (including tls=false, ssl=false, empty) → false.
//   - Empty input → (false, nil).
//   - Malformed URI → (false, err).
func DetectMongoTLS(uri string) (bool, error) {
	if uri == "" {
		return false, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return false, wrapParseError("mongo URI", uri, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "mongodb+srv" {
		return true, nil
	}

	if scheme != "mongodb" {
		return false, nil
	}

	q := u.Query()
	if isQueryParamTrue(q, "tls") || isQueryParamTrue(q, "ssl") {
		return true, nil
	}

	return false, nil
}

// DetectAMQPTLS reports whether the given AMQP URI uses TLS.
//
// Detection rules:
//   - Scheme "amqps://" → TLS.
//   - Scheme "amqp://" → not TLS.
//   - Empty input → (false, nil).
//   - Malformed URI → (false, err).
//
// Query params are not consulted — AMQP TLS is controlled entirely by scheme.
func DetectAMQPTLS(uri string) (bool, error) {
	if uri == "" {
		return false, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return false, wrapParseError("amqp URI", uri, err)
	}

	return strings.ToLower(u.Scheme) == "amqps", nil
}

// DetectRedisTLS reports whether the given Redis URI uses TLS.
//
// Detection rules:
//   - Scheme "rediss://" (note: two `s`) → TLS.
//   - Scheme "redis://" → not TLS.
//   - Empty input → (false, nil).
//   - Malformed URI → (false, err).
//
// Query params are not consulted — Redis TLS is controlled entirely by scheme.
func DetectRedisTLS(uri string) (bool, error) {
	if uri == "" {
		return false, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return false, wrapParseError("redis URI", uri, err)
	}

	return strings.ToLower(u.Scheme) == "rediss", nil
}

// DetectS3TLS reports whether the given S3-compatible endpoint uses TLS.
//
// Detection rules:
//   - Empty endpoint → (false, nil) (not configured = no TLS to detect).
//   - Scheme "https://" → TLS.
//   - Scheme "http://" → not TLS.
//   - Missing scheme (e.g., "localhost:9000") → (false, err): operators MUST
//     declare http or https explicitly. We refuse to guess.
//   - Malformed URL → (false, err).
func DetectS3TLS(endpoint string) (bool, error) {
	if endpoint == "" {
		return false, nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return false, wrapParseError("s3 endpoint", endpoint, err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return true, nil
	case "http":
		return false, nil
	default:
		return false, fmt.Errorf("s3 endpoint missing http(s) scheme: %s", redact.ConnectionString(endpoint))
	}
}

// DetectHTTPUpstreamTLS reports whether the given HTTP upstream base URL uses
// TLS. This is used for Fetcher, Tenant Manager, and any generic HTTP upstream.
//
// Detection rules:
//   - Empty input → (false, nil).
//   - Scheme "https://" → TLS.
//   - Scheme "http://" → not TLS.
//   - Missing scheme → (false, err): operators MUST declare http or https
//     explicitly.
//   - Malformed URL → (false, err).
func DetectHTTPUpstreamTLS(baseURL string) (bool, error) {
	if baseURL == "" {
		return false, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return false, wrapParseError("http upstream URL", baseURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return true, nil
	case "http":
		return false, nil
	default:
		return false, fmt.Errorf("http upstream URL missing http(s) scheme: %s", redact.ConnectionString(baseURL))
	}
}
