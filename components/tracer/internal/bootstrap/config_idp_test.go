// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIDPConfig_Defaults verifies the safe defaults for the optional RI
// (permission-declaration) IDP fields when the IDP_* env vars are absent.
// RI is opt-in and fail-open: with nothing configured, declaration stays off
// and the identity-provider coordinates stay empty, so boot must not require them.
func TestIDPConfig_Defaults(t *testing.T) {
	// Note: no t.Parallel() because t.Setenv is incompatible with parallel tests.
	// Force the IDP_* vars to empty so a polluted environment cannot mask the
	// safe defaults under test (empty string is indistinguishable from unset here).
	t.Setenv("IDP_DECLARATION_ENABLED", "")
	t.Setenv("IDP_HOST", "")
	t.Setenv("IDP_M2M_CLIENT_ID", "")
	t.Setenv("IDP_M2M_CLIENT_SECRET", "")

	cfg := &Config{}
	err := libCommons.SetConfigFromEnvVars(cfg)
	require.NoError(t, err, "SetConfigFromEnvVars should not fail with IDP_* absent")

	assert.False(t, cfg.DeclarationEnabled, "DeclarationEnabled should default to false (RI is opt-in, fail-open)")
	assert.Empty(t, cfg.IDPHost, "IDPHost should default to empty")
	assert.Empty(t, cfg.IDPM2MClientID, "IDPM2MClientID should default to empty")
	assert.Empty(t, cfg.IDPM2MClientSecret, "IDPM2MClientSecret should default to empty")
}

// TestIDPConfig_DeclarationEnabled_FromEnv characterizes the safe-by-default
// parsing of IDP_DECLARATION_ENABLED into the DeclarationEnabled flag. RI is
// opt-in and fail-open, so the bool must decode conservatively: an absent value
// and a non-parseable value both fall back to false without erroring or
// panicking, and only an explicit "true" turns the feature on. Asserts only the
// flag — the M2M secret is never read back here.
func TestIDPConfig_DeclarationEnabled_FromEnv(t *testing.T) {
	// Note: no t.Parallel() because sub-tests use t.Setenv which is
	// incompatible with parallel ancestors (Go testing restriction).

	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			// Empty string is indistinguishable from unset for this parser, so it
			// stands in for the "absent" case.
			name:     "absent_defaults_to_false",
			envValue: "",
			want:     false,
		},
		{
			name:     "invalid_value_defaults_to_false",
			envValue: "notabool",
			want:     false,
		},
		{
			name:     "true_string_enables_declaration",
			envValue: "true",
			want:     true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			// Note: no t.Parallel() because t.Setenv is incompatible with parallel sub-tests.
			t.Setenv("IDP_DECLARATION_ENABLED", tt.envValue)

			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err, "SetConfigFromEnvVars should not fail (safe by default, no panic)")

			assert.Equal(t, tt.want, cfg.DeclarationEnabled,
				"DeclarationEnabled should decode to the safe expected value")
		})
	}
}
