// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// TestGetBaseURL covers the resolution order of the integration-test base URL:
// (1) SERVER_ADDRESS as a full URL is returned verbatim; (2) SERVER_ADDRESS as
// ":port" is normalized to "http://localhost:port" (the lib-commons convention
// where a missing host implies localhost); (3) SERVER_ADDRESS as "host:port" is
// prefixed with "http://"; (4) empty SERVER_ADDRESS falls back to SERVER_PORT;
// (5) both empty fall back to the baked-in default.
func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name          string
		serverAddress string
		serverPort    string
		expected      string
	}{
		{
			name:          "full http URL returned verbatim",
			serverAddress: "http://api.example.com:8080",
			expected:      "http://api.example.com:8080",
		},
		{
			name:          "full https URL returned verbatim",
			serverAddress: "https://tracer.prod.internal",
			expected:      "https://tracer.prod.internal",
		},
		{
			name:          "bare port with leading colon gets localhost",
			serverAddress: ":4020",
			expected:      "http://localhost:4020",
		},
		{
			name:          "host:port gets http:// prefix",
			serverAddress: "tracer:4020",
			expected:      "http://tracer:4020",
		},
		{
			name:          "host without port gets http:// prefix",
			serverAddress: "tracer.svc",
			expected:      "http://tracer.svc",
		},
		{
			name:       "empty SERVER_ADDRESS falls back to SERVER_PORT bare number",
			serverPort: "4020",
			expected:   "http://localhost:4020",
		},
		{
			name:       "empty SERVER_ADDRESS falls back to SERVER_PORT with colon",
			serverPort: ":4020",
			expected:   "http://localhost:4020",
		},
		{
			name:     "both empty fall back to baked default",
			expected: constant.DefaultTestServerURL,
		},
	}

	// Intentionally NOT calling t.Parallel() on the subtests below: each case
	// uses t.Setenv to mutate the SERVER_ADDRESS / SERVER_PORT process-wide
	// environment. Parallel subtests would race on those variables and make
	// GetBaseURL's resolution non-deterministic.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SERVER_ADDRESS", tc.serverAddress)
			t.Setenv("SERVER_PORT", tc.serverPort)

			require.Equal(t, tc.expected, GetBaseURL())
		})
	}
}

// TestGetAPIKey verifies the same resolution pattern for the API key helper:
// environment override wins, otherwise the dev default is returned.
func TestGetAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "env override",
			apiKey:   "custom_key_12345",
			expected: "custom_key_12345",
		},
		{
			name:     "empty env returns dev default",
			apiKey:   "",
			expected: "dev_api_key_change_in_production",
		},
	}

	// Not using t.Parallel() — same reason as TestGetBaseURL: t.Setenv below
	// mutates process-wide env, incompatible with parallel subtests.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("API_KEY", tc.apiKey)

			require.Equal(t, tc.expected, GetAPIKey())
		})
	}
}

// TestRandomSuffix verifies the suffix contract deterministically: every call
// must return exactly 8 lowercase hex characters (the first slice of a UUID v4
// string). We validate the FORMAT rather than comparing two outputs against
// each other — a non-deterministic equality check would flake on the ~1-in-2^32
// chance of a UUID collision in the same process, which repo guidelines
// explicitly disallow.
func TestRandomSuffix(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}$`)

	for i := 0; i < 5; i++ {
		suffix := RandomSuffix()

		require.Len(t, suffix, 8)
		require.Regexp(t, pattern, suffix,
			"RandomSuffix must return 8 lowercase hex chars, got %q", suffix)
	}
}
