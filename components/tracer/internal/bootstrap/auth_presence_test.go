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

func TestValidateAuthPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		deploymentMode    string
		apiKeyEnabled     bool
		pluginAuthEnabled bool
		wantErr           bool
		wantWarn          bool
	}{
		{
			name:           "local mode with both auth mechanisms off warns and passes",
			deploymentMode: "local",
			wantErr:        false,
			wantWarn:       true,
		},
		{
			name:           "unset mode defaults to local: both off warns and passes",
			deploymentMode: "",
			wantErr:        false,
			wantWarn:       true,
		},
		{
			name:           "saas mode with both auth mechanisms off fails boot",
			deploymentMode: "saas",
			wantErr:        true,
		},
		{
			name:           "byoc mode with both auth mechanisms off fails boot",
			deploymentMode: "byoc",
			wantErr:        true,
		},
		{
			name:           "mixed-case padded mode cannot slip the gate",
			deploymentMode: " SaaS ",
			wantErr:        true,
		},
		{
			// "onprem" is NOT a documented mode (config.go lists only
			// saas/byoc/local); resolveDeploymentMode passes through any
			// non-empty string, so the gate must treat it as non-local.
			name:           "undocumented non-local mode is still gated",
			deploymentMode: "onprem",
			wantErr:        true,
		},
		{
			name:           "saas mode with API key auth only passes",
			deploymentMode: "saas",
			apiKeyEnabled:  true,
			wantErr:        false,
		},
		{
			name:              "saas mode with plugin auth only passes",
			deploymentMode:    "saas",
			pluginAuthEnabled: true,
			wantErr:           false,
		},
		{
			name:              "local mode with auth enabled passes without warning",
			deploymentMode:    "local",
			apiKeyEnabled:     true,
			pluginAuthEnabled: true,
			wantErr:           false,
			wantWarn:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.NewMockLogger()
			cfg := &Config{
				DeploymentMode:    tt.deploymentMode,
				APIKeyEnabled:     tt.apiKeyEnabled,
				PluginAuthEnabled: tt.pluginAuthEnabled,
			}

			err := ValidateAuthPresence(t.Context(), cfg, logger)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "API_KEY_ENABLED")
				assert.Contains(t, err.Error(), "PLUGIN_AUTH_ENABLED")

				return
			}

			require.NoError(t, err)

			if tt.wantWarn {
				require.Len(t, logger.Calls, 1, "expected exactly one warning when all auth is disabled in local mode")
				assert.Contains(t, logger.Calls[0].Message, "ALL authentication is DISABLED")
			} else {
				assert.Empty(t, logger.Calls, "expected no warnings when at least one auth mechanism is enabled")
			}
		})
	}
}
