// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func validContextPolicyConfig() *Config {
	return &Config{
		ContextPolicyAdminEnabled: true, PluginAuthEnabled: true,
		ContextMaxAccounts: 10, ContextMaxEntries: 20, ContextMaxTextBytes: 256,
		ContextMaxIntegerDigits: 128, ContextMaxFractionDigits: "128", ContextMaxRules: 10,
		ContextMaxExpressionBytes: 5000, ContextCELCostLimit: "100000", ContextCELTotalCostLimit: "1000000", ContextPolicyMaxBodyBytes: 65536,
	}
}

func TestLoadContextPolicyConfig(t *testing.T) {
	t.Parallel()
	cfg, err := loadContextPolicyConfig(&Config{})
	require.NoError(t, err)
	require.Nil(t, cfg)
	service, err := initContextPolicyService(&Config{}, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Nil(t, service, "disabled administration must not produce a typed-nil service")
	for _, invalid := range []string{"auth", "accounts", "entries", "text", "integer", "fraction missing", "fraction negative", "rules", "expression", "cost", "total cost", "body"} {
		t.Run(invalid, func(t *testing.T) {
			input := validContextPolicyConfig()
			switch invalid {
			case "auth":
				input.PluginAuthEnabled = false
			case "accounts":
				input.ContextMaxAccounts = 0
			case "entries":
				input.ContextMaxEntries = 0
			case "text":
				input.ContextMaxTextBytes = 0
			case "integer":
				input.ContextMaxIntegerDigits = 0
			case "fraction missing":
				input.ContextMaxFractionDigits = ""
			case "fraction negative":
				input.ContextMaxFractionDigits = "-1"
			case "rules":
				input.ContextMaxRules = 0
			case "expression":
				input.ContextMaxExpressionBytes = 0
			case "cost":
				input.ContextCELCostLimit = "0"
			case "total cost":
				input.ContextCELTotalCostLimit = "no"
			case "body":
				input.ContextPolicyMaxBodyBytes = 0
			}
			_, err := loadContextPolicyConfig(input)
			require.Error(t, err)
		})
	}
	input := validContextPolicyConfig()
	input.ContextMaxFractionDigits = "0"
	_, err = loadContextPolicyConfig(input)
	require.Error(t, err)
}
