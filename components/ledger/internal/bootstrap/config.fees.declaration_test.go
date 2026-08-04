// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	authdecl "github.com/LerianStudio/lib-auth/v3/auth/declaration"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	feesdecl "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/declaration"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// stubTokenMinter is a hermetic authdecl.TokenMinter: it satisfies the interface
// so authdecl.New can be constructed without a real *middleware.AuthClient, and
// it never dials (New parses+validates+hashes eagerly; only Start would mint a
// token + PUT). Returning a static token keeps the manifest-validity guard test
// fully offline.
type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "tok", nil
}

// TestInitFeesDeclaration_DisabledReturnsSafeNoopStop proves the default-OFF,
// fail-open contract: with DECLARATION_ENABLED unset or not exactly "true",
// initFeesDeclaration must return a NON-NIL no-op stop and a nil error WITHOUT
// touching identity/M2M config, and the returned stop must be safe to call.
// This is the boot-parity guarantee — an un-configured ledger boots unchanged.
//
// Not t.Parallel(): the subtests mutate process env via t.Setenv.
func TestInitFeesDeclaration_DisabledReturnsSafeNoopStop(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "unset_or_empty", value: ""},
		{name: "explicit_false", value: "false"},
		{name: "not_exactly_true", value: "TRUE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DECLARATION_ENABLED", tc.value)

			stop, err := initFeesDeclaration(context.Background(), &libLog.GoLogger{})

			require.NoError(t, err, "disabled declaration must not error")
			require.NotNil(t, stop, "stop must be non-nil on the disabled path")
			assert.NotPanics(t, stop, "calling the no-op stop must be safe")
		})
	}
}

// TestInitFeesDeclaration_EnabledMissingConfigErrors proves that with
// DECLARATION_ENABLED=true the fixed un-prefixed env contract is validated
// internally by WireFromEnv: each missing required var yields an error that
// NAMES the var, and even on the error path the returned stop is non-nil and
// safe to call (so a caller's deferred stop never nil-panics). No network is
// dialed — validation fails before any identity PUT.
//
// Not t.Parallel(): the subtests mutate process env via t.Setenv.
func TestInitFeesDeclaration_EnabledMissingConfigErrors(t *testing.T) {
	cases := []struct {
		name         string
		identityHost string
		clientID     string
		clientSecret string
		wantErrVar   string
	}{
		{
			name:       "missing_identity_host",
			wantErrVar: "PLUGIN_IDENTITY_HOST",
		},
		{
			name:         "missing_client_id",
			identityHost: "http://identity.local",
			wantErrVar:   "M2M_CLIENT_ID",
		},
		{
			name:         "missing_client_secret",
			identityHost: "http://identity.local",
			clientID:     "fees-m2m",
			wantErrVar:   "M2M_CLIENT_SECRET",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DECLARATION_ENABLED", "true")
			t.Setenv("PLUGIN_IDENTITY_HOST", tc.identityHost)
			t.Setenv("M2M_CLIENT_ID", tc.clientID)
			t.Setenv("M2M_CLIENT_SECRET", tc.clientSecret)

			stop, err := initFeesDeclaration(context.Background(), &libLog.GoLogger{})

			require.Error(t, err, "missing required config must error when enabled")
			assert.Contains(t, err.Error(), tc.wantErrVar,
				"error must name the missing env var %q", tc.wantErrVar)
			require.NotNil(t, stop, "stop must be non-nil even on the error path")
			assert.NotPanics(t, stop, "calling the no-op stop must be safe on the error path")
		})
	}
}

// TestFeesManifest_PassesLibAuthValidation runs the embedded fees manifest
// through lib-auth's REAL validator via authdecl.New, which parses + validates
// (undeclared role refs, bad effects, dup resource:action, slug==service BOLA)
// and precomputes the wire hash — all with ZERO network (only Start dials). It
// is the fail-closed guard for the manifest payload: a future bad edit to
// permissions.yaml that would HARD-FAIL ledger boot under DECLARATION_ENABLED=true
// is caught here in CI instead. It subsumes the weaker slug-only assertion (New
// enforces slug==service), but the explicit slug/manifest checks below document
// the invariant the wiring depends on.
func TestFeesManifest_PassesLibAuthValidation(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "plugin-fees", constant.ModuleFees,
		"ModuleFees is the fees declaration slug and must stay 'plugin-fees'")

	var manifest struct {
		Service string `yaml:"service"`
	}

	require.NoError(t, yaml.Unmarshal(feesdecl.Manifest, &manifest),
		"embedded fees manifest must be valid YAML")
	assert.Equal(t, constant.ModuleFees, manifest.Service,
		"declaration slug (constant.ModuleFees) must equal the manifest service:")

	// New is the real lib-auth constructor: parse + validate + hash, no I/O.
	_, err := authdecl.New(authdecl.Config{
		Slug:         constant.ModuleFees,
		Manifest:     feesdecl.Manifest,
		IdentityAddr: "http://identity.local",
		Auth:         stubTokenMinter{},
		ClientID:     "fees-m2m",
		ClientSecret: "secret",
	})
	require.NoError(t, err,
		"embedded fees manifest must pass lib-auth's parse+validate+BOLA checks")
}
