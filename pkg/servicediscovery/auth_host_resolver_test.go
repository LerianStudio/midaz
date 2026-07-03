// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubAuthHostResolver records how many times Resolve was called and returns a
// fixed result, so tests can assert both the returned host and that resolution
// is skipped when auth is disabled.
type stubAuthHostResolver struct {
	calls    int
	resolved string
	err      error
}

func (s *stubAuthHostResolver) Resolve(_ context.Context, _, _ string) (string, error) {
	s.calls++

	return s.resolved, s.err
}

func TestResolveAuthHost(t *testing.T) {
	t.Parallel()

	const staticHost = "plugin-auth:4000"

	resolveErr := errors.New("consul unavailable")

	tests := []struct {
		name          string
		authEnabled   bool
		staticHost    string
		stubResolved  string
		stubErr       error
		expectedHost  string
		expectedCalls int
	}{
		{
			name:          "auth disabled returns static host without resolving",
			authEnabled:   false,
			stubResolved:  "should-not-be-used:9999",
			stubErr:       nil,
			expectedHost:  staticHost,
			expectedCalls: 0,
		},
		{
			name:          "auth enabled uses resolved host on success",
			authEnabled:   true,
			stubResolved:  "consul-host:4000",
			stubErr:       nil,
			expectedHost:  "consul-host:4000",
			expectedCalls: 1,
		},
		{
			name:          "auth enabled falls back to static host on resolve error",
			authEnabled:   true,
			stubResolved:  "",
			stubErr:       resolveErr,
			expectedHost:  staticHost,
			expectedCalls: 1,
		},
		{
			name:          "auth enabled degrades to static host on resolve deadline exceeded",
			authEnabled:   true,
			stubResolved:  "",
			stubErr:       context.DeadlineExceeded,
			expectedHost:  staticHost,
			expectedCalls: 1,
		},
		{
			name:          "auth enabled borrows fallback scheme when resolved host lacks one",
			authEnabled:   true,
			staticHost:    "http://plugin-auth:4000",
			stubResolved:  "plugin-auth:4000",
			stubErr:       nil,
			expectedHost:  "http://plugin-auth:4000",
			expectedCalls: 1,
		},
		{
			name:          "auth enabled keeps resolved scheme without double-prefixing",
			authEnabled:   true,
			staticHost:    "http://x:4000",
			stubResolved:  "https://x:4000",
			stubErr:       nil,
			expectedHost:  "https://x:4000",
			expectedCalls: 1,
		},
		{
			name:          "auth enabled leaves scheme-less resolved host untouched when fallback has none",
			authEnabled:   true,
			staticHost:    "x:4000",
			stubResolved:  "x:4000",
			stubErr:       nil,
			expectedHost:  "x:4000",
			expectedCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubAuthHostResolver{resolved: tc.stubResolved, err: tc.stubErr}

			static := staticHost
			if tc.staticHost != "" {
				static = tc.staticHost
			}

			host := ResolveAuthHost(context.Background(), stub, tc.authEnabled, static)

			require.Equal(t, tc.expectedHost, host)
			require.Equal(t, tc.expectedCalls, stub.calls)
		})
	}
}

func TestWithFallbackScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resolved   string
		staticHost string
		expected   string
	}{
		{
			name:       "resolved already has scheme wins over fallback",
			resolved:   "https://x:4000",
			staticHost: "http://x:4000",
			expected:   "https://x:4000",
		},
		{
			name:       "resolved lacks scheme borrows fallback scheme",
			resolved:   "plugin-auth:4000",
			staticHost: "http://plugin-auth:4000",
			expected:   "http://plugin-auth:4000",
		},
		{
			name:       "neither has scheme returns resolved unchanged",
			resolved:   "x:4000",
			staticHost: "x:4000",
			expected:   "x:4000",
		},
		{
			name:       "fallback with scheme applied to scheme-less resolved host",
			resolved:   "consul-host:4000",
			staticHost: "https://plugin-auth:4000",
			expected:   "https://consul-host:4000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, withFallbackScheme(tc.resolved, tc.staticHost))
		})
	}
}
