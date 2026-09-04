// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v4/auth/declaration"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
)

// None of the tests in this file call t.Parallel(): the package runs
// goleak.VerifyTestMain (see goleak_test.go / goleak_test helpers), and a leak
// check observes the process-global goroutine set, so every test in the package
// must stay sequential — a still-draining sibling is indistinguishable from a
// leak. This is exception #5 of the t.Parallel() hard gate (testing-unit.md).

// stubTokenMinter is a no-op declaration.TokenMinter used where a non-nil minter
// is required but the token is never actually minted: the disabled-flag path
// returns before any publisher is constructed, and declaration.New performs no
// I/O (it never mints), so the empty token is never observed.
type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// fixedTokenMinter mints a non-empty dummy token so the publisher's background
// Publish proceeds PAST the mint step to the PUT against the identity stub. An
// empty token would be classified deterministic (auth disabled/misconfigured)
// and short-circuit before the PUT, so the 5xx fail-open path would never run.
// The value is not a real credential.
type fixedTokenMinter struct{}

func (fixedTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "dummy-m2m-token", nil
}

// TestBuildDeclarationPublishers_DisabledReturnsNoStops asserts the helper is a
// no-op when RI declaration is disabled: it constructs no publisher and returns
// zero stop funcs, so launcherApps registers no runnable.
func TestBuildDeclarationPublishers_DisabledReturnsNoStops(t *testing.T) {
	cfg := &Config{DeclarationEnabled: false}

	stops := buildDeclarationPublishers(cfg, nil, libLog.NewNop())

	assert.Empty(t, stops, "disabled declaration must yield no stop funcs")
}

// TestBuildDeclarationPublishers_IdentityAlways5xx_FailsOpenAndStopsDrain
// characterizes the fail-open posture: with RI enabled and an identity that
// ALWAYS answers 5xx, the helper still returns successfully (boot is NOT
// blocked), builds the single midaz publisher, drives its background PUT against
// the failing identity, and every returned stop() drains its goroutine without
// panic or deadlock. goleak.VerifyTestMain then proves no publisher goroutine
// survives the package run.
func TestBuildDeclarationPublishers_IdentityAlways5xx_FailsOpenAndStopsDrain(t *testing.T) {
	const stopDrainTimeout = 10 * time.Second

	// reqCh signals each PUT the identity stub receives, so the test can prove
	// the publisher actually exercised the failing-identity PUT path before
	// draining it. Buffered + non-blocking send so the handler never blocks on
	// retry traffic the test has stopped reading.
	reqCh := make(chan struct{}, 64)

	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Force per-request connection close so no idle client keep-alive
		// goroutine lingers into the package-level goleak check; this is about
		// the httptest client transport, not the code under test.
		w.Header().Set("Connection", "close")

		select {
		case reqCh <- struct{}{}:
		default:
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer identity.Close()

	cfg := &Config{
		DeclarationEnabled: true,
		IDPHost:            identity.URL,
		IDPM2MClientID:     "dummy-client-id",
		IDPM2MClientSecret: "dummy-client-secret",
	}

	// The helper returning at all is the primary fail-open evidence: an identity
	// that only 5xxs did not block or crash boot.
	stops := buildDeclarationPublishers(cfg, fixedTokenMinter{}, libLog.NewNop())

	require.Len(t, stops, 1, "the single midaz publisher must be constructed and started")

	for _, s := range stops {
		require.NotNil(t, s, "each started publisher must yield a non-nil stop func")
	}

	// Prove the publisher reached the failing-identity PUT (fail-open ran the
	// real publish path, not a short-circuit).
	for i := 0; i < len(stops); i++ {
		select {
		case <-reqCh:
		case <-time.After(stopDrainTimeout):
			t.Fatalf("expected %d PUTs to reach the identity stub; the fail-open publish path was not exercised", len(stops))
		}
	}

	// Every stop() must cancel its publisher's context and drain its goroutine
	// without deadlocking. Run them off-thread and bound the wait so a hang is a
	// reported failure, not an indefinitely stuck test.
	drained := make(chan struct{})

	go func() {
		for _, s := range stops {
			s()
		}

		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(stopDrainTimeout):
		t.Fatal("declaration publisher stop() did not return: possible deadlock draining the background publish goroutine")
	}
}

// TestBuildDeclarationPublishers_RealManifestsMatchWiredSlugs proves the exact
// (slug, manifest) pair the helper wires satisfies declaration.New's client-side
// BOLA guard (slug == manifest.service): the real embedded manifest declares
// "midaz", and New rejects any other slug against it. This is the deterministic,
// I/O-free proof that the single wired slug is correct; New starts no goroutine
// here.
func TestBuildDeclarationPublishers_RealManifestsMatchWiredSlugs(t *testing.T) {
	tests := []struct {
		name            string
		slug            string
		manifest        []byte
		wantErrContains string
	}{
		{
			name:     "midaz slug matches embedded midaz manifest",
			slug:     "midaz",
			manifest: ledgerembed.MidazManifest,
		},
		{
			name:            "mismatched slug rejected against midaz manifest",
			slug:            "not-a-match",
			manifest:        ledgerembed.MidazManifest,
			wantErrContains: `manifest.service "midaz"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			pub, err := declaration.New(declaration.Config{
				Slug:         tt.slug,
				Manifest:     tt.manifest,
				IdentityAddr: "http://identity.invalid",
				Auth:         stubTokenMinter{},
				ClientID:     "dummy-client-id",
				ClientSecret: "dummy-client-secret",
				Cache:        nil,
				Interval:     0,
				FailFast:     false,
				Logger:       libLog.NewNop(),
			})

			if tt.wantErrContains != "" {
				require.Error(t, err, "New must reject a slug that does not equal manifest.service")
				assert.ErrorContains(t, err, tt.wantErrContains)
				assert.Nil(t, pub, "no publisher on a rejected slug/manifest pair")

				return
			}

			require.NoError(t, err, "wired slug %q must equal its manifest.service", tt.slug)
			require.NotNil(t, pub, "a valid slug/manifest pair must yield a publisher")
		})
	}
}

// TestValidateSaaSDeclarationTLS covers the SaaS TLS gate for the RI declaration
// publisher's IdP dial. The publisher ships the M2M client_credentials grant and
// the resulting bearer token to IDP_HOST, so a Lerian-hosted deployment must not
// reach the IdP in cleartext — the same rule Postgres, Mongo, Redis and RabbitMQ
// answer to. The gate is a no-op unless RI is enabled AND IDP_HOST carries an
// explicit http:// scheme (the one case where the publisher actually dials in
// cleartext); an https:// host is secure, and a scheme-less/malformed host is
// rejected fail-open by the publisher's own config validation, so it never trips
// this gate. It does NOT call t.Parallel(): exception #5 of the t.Parallel() hard
// gate (this package runs goleak.VerifyTestMain).
func TestValidateSaaSDeclarationTLS(t *testing.T) {
	tests := []struct {
		name           string
		deploymentMode string
		enabled        bool
		idpHost        string
		wantErr        bool
	}{
		{name: "saas_enabled_http_refused", deploymentMode: "saas", enabled: true, idpHost: "http://identity:4001", wantErr: true},
		{name: "saas_enabled_https_allowed", deploymentMode: "saas", enabled: true, idpHost: "https://identity:4001"},
		{name: "saas_disabled_http_is_noop", deploymentMode: "saas", enabled: false, idpHost: "http://identity:4001"},
		{name: "byoc_enabled_http_allowed", deploymentMode: "byoc", enabled: true, idpHost: "http://identity:4001"},
		{name: "local_enabled_http_allowed", deploymentMode: "local", enabled: true, idpHost: "http://identity:4001"},
		{name: "unset_mode_enabled_http_allowed", deploymentMode: "", enabled: true, idpHost: "http://identity:4001"},
		{name: "saas_enabled_empty_host_is_noop", deploymentMode: "saas", enabled: true, idpHost: ""},
		{name: "saas_enabled_schemeless_host_is_noop", deploymentMode: "saas", enabled: true, idpHost: "identity.invalid"},
		{name: "saas_enabled_opaque_http_is_noop", deploymentMode: "saas", enabled: true, idpHost: "http:identity"},
		{name: "saas_enabled_rootless_http_is_noop", deploymentMode: "saas", enabled: true, idpHost: "http:/identity"},
		{name: "saas_enabled_uppercase_scheme_refused", deploymentMode: "saas", enabled: true, idpHost: "HTTP://identity:4001", wantErr: true},
		{name: "saas_uppercase_mode_http_refused", deploymentMode: "SAAS", enabled: true, idpHost: "http://identity:4001", wantErr: true},
		{name: "saas_whitespace_padded_mode_http_refused", deploymentMode: "  saas  ", enabled: true, idpHost: "http://identity:4001", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSaaSDeclarationTLS(tt.deploymentMode, tt.enabled, tt.idpHost)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "DEPLOYMENT_MODE=saas")
				assert.Contains(t, err.Error(), "idp_declaration")
				assert.Contains(t, err.Error(), "IDP_HOST")
				// The gate never receives the M2M secret, so it cannot leak it; assert
				// the guidance names only the host env var, never a credential token.
				assert.NotContains(t, err.Error(), "client_secret")
				assert.NotContains(t, err.Error(), "dummy-client-secret")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestBuildDeclarationPublishers_EnabledIncompleteConfigFailsOpen exercises the
// enabled-but-incomplete-config branch: RI is ON but IDP_HOST and both M2M
// credentials are empty. The pre-flight warnIncompleteDeclarationConfig names the
// empty env vars, then declaration.New rejects the empty IdentityAddr/credentials
// (lib-auth's validateConfig), so the publisher is Warn-skipped and
// the returned stops slice is empty. No goroutine is started (New fails before
// Start), so the package goleak check stays clean, and the helper must not panic.
func TestBuildDeclarationPublishers_EnabledIncompleteConfigFailsOpen(t *testing.T) {
	cfg := &Config{
		DeclarationEnabled: true,
		IDPHost:            "",
		IDPM2MClientID:     "",
		IDPM2MClientSecret: "",
	}

	stops := buildDeclarationPublishers(cfg, stubTokenMinter{}, libLog.NewNop())

	assert.Empty(t, stops, "enabled RI with empty IdP host/credentials must skip the publisher fail-open")
}
