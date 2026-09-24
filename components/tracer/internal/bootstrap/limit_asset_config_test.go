// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func validLimitAssetConfig() *Config {
	cfg := validContextPolicyConfig()
	cfg.ContextPolicyAdminEnabled = false
	cfg.ContextLimitAdminEnabled = true
	cfg.TracerTLSMode = "mtls"
	cfg.ContextLimitMaxScopes = 100
	cfg.ContextLimitMaxScopeBytes = 32768
	cfg.ContextLimitMaxBodyBytes = 65536
	cfg.ContextProducerBindings = `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","assetNamespace":"ledger"}]`
	return cfg
}

func TestLimitAssetAdminConfig(t *testing.T) {
	disabled, err := loadLimitAssetConfig(&Config{})
	require.NoError(t, err)
	require.Nil(t, disabled)
	for _, scenario := range []string{"valid", "auth off", "mesh", "no identity", "unknown field", "duplicate identity", "invalid bounds", "no scopes", "no scope bytes", "no body bound"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := validLimitAssetConfig()
			switch scenario {
			case "auth off":
				cfg.PluginAuthEnabled = false
			case "mesh":
				cfg.TracerTLSMode = "mesh"
			case "no identity":
				cfg.ContextProducerBindings = "[]"
			case "unknown field":
				cfg.ContextProducerBindings = `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","assetNamespace":"ledger","extra":true}]`
			case "duplicate identity":
				cfg.ContextProducerBindings = `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","assetNamespace":"ledger"},{"uri":"spiffe://example.test/ledger","integrationId":"ledger","assetNamespace":"ledger"}]`
			case "invalid bounds":
				cfg.ContextMaxAccounts = 0
			case "no scopes":
				cfg.ContextLimitMaxScopes = 0
			case "no scope bytes":
				cfg.ContextLimitMaxScopeBytes = 0
			case "no body bound":
				cfg.ContextLimitMaxBodyBytes = 0
			}
			result, err := loadLimitAssetConfig(cfg)
			if scenario == "valid" {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}
