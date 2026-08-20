// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateSaaSTLS exercises the centralized SaaS TLS enforcement gate
// invoked from initCoreInfra BEFORE any external connection opens.
//
// The function is intentionally pure: same Config in ⇒ same (nil|error)
// out, no logging, no clocks. This keeps the gate callable at the very top
// of bootstrap, before logger/telemetry are wired.
//
// Anti-pattern guarded against (N6): inline TLS checks scattered across
// connection sites. Centralizing in this function with one call site means
// a single grep finds the entire enforcement surface.
//
// Scope: Tracer's /readyz cycle is single-tenant, so this gate enforces
// Postgres TLS at boot when DEPLOYMENT_MODE=saas.
func TestValidateSaaSTLS(t *testing.T) {
	// helper: a config representing a fully TLS-correct SaaS deployment.
	// Each subtest derives from this baseline and mutates only the field(s)
	// under test, so the asserted error stays attributable to that mutation.
	baseSaaSCfg := func() *Config {
		return &Config{
			DeploymentMode: "saas",
			DBHost:         "db.internal",
			DBUser:         "tracer",
			DBPassword:     "secret",
			DBName:         "tracer",
			DBPort:         "5432",
			DBSSLMode:      "require",
		}
	}

	tests := []struct {
		name         string
		mutate       func(c *Config) // mutation applied to the baseline; nil ⇒ baseline as-is
		cfgOverride  *Config         // when set, replaces the baseline entirely (used for nil/non-saas)
		wantErr      bool
		wantErrParts []string // every substring must appear in err.Error()
	}{
		{
			// SaaS gate is mode-scoped. local mode never enforces TLS, so a
			// non-TLS Postgres DSN must pass through.
			name: "local mode with non-TLS postgres returns nil",
			cfgOverride: &Config{
				DeploymentMode: "local",
				DBHost:         "localhost",
				DBSSLMode:      "disable",
			},
			wantErr: false,
		},
		{
			// byoc is the customer-hosted mode: TLS is recommended but not
			// hard-enforced. Same input as the local case but with a different
			// mode label — must still pass.
			name: "byoc mode with non-TLS postgres returns nil",
			cfgOverride: &Config{
				DeploymentMode: "byoc",
				DBHost:         "localhost",
				DBSSLMode:      "disable",
			},
			wantErr: false,
		},
		{
			// The happy path: postgres TLS is configured.
			name:    "saas mode with TLS postgres returns nil",
			mutate:  nil,
			wantErr: false,
		},
		{
			// Postgres failure: sslmode=disable explicitly opts out of TLS.
			// The error MUST name the failing dep so the operator knows
			// which env var to flip.
			name: "saas mode with postgres sslmode=disable returns error mentioning postgres",
			mutate: func(c *Config) {
				c.DBSSLMode = "disable"
			},
			wantErr:      true,
			wantErrParts: []string{"postgres"},
		},
		{
			// Postgres dep treated as "not configured" when DBHost is
			// empty. buildPostgresDSN still produces a non-empty string
			// (host= user= ... sslmode=disable), so the gate must check
			// DBHost explicitly to avoid false-positive enforcement on
			// an unconfigured dep.
			name: "saas mode with empty DBHost skips postgres check",
			cfgOverride: &Config{
				DeploymentMode: "saas",
				DBHost:         "",
				DBSSLMode:      "disable",
			},
			wantErr: false,
		},
		{
			// Defensive: nil cfg must not panic.
			name:         "nil cfg returns error",
			cfgOverride:  nil, // signals "use literal nil"
			wantErr:      true,
			wantErrParts: []string{"nil config"},
		},
	}

	// nilSentinel distinguishes "use baseline" (cfgOverride field absent in
	// struct literal ⇒ Go zero value, which is nil for pointers) from "use
	// literal nil cfg". For the nil-cfg test we need an explicit signal.
	const nilSentinelName = "nil cfg returns error"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg *Config

			switch {
			case tc.name == nilSentinelName:
				cfg = nil
			case tc.cfgOverride != nil:
				cfg = tc.cfgOverride
			default:
				cfg = baseSaaSCfg()
				if tc.mutate != nil {
					tc.mutate(cfg)
				}
			}

			err := ValidateSaaSTLS(cfg)

			if tc.wantErr {
				require.Error(t, err)

				for _, part := range tc.wantErrParts {
					require.Contains(t, err.Error(), part,
						"expected error to mention %q, got: %v", part, err)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestValidateSaaSStreamingTLS covers the streaming half of the SaaS TLS gate.
// STREAMING_TLS_ENABLED belongs to lib-streaming's env contract and never lands
// on tracer's Config (binding it there is what left the flag with no reader at
// all in the pre-v3 tracer), so the resolved flag is passed in. The rule and the
// error sentence match the Postgres sibling above, plus the knob name.
func TestValidateSaaSStreamingTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *Config
		tlsEnabled bool
		wantErr    bool
	}{
		{name: "saas plaintext broker refused", cfg: &Config{DeploymentMode: "saas"}, wantErr: true},
		{name: "saas tls broker allowed", cfg: &Config{DeploymentMode: "saas"}, tlsEnabled: true},
		{name: "saas padded and uppercase is still saas", cfg: &Config{DeploymentMode: " SaaS "}, wantErr: true},
		{name: "byoc keeps plaintext broker", cfg: &Config{DeploymentMode: "byoc"}},
		{name: "local keeps plaintext broker", cfg: &Config{DeploymentMode: "local"}},
		{name: "unset mode keeps plaintext broker", cfg: &Config{}},
		{name: "nil config is an error", cfg: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSaaSStreamingTLS(tt.cfg, tt.tlsEnabled)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestValidateSaaSStreamingTLS_ErrorNamesTheKnob keeps the operator-facing
// sentence locked: the failing dependency and the env var that fixes it.
func TestValidateSaaSStreamingTLS_ErrorNamesTheKnob(t *testing.T) {
	t.Parallel()

	err := ValidateSaaSStreamingTLS(&Config{DeploymentMode: "saas"}, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "DEPLOYMENT_MODE=saas")
	require.Contains(t, err.Error(), "streaming")
	require.Contains(t, err.Error(), "STREAMING_TLS_ENABLED")
}
