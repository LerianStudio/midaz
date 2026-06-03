// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecretFields_MasksClientSecret verifies the canonical OAuth2
// client_credentials shape: the response body or request payload contains
// a "clientSecret" field whose string value MUST never appear in logs.
//
// Both camelCase ("clientSecret") and snake_case ("client_secret") are
// covered because OAuth2 servers vary on the wire convention.
func TestSecretFields_MasksClientSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		expectIn []string
		expectNo []string
	}{
		{
			name:     "camelCase clientSecret",
			in:       `{"error":"invalid","clientSecret":"hunter2"}`,
			expectIn: []string{`"clientSecret":"***REDACTED***"`, `"error":"invalid"`},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "snake_case client_secret",
			in:       `{"client_secret":"hunter2","grant_type":"client_credentials"}`,
			expectIn: []string{`"client_secret":"***REDACTED***"`, `"grant_type":"client_credentials"`},
			expectNo: []string{"hunter2"},
		},
		{
			name:     "value with special characters preserved as redacted",
			in:       `{"clientSecret":"p@ss/w0rd+with=special&chars"}`,
			expectIn: []string{`"clientSecret":"***REDACTED***"`},
			expectNo: []string{"p@ss", "w0rd", "special"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(tt.in)

			for _, want := range tt.expectIn {
				assert.Contains(t, got, want, "expected %q in %q", want, got)
			}

			for _, banned := range tt.expectNo {
				assert.NotContains(t, got, banned, "did NOT expect %q in %q", banned, got)
			}
		})
	}
}

// TestSecretFields_MasksAccessToken covers OAuth2 token-exchange responses.
// The plugin-auth token endpoint returns "accessToken" but Casdoor and other
// OAuth2 providers commonly use "access_token". Both must be masked when
// they happen to land inside a logged response body.
func TestSecretFields_MasksAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "camelCase accessToken",
			in:   `{"accessToken":"eyJhbGc.payload.sig","tokenType":"Bearer"}`,
			want: "eyJhbGc",
		},
		{
			name: "snake_case access_token",
			in:   `{"access_token":"eyJhbGc.payload.sig","token_type":"Bearer"}`,
			want: "eyJhbGc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(tt.in)
			assert.NotContains(t, got, tt.want, "JWT prefix %q must be redacted", tt.want)
			assert.Contains(t, got, "***REDACTED***", "redaction marker must be present")
		})
	}
}

// TestSecretFields_MasksMultipleFields verifies that a payload carrying more
// than one sensitive field gets all of them redacted in a single pass.
// This is the realistic OAuth2 error-echo scenario: a malformed request can
// produce a response body that contains both clientSecret AND the original
// access_token, both must be scrubbed.
func TestSecretFields_MasksMultipleFields(t *testing.T) {
	t.Parallel()

	in := `{"clientSecret":"sec1","access_token":"tok1","refresh_token":"refresh1","password":"pw1"}`

	got := SecretFields(in)

	// All sensitive values must be gone.
	for _, banned := range []string{"sec1", "tok1", "refresh1", "pw1"} {
		assert.NotContains(t, got, banned, "value %q must be redacted in %q", banned, got)
	}

	// All field keys must be preserved (operators need to see the SHAPE
	// of what was logged — the absence of the field is itself debug signal).
	for _, key := range []string{
		`"clientSecret":`, `"access_token":`, `"refresh_token":`, `"password":`,
	} {
		assert.Contains(t, got, key, "key %q must be preserved in %q", key, got)
	}

	// Exactly four redactions should have happened.
	assert.Equal(t, 4, strings.Count(got, "***REDACTED***"),
		"expected one REDACTED marker per sensitive field, got %q", got)
}

// TestSecretFields_CaseInsensitive verifies that the matcher does not depend
// on the canonical case of the field name. OAuth2 servers and SDKs vary;
// case-insensitive matching is the safe default.
func TestSecretFields_CaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"ClientSecret":"hunter2"}`,
		`{"CLIENTSECRET":"hunter2"}`,
		`{"clientsecret":"hunter2"}`,
		`{"Client_Secret":"hunter2"}`,
		`{"ACCESS_TOKEN":"abc"}`,
		`{"AccessToken":"abc"}`,
	}

	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(in)
			assert.NotContains(t, got, "hunter2", "case variant must still redact")
			assert.NotContains(t, got, "abc", "case variant must still redact")
			assert.Contains(t, got, "***REDACTED***")
		})
	}
}

// TestSecretFields_PreservesNonSensitiveJSON verifies that we do NOT over-
// redact: non-sensitive keys must be returned verbatim. False-positive
// redaction would make operator messages strictly worse.
func TestSecretFields_PreservesNonSensitiveJSON(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"error":"invalid_grant","error_description":"client not found"}`,
		`{"status":"failed","code":"AUT-1003"}`,
		`{"tenant_id":"acme","target_service":"fetcher"}`,
		// Edge case: a field that COINCIDENTALLY contains "secret" or "token"
		// as a substring but is not on our allowlist must pass through.
		`{"non_secret_field":"keep_me","my_custom_token_holder":"keep_me_too"}`,
	}

	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(in)
			assert.Equal(t, in, got, "non-sensitive JSON must be unchanged")
		})
	}
}

// TestSecretFields_HandlesMalformedJSON verifies that the function never
// panics or hangs on adversarial / malformed input. We do NOT make any
// guarantee about WHAT comes out for malformed input — only that the
// function returns. This matches the dsn package precedent.
func TestSecretFields_HandlesMalformedJSON(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"{",
		"}",
		"not json at all",
		`{"clientSecret":}`,
		`{"clientSecret":"unterminated`,
		`{"clientSecret":null}`,
		`{"clientSecret":123}`,
		strings.Repeat(`{"clientSecret":"x"},`, 100),
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			// The only assertion is "does not panic".
			_ = SecretFields(in)
		})
	}
}

// TestSecretFields_Idempotent verifies that running SecretFields on its own
// output is a no-op. Callers might apply the redactor to already-sanitized
// strings (e.g., when concatenating logs from upstream tools) and we must
// not duplicate or corrupt the REDACTED marker.
func TestSecretFields_Idempotent(t *testing.T) {
	t.Parallel()

	in := `{"clientSecret":"hunter2","access_token":"abc"}`

	once := SecretFields(in)
	twice := SecretFields(once)

	assert.Equal(t, once, twice, "second application must be identity")
}

// TestSecretFields_WhitespaceTolerant verifies the regex matches even when
// JSON pretty-printers or upstream code introduce whitespace around the
// colon. Most OAuth2 servers compact their responses, but the JWT/OIDC
// space is full of pretty-printed examples that could land in logs.
func TestSecretFields_WhitespaceTolerant(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"clientSecret" : "hunter2"}`,
		`{"clientSecret":  "hunter2"}`,
		`{"clientSecret"  :"hunter2"}`,
		`{"clientSecret"	:	"hunter2"}`, // tabs
	}

	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(in)
			assert.NotContains(t, got, "hunter2", "whitespace variant must redact")
			assert.Contains(t, got, "***REDACTED***")
		})
	}
}

// TestTruncateForLog covers the truncation helper used in the M1 fix path.
// The contract: leave strings ≤ max unchanged, suffix anything longer with
// "...[truncated]" so log readers can tell at a glance the value was cut.
func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{
			name: "shorter than max unchanged",
			in:   "abc",
			max:  10,
			want: "abc",
		},
		{
			name: "exactly max unchanged",
			in:   "abcdefghij",
			max:  10,
			want: "abcdefghij",
		},
		{
			name: "longer than max gets suffix",
			in:   "abcdefghijklmnop",
			max:  10,
			want: "abcdefghij...[truncated]",
		},
		{
			name: "empty input unchanged",
			in:   "",
			max:  10,
			want: "",
		},
		{
			name: "max zero returns suffix-only for non-empty input",
			in:   "anything",
			max:  0,
			want: "...[truncated]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := TruncateForLog(tt.in, tt.max)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSensitiveFieldNames_OperatorVisibility asserts the exported sentinel
// list of redacted field names is non-empty and contains the values our
// security model relies on. This guards against accidental deletion of an
// entry during future edits — the test will RED if someone removes
// "clientSecret" or "access_token" from the allowlist.
func TestSensitiveFieldNames_OperatorVisibility(t *testing.T) {
	t.Parallel()

	required := []string{
		"clientSecret", "client_secret",
		"accessToken", "access_token",
		"refreshToken", "refresh_token",
		"password", "secret", "token",
	}

	for _, want := range required {
		assert.Contains(t, SensitiveFieldNames, want,
			"field %q must remain in the redaction allowlist", want)
	}
}

// TestSecretFields_MasksBareFields complements
// TestSensitiveFieldNames_OperatorVisibility by exercising the redaction
// behavior end-to-end for the "bare" allowlist entries — "secret", "token",
// and "password" — not just compound names such as "clientSecret" or
// "access_token". Without this, a regression in the regex (e.g., a future
// edit that scopes the matcher to compound names only) would silently slip
// past the meta-level allowlist assertion above.
func TestSecretFields_MasksBareFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         string
		banned     string
		expectIn   string
		expectKeep string // non-sensitive sibling that must survive the redactor
	}{
		{
			name:       "bare secret",
			in:         `{"secret":"hunter2","other":"keep-me"}`,
			banned:     "hunter2",
			expectIn:   `"secret":"***REDACTED***"`,
			expectKeep: `"other":"keep-me"`,
		},
		{
			name:       "bare token",
			in:         `{"token":"eyJhbGc.payload.sig","kind":"access"}`,
			banned:     "eyJhbGc",
			expectIn:   `"token":"***REDACTED***"`,
			expectKeep: `"kind":"access"`,
		},
		{
			name:       "bare password",
			in:         `{"password":"p@ss/w0rd","username":"alice"}`,
			banned:     "p@ss",
			expectIn:   `"password":"***REDACTED***"`,
			expectKeep: `"username":"alice"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SecretFields(tt.in)
			assert.NotContains(t, got, tt.banned,
				"bare-field value %q must be redacted in %q", tt.banned, got)
			assert.Contains(t, got, tt.expectIn,
				"expected %q in %q", tt.expectIn, got)
			// Asserting non-sensitive sibling preservation catches a
			// regression where the redactor over-eagerly drops content
			// instead of just masking the sensitive value.
			assert.Contains(t, got, tt.expectKeep,
				"non-sensitive sibling %q must survive redaction in %q",
				tt.expectKeep, got)
		})
	}
}
