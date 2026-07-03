// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildManager_DisabledReturnsNoopManager(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.NoError(t, err)
	require.NotNil(t, manager)
	require.False(t, enabled)
}

func TestBuildManager_EnabledWithoutAdvertiseAddrFailsFast(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.Error(t, err)
	require.Nil(t, manager)
	require.True(t, enabled)
	require.True(t, errors.Is(err, libsd.ErrEmptyAdvertiseAddr))
}

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

func TestBuildServiceDescriptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		svcName string
		port    int
		wantID  string
	}{
		{name: "ledger", svcName: "midaz-ledger", port: 3002, wantID: "midaz-ledger-3002"},
		{name: "crm", svcName: "midaz-crm", port: 4003, wantID: "midaz-crm-4003"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := BuildServiceDescriptor(tc.svcName, tc.port)

			assert.Equal(t, tc.wantID, svc.ID)
			assert.Equal(t, tc.svcName, svc.Name)
			assert.Equal(t, tc.port, svc.Port)
			require.NotNil(t, svc.HealthCheck)
			assert.Equal(t, "30s", svc.HealthCheck.TTL)
			// Address/Scheme are left empty: Manager.Register fills them from
			// SD_ADVERTISE_ADDRESS.
			assert.Empty(t, svc.Address)
			assert.Empty(t, svc.Scheme)
		})
	}
}

func TestBuildServiceDescriptor_IDReflectsPort(t *testing.T) {
	t.Parallel()

	svc := BuildServiceDescriptor("midaz-ledger", 8080)

	assert.Equal(t, "midaz-ledger-8080", svc.ID)
	assert.Equal(t, 8080, svc.Port)
}
