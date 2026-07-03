// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubAuthHostResolver{resolved: tc.stubResolved, err: tc.stubErr}

			host := resolveAuthHost(context.Background(), stub, tc.authEnabled, staticHost)

			require.Equal(t, tc.expectedHost, host)
			require.Equal(t, tc.expectedCalls, stub.calls)
		})
	}
}
