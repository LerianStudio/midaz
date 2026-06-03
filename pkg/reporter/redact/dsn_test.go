// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redact

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConnectionString_TableDriven exercises the canonical redaction rules
// defined for ConnectionString:
//
//   - Empty input returns empty output (no error sentinel).
//   - Malformed URL never returns the original input verbatim — it returns
//     a safe placeholder so callers cannot accidentally surface a raw secret.
//   - Userinfo (user:password@) is masked with the literal "REDACTED".
//   - Known-sensitive query parameters (password, token, api_key, apikey,
//     secret, auth) are replaced with "REDACTED".
//   - TLS-relevant query parameters (tls, ssl, sslmode, authSource) are
//     PRESERVED — the function redacts secrets, not transport posture.
//   - The function is idempotent: redacting an already-redacted string is a
//     no-op (or at worst a structural normalization).
func TestConnectionString_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		expectIn []string // substrings that MUST appear in output
		expectNo []string // substrings that MUST NOT appear in output
		exact    string   // when non-empty, output must equal this exactly
	}{
		{
			name:  "empty input returns empty",
			in:    "",
			exact: "",
		},
		{
			name:     "valid URL with userinfo masks credentials",
			in:       "mongodb://alice:hunter2@db.internal:27017/reporter",
			expectIn: []string{"REDACTED", "db.internal:27017", "/reporter"},
			expectNo: []string{"alice", "hunter2"},
		},
		{
			name:     "valid URL without userinfo unchanged",
			in:       "mongodb://db.internal:27017/reporter",
			expectIn: []string{"mongodb://db.internal:27017/reporter"},
			expectNo: []string{"REDACTED"},
		},
		{
			name:     "malformed URL returns safe sentinel, not the original",
			in:       "://bad-url-no-scheme",
			expectIn: []string{"redacted"},
			expectNo: []string{"bad-url-no-scheme"},
		},
		{
			name:     "URL with password query param redacted",
			in:       "https://api.example.com/v1?password=hunter2&keep=me",
			expectIn: []string{"REDACTED", "keep=me"},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "URL with token query param redacted",
			in:       "https://api.example.com/v1?token=abc123",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"abc123"},
		},
		{
			name:     "URL with api_key and apikey both redacted",
			in:       "https://api.example.com/v1?api_key=abc&apikey=xyz",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"abc", "xyz"},
		},
		{
			name:     "URL with secret param redacted",
			in:       "https://api.example.com/v1?secret=topsecret",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"topsecret"},
		},
		{
			name:     "tls=true is NOT a secret, preserved",
			in:       "mongodb://db.example.com:27017/admin?tls=true",
			expectIn: []string{"tls=true"},
		},
		{
			name:     "ssl and sslmode are NOT secrets, preserved",
			in:       "postgres://db:5432/postgres?ssl=true&sslmode=require",
			expectIn: []string{"ssl=true", "sslmode=require"},
		},
		{
			name:     "userinfo AND query secrets both redacted, tls preserved",
			in:       "mongodb://alice:hunter2@db:27017/reporter?tls=true&password=other",
			expectIn: []string{"REDACTED", "tls=true"},
			expectNo: []string{"alice", "hunter2", "other"},
		},
		{
			name:     "mongodb+srv scheme handled",
			in:       "mongodb+srv://alice:pw@cluster0.example.mongodb.net/admin?tls=true",
			expectIn: []string{"mongodb+srv://", "cluster0.example.mongodb.net", "tls=true", "REDACTED"},
			expectNo: []string{"alice", "pw"},
		},
		{
			name:     "amqps scheme with userinfo redacted",
			in:       "amqps://guest:guest@rabbit.example.com:5671/vhost",
			expectIn: []string{"amqps://", "rabbit.example.com:5671", "/vhost", "REDACTED"},
			expectNo: []string{"guest:guest"},
		},
		{
			name:     "idempotent: already-redacted input stays safe",
			in:       "mongodb://REDACTED:@db:27017/reporter",
			expectIn: []string{"REDACTED", "db:27017"},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "auth query param redacted",
			in:       "https://api.example.com/v1?auth=bearer-token-xyz",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"bearer-token-xyz"},
		},
		{
			name:     "pwd query param redacted",
			in:       "https://api.example.com/v1?pwd=hunter2",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "pass query param redacted",
			in:       "https://api.example.com/v1?pass=hunter2",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "credentials query param redacted",
			in:       "https://api.example.com/v1?credentials=xyz",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"xyz"},
		},
		{
			name:     "access_token query param redacted",
			in:       "https://api.example.com/v1?access_token=abc123",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"abc123"},
		},
		{
			name:     "uppercase PWD query param still redacted (case-insensitive lookup)",
			in:       "https://api.example.com/v1?PWD=hunter2",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "mixed-case Access_Token query param still redacted",
			in:       "https://api.example.com/v1?Access_Token=abc123",
			expectIn: []string{"REDACTED"},
			expectNo: []string{"abc123"},
		},
		{
			name:     "userinfo redacted has no trailing colon",
			in:       "mongodb://alice:hunter2@db.internal:27017/reporter",
			expectIn: []string{"mongodb://REDACTED@db.internal:27017/reporter"},
			expectNo: []string{"alice", "hunter2", "REDACTED:@"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ConnectionString(tt.in)

			if tt.exact != "" || tt.in == "" {
				assert.Equal(t, tt.exact, got)
				return
			}

			for _, want := range tt.expectIn {
				assert.Contains(t, got, want, "expected %q in output %q", want, got)
			}

			for _, banned := range tt.expectNo {
				assert.NotContains(t, got, banned, "did NOT expect %q in output %q", banned, got)
			}
		})
	}
}

// TestConnectionString_MalformedNeverReturnsOriginal is a security
// invariant: any input that fails to parse must NOT echo the input back —
// callers might surface this string, and the input could itself be an
// attack vector or hold a credential we failed to recognize.
func TestConnectionString_MalformedNeverReturnsOriginal(t *testing.T) {
	t.Parallel()

	for _, malformed := range []string{
		"://no-scheme",
		"http://[::1:bad",
	} {
		got := ConnectionString(malformed)
		assert.NotEqual(t, malformed, got,
			"malformed input %q must not be echoed back", malformed)
		assert.Contains(t, strings.ToLower(got), "redacted",
			"malformed sentinel must include the word redacted")
	}
}

// TestError_NilReturnsEmpty verifies that a nil error is normalized to "".
func TestError_NilReturnsEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", Error(nil))
}

// TestError_PreservesNonURLMessages verifies that messages without
// URL-shaped substrings pass through unchanged. We must not over-redact:
// false-positive substitution would make operator messages worse, not
// better.
func TestError_PreservesNonURLMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "plain message", err: errors.New("connection refused")},
		{name: "error with code", err: errors.New("ECONNREFUSED: connection refused")},
		{name: "wrapped error", err: fmt.Errorf("context: %w", errors.New("deadline exceeded"))},
		{name: "tls error", err: errors.New("tls: failed to verify certificate")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.err.Error(), Error(tt.err))
		})
	}
}

// TestError_RedactsEmbeddedURL verifies that error messages that embed a
// connection-string-shaped token (typical of net/http.Client errors —
// `Get "URL": underlying error`) get the embedded URL redacted while the
// surrounding context is preserved.
func TestError_RedactsEmbeddedURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expectIn []string
		expectNo []string
	}{
		{
			name:     "net/http style error with userinfo",
			err:      errors.New(`Get "https://alice:hunter2@api.example.com/readyz": dial tcp: connection refused`),
			expectIn: []string{"REDACTED", "api.example.com", "connection refused"},
			expectNo: []string{"alice", "hunter2"},
		},
		{
			name:     "net/http style error with token query",
			err:      errors.New(`Get "https://api.example.com/v1?token=abc123": context deadline exceeded`),
			expectIn: []string{"REDACTED", "api.example.com", "context deadline exceeded"},
			expectNo: []string{"abc123"},
		},
		{
			name:     "no URL means no redaction substitution",
			err:      errors.New("dial tcp 10.0.0.5:27017: connection refused"),
			expectIn: []string{"dial tcp 10.0.0.5:27017", "connection refused"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Error(tt.err)

			for _, want := range tt.expectIn {
				assert.Contains(t, got, want, "expected %q in %q", want, got)
			}

			for _, banned := range tt.expectNo {
				assert.NotContains(t, got, banned, "did NOT expect %q in %q", banned, got)
			}
		})
	}
}
