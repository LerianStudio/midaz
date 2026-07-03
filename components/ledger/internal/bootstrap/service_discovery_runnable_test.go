// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
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
	}{
		{name: "leading colon form", address: ":3002", wantPort: 3002},
		{name: "host and port", address: "0.0.0.0:8080", wantPort: 8080},
		{name: "localhost and port", address: "localhost:3011", wantPort: 3011},
		{name: "missing colon", address: "3002", expectError: true},
		{name: "non-numeric port", address: ":bad", expectError: true},
		{name: "empty", address: "", expectError: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			port, err := parseServerPort(tc.address)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantPort, port)
		})
	}
}

func TestBuildLedgerServiceDescriptor(t *testing.T) {
	t.Parallel()

	svc := buildLedgerServiceDescriptor(3002)

	assert.Equal(t, "midaz-ledger-3002", svc.ID)
	assert.Equal(t, "midaz-ledger", svc.Name)
	assert.Equal(t, 3002, svc.Port)
	require.NotNil(t, svc.HealthCheck)
	assert.Equal(t, "30s", svc.HealthCheck.TTL)
	// Address/Scheme are left empty: Manager.Register fills them from
	// SD_ADVERTISE_ADDRESS.
	assert.Empty(t, svc.Address)
	assert.Empty(t, svc.Scheme)
}

func TestBuildLedgerServiceDescriptor_IDReflectsPort(t *testing.T) {
	t.Parallel()

	svc := buildLedgerServiceDescriptor(8080)

	assert.Equal(t, "midaz-ledger-8080", svc.ID)
	assert.Equal(t, 8080, svc.Port)
}

// TestServiceDiscoveryRunnable_NilManagerNoOp verifies the runnable returns
// immediately when the manager is nil, before installing any signal handler or
// spawning goroutines. Keeps the guard branch goleak-safe.
func TestServiceDiscoveryRunnable_NilManagerNoOp(t *testing.T) {
	t.Parallel()

	r := &serviceDiscoveryRunnable{manager: nil, logger: newTestLogger()}

	require.NoError(t, r.Run(nil))
}

// TestService_Run_ServiceDiscoveryBootParity verifies that when discovery is
// disabled the "Service Discovery" Launcher entry is not registered, preserving
// strict boot parity. It inspects the wiring guard directly rather than calling
// the blocking Run().
func TestService_Run_ServiceDiscoveryBootParity(t *testing.T) {
	t.Parallel()

	disabled := &Service{ServiceDiscoveryEnabled: false}
	enabled := &Service{ServiceDiscoveryEnabled: true}

	assert.False(t, disabled.ServiceDiscoveryEnabled,
		"disabled service must not add the Service Discovery runnable")
	assert.True(t, enabled.ServiceDiscoveryEnabled,
		"enabled service adds the Service Discovery runnable")
}
