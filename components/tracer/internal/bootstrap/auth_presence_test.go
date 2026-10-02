// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestValidateAuthPresence locks the HTTP auth gate: only saas and
// multi-tenant refuse a tracer with neither auth mechanism; every other
// deployment boots with exactly one Warn, and any mechanism silences it.
func TestValidateAuthPresence(t *testing.T) {
	t.Parallel()

	const (
		localWarn    = "ALL authentication is DISABLED — every /v1 route is open (acceptable for local development only)"
		nonLocalWarn = authDisabledNonLocalMsg
	)

	tests := []struct {
		name              string
		deploymentMode    string
		multiTenant       bool
		apiKeyEnabled     bool
		pluginAuthEnabled bool
		wantErr           bool
		wantWarn          string
	}{
		{name: "local without auth warns", deploymentMode: "local", wantWarn: localWarn},
		{name: "unset mode without auth warns as local", deploymentMode: "", wantWarn: localWarn},
		{name: "saas without auth is refused", deploymentMode: "saas", wantErr: true},
		{name: "padded mixed-case saas without auth is refused", deploymentMode: " SaaS ", wantErr: true},
		{name: "multi-tenant without auth is refused", deploymentMode: "byoc", multiTenant: true, wantErr: true},
		{name: "multi-tenant without auth is refused even locally", deploymentMode: "local", multiTenant: true, wantErr: true},
		{name: "byoc without auth boots with one warn", deploymentMode: "byoc", wantWarn: nonLocalWarn},
		{name: "undocumented mode without auth boots with one warn", deploymentMode: "onprem", wantWarn: nonLocalWarn},
		{name: "saas with API key boots silently", deploymentMode: "saas", apiKeyEnabled: true},
		{name: "saas with plugin auth boots silently", deploymentMode: "saas", pluginAuthEnabled: true},
		{name: "multi-tenant with plugin auth boots silently", deploymentMode: "saas", multiTenant: true, pluginAuthEnabled: true},
		{name: "byoc with API key boots silently", deploymentMode: "byoc", apiKeyEnabled: true},
		{name: "local with both mechanisms boots silently", deploymentMode: "local", apiKeyEnabled: true, pluginAuthEnabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.NewMockLogger()
			cfg := &Config{
				DeploymentMode:     tt.deploymentMode,
				MultiTenantEnabled: tt.multiTenant,
				APIKeyEnabled:      tt.apiKeyEnabled,
				PluginAuthEnabled:  tt.pluginAuthEnabled,
			}

			err := ValidateAuthPresence(t.Context(), cfg, logger)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "API_KEY_ENABLED")
				assert.Contains(t, err.Error(), "PLUGIN_AUTH_ENABLED")
				assert.Empty(t, logger.Calls, "a refused boot logs nothing")

				return
			}

			require.NoError(t, err)

			if tt.wantWarn == "" {
				assert.Empty(t, logger.Calls)

				return
			}

			require.Len(t, logger.Calls, 1, "exactly one warning when all auth is disabled")
			assert.Equal(t, "warn", logger.Calls[0].Level)
			assert.Equal(t, tt.wantWarn, logger.Calls[0].Message)

			fields := map[string]any{}
			for _, f := range logger.Calls[0].Fields {
				fields[f.Key] = f.Value
			}

			assert.Equal(t, "API_KEY_ENABLED", fields["config"])
			assert.Equal(t, "PLUGIN_AUTH_ENABLED", fields["config_alt"])
		})
	}
}
