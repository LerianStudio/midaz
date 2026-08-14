// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolveDeploymentMode covers the three branches of resolveDeploymentMode:
//   - nil cfg → "local" fallback
//   - empty DeploymentMode → "local" fallback
//   - explicit value → returned as-is
//
// Keeps bootstrap composition deterministic regardless of how Config was
// initialized (env vars, struct literal, nil) — anti-pattern that would crash
// on a nil cfg is avoided.
func TestResolveDeploymentMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{name: "nil_cfg_returns_local", cfg: nil, want: "local"},
		{name: "empty_string_returns_local", cfg: &Config{DeploymentMode: ""}, want: "local"},
		{name: "explicit_saas_returns_saas", cfg: &Config{DeploymentMode: "saas"}, want: "saas"},
		{name: "explicit_onprem_returns_onprem", cfg: &Config{DeploymentMode: "onprem"}, want: "onprem"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, resolveDeploymentMode(tc.cfg))
		})
	}
}
