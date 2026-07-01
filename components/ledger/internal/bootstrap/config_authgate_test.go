// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBootAuthGates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		envName            string
		authEnabled        bool
		multiTenantEnabled bool
		wantErrContains    string
	}{
		{
			name:            "production single-tenant without auth fails boot",
			envName:         "production",
			authEnabled:     false,
			wantErrContains: "PLUGIN_AUTH_ENABLED",
		},
		{
			name:            "mixed-case padded production cannot slip the gate",
			envName:         " Production ",
			authEnabled:     false,
			wantErrContains: "PLUGIN_AUTH_ENABLED",
		},
		{
			name:        "production with auth enabled boots",
			envName:     "production",
			authEnabled: true,
		},
		{
			name:        "local without auth boots (developer onboarding path)",
			envName:     "local",
			authEnabled: false,
		},
		{
			name:        "development without auth boots",
			envName:     "development",
			authEnabled: false,
		},
		{
			name:        "unset env name without auth boots",
			envName:     "",
			authEnabled: false,
		},
		{
			name:               "multi-tenant without auth fails in any environment",
			envName:            "local",
			authEnabled:        false,
			multiTenantEnabled: true,
			wantErrContains:    "MULTI_TENANT_ENABLED",
		},
		{
			name:               "multi-tenant with auth boots outside production",
			envName:            "local",
			authEnabled:        true,
			multiTenantEnabled: true,
		},
		{
			name:               "production multi-tenant without auth fails boot",
			envName:            "production",
			authEnabled:        false,
			multiTenantEnabled: true,
			wantErrContains:    "PLUGIN_AUTH_ENABLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				EnvName:            tt.envName,
				AuthEnabled:        tt.authEnabled,
				MultiTenantEnabled: tt.multiTenantEnabled,
			}

			err := validateBootAuthGates(cfg)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)

				return
			}

			require.NoError(t, err)
		})
	}
}
