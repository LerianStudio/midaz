// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildTracerReserver_AuthGuard pins the boot-time fail-fast guard: a
// multi-tenant deployment that enables the tracer reservation integration
// (TRACER_BASE_URL set) must refuse to boot, because no M2M auth provider is
// wired and reservation calls would ship unauthenticated and tenant-less. The
// guard must NOT fire in single-tenant mode (no auth needed today) nor when the
// integration is disabled (TRACER_BASE_URL empty).
func TestBuildTracerReserver_AuthGuard(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tracerBaseURL      string
		wantErrContains    string
		wantReserverNil    bool
	}{
		{
			name:               "multi-tenant with tracer base URL fails boot",
			multiTenantEnabled: true,
			tracerBaseURL:      "http://tracer:4020",
			wantErrContains:    "MULTI_TENANT_ENABLED",
		},
		{
			name:               "single-tenant with tracer base URL boots",
			multiTenantEnabled: false,
			tracerBaseURL:      "http://tracer:4020",
		},
		{
			name:               "multi-tenant with empty tracer base URL boots disabled",
			multiTenantEnabled: true,
			tracerBaseURL:      "",
			wantReserverNil:    true,
		},
		{
			name:               "single-tenant with empty tracer base URL boots disabled",
			multiTenantEnabled: false,
			tracerBaseURL:      "",
			wantReserverNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				MultiTenantEnabled: tt.multiTenantEnabled,
				TracerBaseURL:      tt.tracerBaseURL,
			}

			reserver, err := buildTracerReserver(cfg, logger)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, reserver)

				return
			}

			require.NoError(t, err)

			if tt.wantReserverNil {
				assert.Nil(t, reserver)
			} else {
				assert.NotNil(t, reserver)
			}
		})
	}
}
