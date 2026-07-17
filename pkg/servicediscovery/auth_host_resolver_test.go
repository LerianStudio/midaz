// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubAuthHostResolver mimics the libsd Manager.ResolvePreferredURL fallback
// contract so tests exercise the same source-classification the production
// resolver relies on: on a consul error it returns the caller-supplied fallback
// verbatim (nil err) when that fallback is non-empty, and the consul error
// otherwise. This is what lets ResolveAuthHost distinguish resolved-from-consul
// from fell-back given it passes an EMPTY fallback.
type stubAuthHostResolver struct {
	calls     int
	resolved  string
	consulErr error
}

func (s *stubAuthHostResolver) ResolvePreferredURL(_ context.Context, _, fallback string) (string, error) {
	s.calls++

	if s.consulErr == nil {
		return s.resolved, nil
	}

	if fallback != "" {
		return fallback, nil
	}

	return "", s.consulErr
}

// stubElapsed freezes the resolve-duration seam so tests observe a fixed elapsed
// time regardless of wall clock. It returns a restore func to reset the seam.
func stubElapsed(d time.Duration) func() {
	orig := sinceFn
	sinceFn = func(time.Time) time.Duration { return d }

	return func() { sinceFn = orig }
}

func TestResolveAuthHost(t *testing.T) {
	t.Parallel()

	const staticHost = "plugin-auth:4000"

	resolveErr := errors.New("consul unavailable")

	tests := []struct {
		name           string
		authEnabled    bool
		staticHost     string
		stubResolved   string
		stubConsulErr  error
		expectedHost   string
		expectedCalls  int
		expectRecorded bool
		expectedResult string
	}{
		{
			name:           "auth disabled returns static host without resolving or recording",
			authEnabled:    false,
			stubResolved:   "should-not-be-used:9999",
			stubConsulErr:  nil,
			expectedHost:   staticHost,
			expectedCalls:  0,
			expectRecorded: false,
		},
		{
			name:           "auth enabled uses resolved url on success and records resolved",
			authEnabled:    true,
			stubResolved:   "http://consul-host:4000",
			stubConsulErr:  nil,
			expectedHost:   "http://consul-host:4000",
			expectedCalls:  1,
			expectRecorded: true,
			expectedResult: ResultResolved,
		},
		{
			name:           "auth enabled falls back to static host on resolve error and records fallback",
			authEnabled:    true,
			stubResolved:   "",
			stubConsulErr:  resolveErr,
			expectedHost:   staticHost,
			expectedCalls:  1,
			expectRecorded: true,
			expectedResult: ResultFallback,
		},
		{
			name:           "auth enabled degrades to static host on resolve deadline exceeded and records fallback",
			authEnabled:    true,
			stubResolved:   "",
			stubConsulErr:  context.DeadlineExceeded,
			expectedHost:   staticHost,
			expectedCalls:  1,
			expectRecorded: true,
			expectedResult: ResultFallback,
		},
		{
			name:           "auth enabled with empty static host on error returns empty and records error",
			authEnabled:    true,
			staticHost:     " ", // sentinel meaning "force empty static host" (see below)
			stubResolved:   "",
			stubConsulErr:  resolveErr,
			expectedHost:   "",
			expectedCalls:  1,
			expectRecorded: true,
			expectedResult: ResultError,
		},
		{
			name:           "auth enabled returns scheme-complete resolved url verbatim without mutation",
			authEnabled:    true,
			staticHost:     "http://plugin-auth:4000",
			stubResolved:   "https://consul-host:5000",
			stubConsulErr:  nil,
			expectedHost:   "https://consul-host:5000",
			expectedCalls:  1,
			expectRecorded: true,
			expectedResult: ResultResolved,
		},
	}

	// Freeze the duration seam once for the whole table so elapsed is exactly 5ms.
	// Subtests are not run in parallel here because they share this seam.
	restore := stubElapsed(5 * time.Millisecond)
	defer restore()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubAuthHostResolver{resolved: tc.stubResolved, consulErr: tc.stubConsulErr}
			recorder := &stubRecorder{}

			static := staticHost
			if tc.staticHost == " " {
				static = ""
			} else if tc.staticHost != "" {
				static = tc.staticHost
			}

			host := ResolveAuthHost(context.Background(), stub, tc.authEnabled, static, recorder)

			require.Equal(t, tc.expectedHost, host)
			require.Equal(t, tc.expectedCalls, stub.calls)

			if !tc.expectRecorded {
				require.Empty(t, recorder.resolveResults, "no resolve metric expected when resolution not attempted")
				return
			}

			require.Len(t, recorder.resolveResults, 1)
			require.Equal(t, "plugin-auth", recorder.resolveResults[0].service)
			require.Equal(t, tc.expectedResult, recorder.resolveResults[0].result)
			require.Equal(t, int64(5), recorder.resolveResults[0].durationMs)
		})
	}
}

func TestResolveAuthHostNilRecorderDoesNotPanic(t *testing.T) {
	restore := stubElapsed(5 * time.Millisecond)
	defer restore()

	stub := &stubAuthHostResolver{resolved: "http://consul-host:4000"}

	require.NotPanics(t, func() {
		host := ResolveAuthHost(context.Background(), stub, true, "plugin-auth:4000", nil)
		require.Equal(t, "http://consul-host:4000", host)
	})
}
