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
