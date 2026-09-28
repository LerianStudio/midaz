// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitContextLimitDefinitionPolicy(t *testing.T) {
	t.Parallel()

	disabled, err := initContextLimitDefinitionPolicy(&Config{})
	require.NoError(t, err)
	require.Nil(t, disabled, "the legacy profile installs no definition policy")

	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{name: "valid", mutate: func(*Config) {}, valid: true},
		{name: "invalid bounds", mutate: func(c *Config) { c.ContextMaxAccounts = 0 }},
		{name: "invalid fraction digits", mutate: func(c *Config) { c.ContextMaxFractionDigits = "1" }},
		{name: "no scopes", mutate: func(c *Config) { c.ContextLimitMaxScopes = 0 }},
		{name: "no scope bytes", mutate: func(c *Config) { c.ContextLimitMaxScopeBytes = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.ContextReserveEnabled = true
			cfg.ContextLimitMaxScopes = 100
			cfg.ContextLimitMaxScopeBytes = 32768
			tc.mutate(cfg)

			policy, err := initContextLimitDefinitionPolicy(cfg)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, policy)

				return
			}

			require.Error(t, err)
			require.Nil(t, policy)
		})
	}
}
