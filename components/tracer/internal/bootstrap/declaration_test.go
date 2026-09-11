// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v4/auth/declaration"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
)

// stubTokenMinter is a no-op declaration.TokenMinter used where a non-nil minter
// is structurally required but no token is ever minted: declaration.New performs
// no I/O (it never mints), so the empty token is never observed. Not a credential.
type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// fixedTokenMinter mints a non-empty dummy token so the publisher's background
// Publish proceeds PAST the mint step to the PUT against the identity stub. An
// empty token would be classified as auth-disabled/misconfigured and short-circuit
// before the PUT, so the 5xx fail-open path would never run. Not a real credential.
type fixedTokenMinter struct{}

func (fixedTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "dummy-m2m-token", nil
}

// TestBuildDeclarationPublisher_DisabledReturnsNoStops asserts the helper is a
// no-op when RI declaration is disabled: it constructs no publisher, returns
// zero stop funcs, and never dereferences the TokenMinter (nil is passed here).
// So Run() registers no declaration runnable and boot is byte-identical to today.
//
// The enabled / fail-open / drain-runnable cases live in their own tests; this
// test stays scoped to the disabled-path contract only.
func TestBuildDeclarationPublisher_DisabledReturnsNoStops(t *testing.T) {
	t.Parallel()

	cfg := &Config{DeclarationEnabled: false}

	stops, err := buildDeclarationPublisher(cfg, nil, libLog.NewNop())
	require.NoError(t, err, "the disabled path validates nothing and never errors")

	assert.Empty(t, stops, "disabled declaration must yield no stop funcs")
}

// TestBuildDeclarationPublisher_EnabledIncompleteConfigFailsClosed pins the
// CONFIGURATION half of the failure policy for tracer's single slug: RI is ON but
// IDP_HOST and both M2M credentials are empty, which is deterministic operator
// error, so the helper returns an error and the caller aborts boot instead of
// serving with nothing declared.
//
// The error must name the three empty env vars and must never carry a value. No
// goroutine is started (validation precedes New), so there is nothing to leak.
func TestBuildDeclarationPublisher_EnabledIncompleteConfigFailsClosed(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		DeclarationEnabled: true,
		IDPHost:            "",
		IDPM2MClientID:     "",
		IDPM2MClientSecret: "",
	}

	var (
		stops []func()
		err   error
	)

	require.NotPanics(t, func() {
		stops, err = buildDeclarationPublisher(cfg, stubTokenMinter{}, libLog.NewNop())
	}, "the fail-closed path must return an error, never panic")

	require.Error(t, err, "enabled RI with empty IdP host/credentials must fail closed")
	assert.Empty(t, stops, "no publisher may be returned on the fail-closed path")
	assert.Contains(t, err.Error(), "IDP_HOST")
	assert.Contains(t, err.Error(), "IDP_M2M_CLIENT_ID")
	assert.Contains(t, err.Error(), "IDP_M2M_CLIENT_SECRET")
}

// TestValidateDeclarationConfig_NamesOnlyTheMissingFields mirrors the ledger's
// guard: a complete configuration validates, a partial one names only what is
// missing, and the secret value never reaches the error.
func TestValidateDeclarationConfig_NamesOnlyTheMissingFields(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateDeclarationConfig(&Config{
		DeclarationEnabled: true,
		IDPHost:            "https://identity.example.test",
		IDPM2MClientID:     "id",
		IDPM2MClientSecret: "secret",
	}), "a complete configuration must validate")

	err := validateDeclarationConfig(&Config{
		DeclarationEnabled: true,
		IDPHost:            "https://identity.example.test",
		IDPM2MClientID:     "",
		IDPM2MClientSecret: "shhh",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IDP_M2M_CLIENT_ID")
	assert.NotContains(t, err.Error(), "IDP_HOST", "a field that is set must not be reported missing")
	assert.NotContains(t, err.Error(), "shhh", "the secret VALUE must never reach the error")
}

// TestBuildDeclarationPublisher_IdentityAlways5xx_FailsOpenAndStopsDrain
// characterizes the fail-open posture for tracer's SINGLE service slug: with RI
// enabled and an identity that ALWAYS answers 5xx, the helper still returns
// successfully (boot is NOT blocked), builds exactly ONE publisher (tracer owns a
// single slug), drives its background PUT against the failing identity, and the
// returned stop() drains its goroutine without panic or deadlock.
//
// No t.Parallel(): this test relies on a bounded timeout guard and shares the
// package with tests that would otherwise interleave; keeping it sequential keeps
// the drain wait deterministic. It touches no process-global state on its own.
func TestBuildDeclarationPublisher_IdentityAlways5xx_FailsOpenAndStopsDrain(t *testing.T) {
	const stopDrainTimeout = 10 * time.Second

	// reqCh signals each PUT the identity stub receives, so the test can prove the
	// publisher actually exercised the failing-identity PUT path before draining
	// it. Buffered + non-blocking send so the handler never blocks on retry traffic
	// the test has stopped reading.
	reqCh := make(chan struct{}, 64)

	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Force per-request connection close so no idle client keep-alive goroutine
		// lingers past the test; this is about the httptest client transport, not
		// the code under test.
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
	stops, err := buildDeclarationPublisher(cfg, fixedTokenMinter{}, libLog.NewNop())
	require.NoError(t, err, "an identity answering 5xx is a RUNTIME failure and must stay fail-open")

	require.Len(t, stops, 1, "tracer owns a single slug, so exactly one publisher must be constructed and started")
	require.NotNil(t, stops[0], "the started publisher must yield a non-nil stop func")

	// Prove the publisher reached the failing-identity PUT (fail-open ran the real
	// publish path, not a short-circuit).
	select {
	case <-reqCh:
	case <-time.After(stopDrainTimeout):
		t.Fatal("expected the publisher PUT to reach the identity stub; the fail-open publish path was not exercised")
	}

	// stop() must cancel the publisher's context and drain its goroutine without
	// deadlocking. Run it off-thread and bound the wait so a hang is a reported
	// failure, not an indefinitely stuck test.
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

// TestBuildDeclarationPublisher_RealManifestMatchesTracerSlug proves the exact
// (slug, manifest) pair the helper wires satisfies declaration.New's client-side
// BOLA guard (slug == manifest.service): the real embedded manifest declares
// "tracer", and New rejects any cross-wiring. This is the deterministic, I/O-free
// proof that the single wired slug is "tracer"; New starts no goroutine here.
func TestBuildDeclarationPublisher_RealManifestMatchesTracerSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		slug            string
		manifest        []byte
		wantErrContains string
	}{
		{
			name:     "tracer slug matches embedded tracer manifest",
			slug:     "tracer",
			manifest: tracerembed.TracerManifest,
		},
		{
			name:            "non-tracer slug rejected against tracer manifest",
			slug:            "midaz",
			manifest:        tracerembed.TracerManifest,
			wantErrContains: `manifest.service "tracer"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

// TestDeclarationPublisherRunnable_EmptyStops_ReturnsImmediately covers the
// runnable's early-return contract: with a nil receiver or no stop hooks, Run must
// return nil WITHOUT registering a signal handler or blocking. Parallel-safe
// because these branches never touch process-global signal state.
func TestDeclarationPublisherRunnable_EmptyStops_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		r    *declarationPublisherRunnable
	}{
		{name: "nil receiver", r: nil},
		{name: "non-nil receiver with nil stops", r: &declarationPublisherRunnable{}},
		{name: "non-nil receiver with empty stops slice", r: &declarationPublisherRunnable{stops: []func(){}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			done := make(chan error, 1)

			go func() {
				done <- tt.r.Run(nil)
			}()

			select {
			case err := <-done:
				require.NoError(t, err, "empty-stops Run must return nil without waiting on a signal")
			case <-time.After(2 * time.Second):
				t.Fatal("Run with no stops must return immediately; it blocked on a signal instead")
			}
		})
	}
}

// TestDeclarationPublisherRunnable_SIGTERM_DrainsStopsExactlyOnceAndExits proves
// the shutdown contract of declarationPublisherRunnable: on SIGTERM it invokes
// every stop() exactly once and its Run goroutine terminates (no leak) within a
// bounded window.
//
// No t.Parallel() — by rule. This test registers a process-global SIGTERM handler
// (signal.Notify) and delivers a real SIGTERM to the test process, which is the
// only mechanism that unblocks the runnable's signal.NotifyContext wait as written.
// Process-global signal state is one of the t.Parallel() hard-gate exceptions
// (testing-unit.md), so the test stays sequential and restores the handler in
// t.Cleanup. The guard handler is registered BEFORE any signal is sent so SIGTERM's
// default terminate action stays suppressed regardless of when the runnable
// registers its own NotifyContext handler — that is what keeps a stray SIGTERM from
// killing the test binary. The runnable uses signal.NotifyContext (not signal.Reset),
// so the re-raise hazard documented in service_drain_test.go does not apply here.
func TestDeclarationPublisherRunnable_SIGTERM_DrainsStopsExactlyOnceAndExits(t *testing.T) {
	const drainTimeout = 10 * time.Second

	// Guard: keep SIGTERM's default (terminate) suppressed for the whole test,
	// closing the race window before the runnable registers its own handler.
	sigGuard := make(chan os.Signal, 1)
	signal.Notify(sigGuard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(sigGuard) })

	var stopCalls atomic.Int32

	stopped := make(chan struct{})
	r := &declarationPublisherRunnable{
		stops: []func(){
			func() {
				stopCalls.Add(1)
				close(stopped)
			},
		},
	}

	runReturned := make(chan error, 1)

	go func() {
		runReturned <- r.Run(nil)
	}()

	self, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)

	// Deliver SIGTERM now, then keep resending until Run observes it. Resending
	// defeats the race where Run has not yet reached NotifyContext registration;
	// the guard absorbs any signal landing outside that window, and extra signals
	// are coalesced by the buffered NotifyContext channel, so this is harmless.
	require.NoError(t, self.Signal(syscall.SIGTERM))

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.NewTimer(drainTimeout)
	defer deadline.Stop()

	for {
		select {
		case runErr := <-runReturned:
			require.NoError(t, runErr, "runnable Run must return nil after draining on SIGTERM")

			select {
			case <-stopped:
			default:
				t.Fatal("stop() channel was not closed: the runnable returned without invoking stop()")
			}

			assert.Equal(t, int32(1), stopCalls.Load(), "each publisher stop() must be invoked exactly once")

			return
		case <-ticker.C:
			_ = self.Signal(syscall.SIGTERM)
		case <-deadline.C:
			t.Fatal("runnable Run did not return after SIGTERM: possible deadlock or leaked goroutine in the drain path")
		}
	}
}
