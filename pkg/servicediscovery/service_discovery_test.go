// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseServerPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		address     string
		wantPort    int
		expectError bool
		errContains string
	}{
		{name: "leading colon form", address: ":3002", wantPort: 3002},
		{name: "host and port", address: "0.0.0.0:8080", wantPort: 8080},
		{name: "localhost and port", address: "localhost:3011", wantPort: 3011},
		{name: "missing colon", address: "3002", expectError: true, errContains: "parsing server address"},
		{name: "empty", address: "", expectError: true, errContains: "parsing server address"},
		{name: "non-numeric port", address: ":bad", expectError: true, errContains: "invalid syntax"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			port, err := ParseServerPort(tc.address)

			if tc.expectError {
				require.ErrorContains(t, err, tc.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantPort, port)
		})
	}
}

// withHostnameFn overrides the package hostnameFn seam for the duration of the
// test and restores it on cleanup. It is NOT parallel-safe: tests that call it
// must run serially (no t.Parallel) so the shared package var is not raced.
func withHostnameFn(t *testing.T, fn func() (string, error)) {
	t.Helper()

	prev := hostnameFn
	hostnameFn = fn
	t.Cleanup(func() { hostnameFn = prev })
}

func TestBuildServiceDescriptor(t *testing.T) {
	// No t.Parallel: subtests override the shared hostnameFn package var and run
	// serially to avoid a data race with each other.
	tests := []struct {
		name        string
		svcName     string
		port        int
		hostname    string
		hostnameErr error
		wantID      string
	}{
		{name: "ledger", svcName: "midaz-ledger", port: 3002, hostname: "testhost", wantID: "midaz-ledger-testhost-3002"},
		{name: "tracer", svcName: "midaz-tracer", port: 4020, hostname: "testhost", wantID: "midaz-tracer-testhost-4020"},
		{name: "hostname error falls back to legacy scheme", svcName: "midaz-ledger", port: 3002, hostnameErr: errors.New("boom"), wantID: "midaz-ledger-3002"},
		{name: "empty hostname falls back to legacy scheme", svcName: "midaz-tracer", port: 4020, hostname: "", wantID: "midaz-tracer-4020"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withHostnameFn(t, func() (string, error) { return tc.hostname, tc.hostnameErr })

			svc := BuildServiceDescriptor(tc.svcName, tc.port)

			assert.Equal(t, tc.wantID, svc.ID)
			assert.Equal(t, tc.svcName, svc.Name)
			assert.Equal(t, tc.port, svc.Port)
			assert.Equal(t, "30s", svc.HealthCheckTTL)
		})
	}
}

func TestBuildServiceDescriptor_IDIncludesHostAndPort(t *testing.T) {
	withHostnameFn(t, func() (string, error) { return "pod-7", nil })

	svc := BuildServiceDescriptor("midaz-ledger", 8080)

	assert.Equal(t, "midaz-ledger-pod-7-8080", svc.ID)
	assert.Equal(t, "midaz-ledger", svc.Name)
	assert.Equal(t, 8080, svc.Port)
}
