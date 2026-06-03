// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectMongoTLS verifies the MongoDB URI TLS detection contract:
//   - Scheme mongodb+srv:// → TLS implicit (always true).
//   - Scheme mongodb:// with tls=true OR ssl=true (case-insensitive) → true.
//   - Anything else (including tls=false, ssl=false) → false.
//   - Empty input → (false, nil).
//   - Malformed input → (false, err).
//
// The contract MUST be satisfied via url.Parse + Query().Get(...) — substring
// matching is forbidden. Test cases include URL-encoded params and
// substring-ambiguous URIs to lock that down.
func TestDetectMongoTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		uri       string
		wantTLS   bool
		wantError bool
	}{
		{
			name:    "empty string returns false without error",
			uri:     "",
			wantTLS: false,
		},
		{
			name:    "mongodb+srv scheme is implicit TLS",
			uri:     "mongodb+srv://user:pass@cluster.example.net/mydb",
			wantTLS: true,
		},
		{
			name:    "mongodb+srv with tls=false query is still TLS (scheme wins)",
			uri:     "mongodb+srv://cluster.example.net/mydb?tls=false",
			wantTLS: true,
		},
		{
			name:    "mongodb scheme with tls=true returns true",
			uri:     "mongodb://host:27017/mydb?tls=true",
			wantTLS: true,
		},
		{
			name:    "mongodb scheme with ssl=true returns true",
			uri:     "mongodb://host:27017/mydb?ssl=true",
			wantTLS: true,
		},
		{
			name:    "mongodb scheme with tls=true uppercase TRUE returns true",
			uri:     "mongodb://host:27017/mydb?tls=TRUE",
			wantTLS: true,
		},
		{
			name:    "mongodb scheme with tls=False returns false (case-insensitive false)",
			uri:     "mongodb://host:27017/mydb?tls=False",
			wantTLS: false,
		},
		{
			name:    "mongodb scheme with no query params returns false",
			uri:     "mongodb://host:27017/mydb",
			wantTLS: false,
		},
		{
			name:    "mongodb scheme with explicit tls=false returns false",
			uri:     "mongodb://host:27017/mydb?tls=false",
			wantTLS: false,
		},
		{
			name:    "mongodb scheme with explicit ssl=false returns false",
			uri:     "mongodb://host:27017/mydb?ssl=false",
			wantTLS: false,
		},
		{
			name: "ambiguous substring xtlsenabled=false&tls=true → true (url.Parse disambiguates)",
			// strings.Contains(uri, "tls=") would match "xtlsenabled=", but
			// url.Parse correctly looks at distinct query keys.
			uri:     "mongodb://host:27017/mydb?xtlsenabled=false&tls=true",
			wantTLS: true,
		},
		{
			name: "ambiguous substring tls=false embedded with ssl=true → true (ssl wins)",
			// strings.Contains for "tls=true" would miss this; we rely on
			// url.Parse to evaluate ssl correctly.
			uri:     "mongodb://host:27017/mydb?tls=false&ssl=true",
			wantTLS: true,
		},
		{
			name: "URL-encoded ?tls%3Dtrue is a single literal key, NOT TLS",
			// %3D is "=", so the literal key is "tls=true" with empty value.
			// Substring matching on "tls=true" would mistakenly mark this as
			// TLS. url.Parse correctly treats the whole thing as a key with
			// no "=" separator, so neither tls nor ssl is set.
			uri:     "mongodb://host:27017/mydb?tls%3Dtrue",
			wantTLS: false,
		},
		{
			name: "tls value with extra param noise still parses correctly",
			uri:  "mongodb://host:27017/mydb?retryWrites=true&tls=true&w=majority",
			// Multiple legitimate params with tls=true → TLS.
			wantTLS: true,
		},
		{
			name: "non-mongo scheme parses but returns false (no TLS to detect)",
			// e.g., the user accidentally passed an HTTP URL where a mongo URI
			// was expected. We don't error — we just say "no mongo TLS here".
			uri:     "http://host:27017/db?tls=true",
			wantTLS: false,
		},
		{
			name:      "malformed URI with invalid port returns error",
			uri:       "mongodb://host:abcd/x",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "malformed URI with control char returns error",
			uri:       "mongodb://host\x7f/db",
			wantTLS:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectMongoTLS(tt.uri)
			if tt.wantError {
				require.Error(t, err)
				assert.False(t, gotTLS, "TLS must be false when an error is returned")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// TestDetectAMQPTLS verifies the AMQP URI TLS detection contract:
//   - Scheme amqps:// → TLS.
//   - Scheme amqp:// → not TLS.
//   - Empty input → (false, nil).
//   - Malformed input → (false, err).
//   - Substring matching is forbidden — url.Parse is the only way.
func TestDetectAMQPTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		uri       string
		wantTLS   bool
		wantError bool
	}{
		{
			name:    "empty string returns false without error",
			uri:     "",
			wantTLS: false,
		},
		{
			name:    "amqps scheme returns true",
			uri:     "amqps://user:pass@rabbit.example.com:5671/",
			wantTLS: true,
		},
		{
			name:    "amqp scheme returns false",
			uri:     "amqp://user:pass@rabbit.example.com:5672/",
			wantTLS: false,
		},
		{
			name:    "AMQPS uppercase scheme returns true (case-insensitive)",
			uri:     "AMQPS://rabbit.example.com:5671/",
			wantTLS: true,
		},
		{
			name: "amqp with bogus tls=true query does NOT mark TLS",
			// The scheme is the only signal for AMQP; query params are
			// ignored. Substring matching would have mistakenly returned true
			// here because of the literal "tls=true" in the URI.
			uri:     "amqp://rabbit.example.com:5672/?tls=true",
			wantTLS: false,
		},
		{
			name: "amqp with substring-ambiguous query is still false",
			// "?xamqps=true" must NOT trip a substring check on "amqps".
			uri:     "amqp://rabbit.example.com:5672/?xamqps=true",
			wantTLS: false,
		},
		{
			name:    "amqps with vhost path retains TLS",
			uri:     "amqps://user:pass@rabbit.example.com:5671/myvhost",
			wantTLS: true,
		},
		{
			name:      "malformed URI with bad port returns error",
			uri:       "amqps://rabbit:notaport/",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "URI with control character returns error",
			uri:       "amqps://rabbit\x7f.example.com/",
			wantTLS:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectAMQPTLS(tt.uri)
			if tt.wantError {
				require.Error(t, err)
				assert.False(t, gotTLS)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// TestDetectRedisTLS verifies the Redis URI TLS detection contract:
//   - Scheme rediss:// (two `s`) → TLS.
//   - Scheme redis:// → not TLS.
//   - Empty input → (false, nil).
//   - Malformed input → (false, err).
//
// The "rediss" vs "redis" typo trap is explicitly covered.
func TestDetectRedisTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		uri       string
		wantTLS   bool
		wantError bool
	}{
		{
			name:    "empty string returns false without error",
			uri:     "",
			wantTLS: false,
		},
		{
			name:    "rediss scheme (two s) returns true",
			uri:     "rediss://valkey.example.com:6380/0",
			wantTLS: true,
		},
		{
			name:    "redis scheme returns false",
			uri:     "redis://valkey.example.com:6379/0",
			wantTLS: false,
		},
		{
			name:    "REDISS uppercase scheme returns true (case-insensitive)",
			uri:     "REDISS://valkey.example.com:6380/0",
			wantTLS: true,
		},
		{
			name: "redis with tls=true query does NOT mark TLS (scheme is the only signal)",
			// Substring matching on "tls=true" would mistakenly return true
			// here. The Redis URL spec only uses the scheme.
			uri:     "redis://valkey.example.com:6379/0?tls=true",
			wantTLS: false,
		},
		{
			name: "redis with substring-ambiguous query is still false",
			// "?xrediss=1" must NOT trip a substring check on "rediss".
			uri:     "redis://valkey.example.com:6379/0?xrediss=1",
			wantTLS: false,
		},
		{
			name:    "rediss with auth + db path retains TLS",
			uri:     "rediss://user:pass@valkey.example.com:6380/3",
			wantTLS: true,
		},
		{
			name:      "malformed URI with bad port returns error",
			uri:       "rediss://valkey:notaport/",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "URI with control character returns error",
			uri:       "redis://val\x7fkey.example.com/",
			wantTLS:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectRedisTLS(tt.uri)
			if tt.wantError {
				require.Error(t, err)
				assert.False(t, gotTLS)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// TestDetectS3TLS verifies the S3 endpoint TLS detection contract:
//   - Empty endpoint → (false, nil) (not configured = no TLS to detect).
//   - Scheme https:// → TLS.
//   - Scheme http:// → not TLS.
//   - No scheme (e.g., "localhost:9000") → (false, err): operators MUST be
//     explicit; we cannot guess.
//   - Malformed input → (false, err).
func TestDetectS3TLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		endpoint  string
		wantTLS   bool
		wantError bool
	}{
		{
			name:     "empty endpoint returns false without error",
			endpoint: "",
			wantTLS:  false,
		},
		{
			name:     "https endpoint returns true",
			endpoint: "https://s3.amazonaws.com",
			wantTLS:  true,
		},
		{
			name:     "http endpoint returns false",
			endpoint: "http://seaweedfs:8333",
			wantTLS:  false,
		},
		{
			name:     "HTTPS uppercase scheme returns true (case-insensitive)",
			endpoint: "HTTPS://s3.amazonaws.com",
			wantTLS:  true,
		},
		{
			name: "https with bogus tls=false query is still TLS (scheme wins)",
			// Substring matching on "tls=false" would return false here.
			endpoint: "https://s3.amazonaws.com/?tls=false",
			wantTLS:  true,
		},
		{
			name: "http with substring-ambiguous query is still not TLS",
			// "?xhttps=1" must NOT trip a substring check on "https".
			endpoint: "http://seaweedfs:8333/?xhttps=1",
			wantTLS:  false,
		},
		{
			name:      "endpoint without scheme returns error",
			endpoint:  "localhost:9000",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "bare hostname without scheme returns error",
			endpoint:  "s3.amazonaws.com",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "malformed endpoint with bad port returns error",
			endpoint:  "https://s3.amazonaws.com:notaport",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "endpoint with control char returns error",
			endpoint:  "https://s3\x7f.example.com/",
			wantTLS:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectS3TLS(tt.endpoint)
			if tt.wantError {
				require.Error(t, err)
				assert.False(t, gotTLS)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// TestDetectHTTPUpstreamTLS verifies HTTP upstream TLS detection (Fetcher,
// Tenant Manager, generic upstreams):
//   - Scheme https:// → TLS.
//   - Scheme http:// → not TLS.
//   - Empty input → (false, nil).
//   - No scheme → (false, err): we cannot guess.
//   - Malformed input → (false, err).
func TestDetectHTTPUpstreamTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseURL   string
		wantTLS   bool
		wantError bool
	}{
		{
			name:    "empty string returns false without error",
			baseURL: "",
			wantTLS: false,
		},
		{
			name:    "https URL returns true",
			baseURL: "https://fetcher.example.com",
			wantTLS: true,
		},
		{
			name:    "http URL returns false",
			baseURL: "http://fetcher.example.com",
			wantTLS: false,
		},
		{
			name:    "HTTPS uppercase scheme returns true (case-insensitive)",
			baseURL: "HTTPS://fetcher.example.com",
			wantTLS: true,
		},
		{
			name: "https with bogus tls=false query is still TLS (scheme wins)",
			// Substring matching would mistakenly return false here.
			baseURL: "https://fetcher.example.com/?tls=false",
			wantTLS: true,
		},
		{
			name: "http with substring-ambiguous query is still not TLS",
			// "?xhttps=1" must NOT trip a substring check on "https".
			baseURL: "http://fetcher.example.com/?xhttps=1",
			wantTLS: false,
		},
		{
			name:    "https with path and query retains TLS",
			baseURL: "https://fetcher.example.com/api/v1/jobs?status=pending",
			wantTLS: true,
		},
		{
			name:      "URL without scheme returns error",
			baseURL:   "fetcher.example.com",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "bare host:port without scheme returns error",
			baseURL:   "fetcher.example.com:8080",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "malformed URL with bad port returns error",
			baseURL:   "https://fetcher.example.com:notaport",
			wantTLS:   false,
			wantError: true,
		},
		{
			name:      "URL with control char returns error",
			baseURL:   "https://fetcher\x7f.example.com/",
			wantTLS:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectHTTPUpstreamTLS(tt.baseURL)
			if tt.wantError {
				require.Error(t, err)
				assert.False(t, gotTLS)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// TestDetectMongoTLS_CaseInsensitiveQueryKey verifies that query-parameter
// key matching is case-insensitive. URIs in the wild use a mix of casings
// (?TLS=true, ?Ssl=TRUE, ?TlS=True). url.Values.Get is case-sensitive, so
// the helper iterates the map manually with strings.EqualFold to honor the
// documented case-insensitive contract for ALL key forms.
func TestDetectMongoTLS_CaseInsensitiveQueryKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		wantTLS bool
	}{
		{
			name:    "uppercase TLS key with lowercase value",
			uri:     "mongodb://host:27017/db?TLS=true",
			wantTLS: true,
		},
		{
			name:    "uppercase SSL key with uppercase value",
			uri:     "mongodb://host:27017/db?Ssl=TRUE",
			wantTLS: true,
		},
		{
			name:    "mixed-case TLS key with mixed-case value",
			uri:     "mongodb://host:27017/db?TlS=True",
			wantTLS: true,
		},
		{
			name:    "lowercase tls with explicit false",
			uri:     "mongodb://host:27017/db?tls=false",
			wantTLS: false,
		},
		{
			name:    "uppercase TLS=false explicitly false",
			uri:     "mongodb://host:27017/db?TLS=false",
			wantTLS: false,
		},
		{
			name:    "uppercase TLS with non-true value is false (only true matches)",
			uri:     "mongodb://host:27017/db?TlS=invalid",
			wantTLS: false,
		},
		{
			name:    "uppercase TLS=1 is not true (only literal true matches)",
			uri:     "mongodb://host:27017/db?TLS=1",
			wantTLS: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTLS, err := DetectMongoTLS(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}

// Detection error redaction. url.Parse errors include the input URL
// verbatim (see net/url.Error.Error()), so a malformed URI containing
// userinfo or sensitive query tokens would leak credentials into error
// strings that surface in /readyz responses, structured logs, or
// bootstrap fail-fast messages. Each Detect*TLS helper must produce an
// error whose text contains neither the credentials nor the raw input.

// secretsLeakChecker verifies an error string does not surface known
// secret substrings. Centralized so the per-detector tests stay terse
// and the assertion order is uniform.
func assertNoSecretsLeaked(t *testing.T, err error, secrets ...string) {
	t.Helper()

	require.Error(t, err)

	got := err.Error()
	for _, s := range secrets {
		assert.NotContains(t, got, s,
			"error string MUST NOT contain secret %q (got %q)", s, got)
	}
}

func TestDetectMongoTLS_MalformedURI_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	// invalid port forces url.Parse to fail; userinfo is in the input.
	_, err := DetectMongoTLS("mongodb://alice:hunter2@host:abcd/db?token=topsecret")
	assertNoSecretsLeaked(t, err, "hunter2", "topsecret")

	// control character — different parse failure path, same redaction
	// requirement.
	_, err = DetectMongoTLS("mongodb://alice:hunter2@host\x7f/db")
	assertNoSecretsLeaked(t, err, "hunter2")
}

func TestDetectAMQPTLS_MalformedURI_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	_, err := DetectAMQPTLS("amqp://alice:hunter2@host:abcd/")
	assertNoSecretsLeaked(t, err, "hunter2")
}

func TestDetectRedisTLS_MalformedURI_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	_, err := DetectRedisTLS("redis://alice:hunter2@host:abcd/0")
	assertNoSecretsLeaked(t, err, "hunter2")
}

func TestDetectS3TLS_MalformedURI_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	// Malformed parse path.
	_, err := DetectS3TLS("https://alice:hunter2@bad-host[broken")
	assertNoSecretsLeaked(t, err, "hunter2")

	// Missing-scheme path: previously formatted endpoint via %q which
	// leaked any embedded credentials. This case forces the
	// non-http/https branch.
	_, err = DetectS3TLS("ftp://alice:hunter2@s3.example.com")
	assertNoSecretsLeaked(t, err, "hunter2")
}

func TestDetectHTTPUpstreamTLS_MalformedURI_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	_, err := DetectHTTPUpstreamTLS("https://alice:hunter2@bad-host[broken")
	assertNoSecretsLeaked(t, err, "hunter2")

	_, err = DetectHTTPUpstreamTLS("ftp://alice:hunter2@upstream.example.com")
	assertNoSecretsLeaked(t, err, "hunter2")
}
