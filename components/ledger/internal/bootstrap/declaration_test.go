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

	"github.com/LerianStudio/lib-auth/v3/auth/declaration"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
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
// blocked), builds both publishers, drives their background PUTs against the
// failing identity, and every returned stop() drains its goroutine without
// panic or deadlock. goleak.VerifyTestMain then proves no publisher goroutine
// survives the package run.
func TestBuildDeclarationPublishers_IdentityAlways5xx_FailsOpenAndStopsDrain(t *testing.T) {
	const stopDrainTimeout = 10 * time.Second

	// reqCh signals each PUT the identity stub receives, so the test can prove
	// both publishers actually exercised the failing-identity PUT path before
	// draining them. Buffered + non-blocking send so the handler never blocks on
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

	require.Len(t, stops, 2, "both midaz and plugin-fees publishers must be constructed and started")

	for _, s := range stops {
		require.NotNil(t, s, "each started publisher must yield a non-nil stop func")
	}

	// Prove both publishers reached the failing-identity PUT (fail-open ran the
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
// (slug, manifest) pairs the helper wires satisfy declaration.New's client-side
// BOLA guard (slug == manifest.service): the real embedded manifests declare
// "midaz" and "plugin-fees", and New rejects any cross-wiring. This is the
// deterministic, I/O-free proof that the two wired slugs are correct; New starts
// no goroutine here.
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
			name:     "plugin-fees slug matches embedded fees manifest",
			slug:     "plugin-fees",
			manifest: feesservices.FeesManifest,
		},
		{
			name:            "midaz slug rejected against fees manifest",
			slug:            "midaz",
			manifest:        feesservices.FeesManifest,
			wantErrContains: `manifest.service "plugin-fees"`,
		},
		{
			name:            "plugin-fees slug rejected against midaz manifest",
			slug:            "plugin-fees",
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
