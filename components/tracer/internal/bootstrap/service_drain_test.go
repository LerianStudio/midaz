// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/http/in"
)

// newDrainTestService builds a Service wired with a healthChecker so the
// drain ordering can be observed without needing the full bootstrap chain.
func newDrainTestService(t *testing.T, cfg *Config) (*Service, *in.HealthChecker) {
	t.Helper()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	logger := libLog.NewNop()
	tel := libOtel.Telemetry{}

	hs, err := NewHTTPServer(&Config{ServerAddress: ":0"}, app, logger, &tel)
	require.NoError(t, err)

	hc := in.NewTestableHealthChecker(nil)

	svc := &Service{
		HTTPServer:    hs,
		Logger:        logger,
		healthChecker: hc,
		config:        cfg,
	}

	return svc, hc
}

// TestService_Shutdown_MarksDrainingBeforeServerShutdown verifies the
// CRITICAL drain-then-shutdown ordering: the readiness flip MUST precede
// ShutdownWithContext, otherwise K8s sees the pod healthy until the moment
// connections are torn down — guaranteeing dropped in-flight requests.
func TestService_Shutdown_MarksDrainingBeforeServerShutdown(t *testing.T) {
	cfg := &Config{ReadyzDrainGraceSeconds: 1}
	svc, hc := newDrainTestService(t, cfg)

	require.False(t, hc.IsDraining(), "draining must start false")

	// Run shutdown in a goroutine so we can observe the draining flip while
	// the grace sleep is still running.
	done := make(chan error, 1)
	go func() {
		done <- svc.Shutdown(context.Background())
	}()

	// Poll for the draining flag — must flip almost immediately, well before
	// the 1s grace elapses. 200ms is generous for a single atomic store.
	assert.Eventually(t, hc.IsDraining, 200*time.Millisecond, 5*time.Millisecond,
		"MarkDraining must fire BEFORE ShutdownWithContext returns")

	require.NoError(t, <-done)
}

// TestService_Shutdown_GracePeriodElapses asserts the configured grace
// duration is honored when the parent context is not cancelled. The total
// shutdown wall-clock time must be ≥ grace.
func TestService_Shutdown_GracePeriodElapses(t *testing.T) {
	cfg := &Config{ReadyzDrainGraceSeconds: 1}
	svc, _ := newDrainTestService(t, cfg)

	start := time.Now()
	require.NoError(t, svc.Shutdown(context.Background()))

	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 1*time.Second,
		"shutdown must wait at least the configured grace duration")
	// Upper bound prevents the test from accidentally hiding a runaway timer.
	assert.Less(t, elapsed, 3*time.Second,
		"shutdown must not vastly exceed the grace duration")
}

// TestService_Shutdown_RespectsParentContext asserts that a cancelled parent
// context aborts the grace sleep. Operators must be able to bail out of a
// stuck shutdown without waiting the full grace window.
func TestService_Shutdown_RespectsParentContext(t *testing.T) {
	// 30s grace would normally make the test slow; cancellation must cut
	// that short.
	cfg := &Config{ReadyzDrainGraceSeconds: 30}
	svc, _ := newDrainTestService(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a brief delay so MarkDraining + sleep entry happen first.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	require.NoError(t, svc.Shutdown(ctx))

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 5*time.Second,
		"cancelled parent ctx must cut the grace sleep short")
}

// TestService_Shutdown_NoHealthChecker tolerates a Service with no
// healthChecker (defensive — should not panic). Belt-and-braces for paths
// that build a partial Service in tests.
func TestService_Shutdown_NoHealthChecker(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	logger := libLog.NewNop()
	tel := libOtel.Telemetry{}

	hs, err := NewHTTPServer(&Config{ServerAddress: ":0"}, app, logger, &tel)
	require.NoError(t, err)

	svc := &Service{
		HTTPServer: hs,
		Logger:     logger,
		// healthChecker intentionally nil
		config: &Config{ReadyzDrainGraceSeconds: 0},
	}

	require.NotPanics(t, func() {
		_ = svc.Shutdown(context.Background())
	})
}

// TestService_InstallDrainHandler_StopReleasesGoroutine verifies C1
// plumbing: installDrainHandler returns a stop function that, when
// called, unblocks and reclaims the listener goroutine even when no
// signal was delivered. This is the test-cleanup path that runs after
// every Run() call; without it the goroutine would leak across tests.
//
// Production behavior — actually firing the drain on SIGTERM — is
// covered indirectly by TestService_Shutdown_MarksDrainingBeforeServerShutdown
// (the Shutdown method) because the drain handler delegates to it.
// Driving real OS signals in a unit test is fraught: signal.Reset
// inside the handler removes any guard the test installed, so a
// re-raise can kill the test runner. The cleanest contract test is
// "stop is honored" — the production wiring (handler ↔ Shutdown) is
// exercised by the Shutdown tests above.
func TestService_InstallDrainHandler_StopReleasesGoroutine(t *testing.T) {
	cfg := &Config{ReadyzDrainGraceSeconds: 0}
	svc, _ := newDrainTestService(t, cfg)

	stop := svc.installDrainHandler()

	// Stop must return promptly without an OS signal — the listener
	// goroutine sees the channel close and exits.
	done := make(chan struct{})

	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("installDrainHandler stop did not release the goroutine within 2s")
	}
}
