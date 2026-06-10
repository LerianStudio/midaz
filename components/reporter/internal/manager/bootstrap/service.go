// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
)

// Service is the application glue where we put all top-level components to be used.
type Service struct {
	*Server
	log.Logger
	cleanup func()
	// drainState is the shared graceful-shutdown flag. The signal listener
	// goroutine launched in Run() flips this to true on SIGTERM/SIGINT so
	// /readyz short-circuits to 503 BEFORE lib-commons begins tearing the
	// server down. K8s and load balancers see the unready state and stop
	// routing new traffic while in-flight requests complete.
	//
	// lib-commons retains ownership of the actual app.Shutdown() call —
	// this listener only updates readiness state.
	drainState *readyz.DrainState
	// SelfProbeState gates the /health endpoint. Initialized at bootstrap
	// (in Run() right after lib-commons starts) by readyz.RunSelfProbe:
	// success → MarkHealthy() flips /health to 200; failure → state stays
	// unhealthy and K8s livenessProbe restarts the pod. Exported because
	// the Manager's HTTP routes read it directly via ManagerReadyzDeps.
	SelfProbeState *readyz.SelfProbeState
}

func (app *Service) Info(message string) {
	app.Log(context.Background(), log.LevelInfo, message)
}

// Run starts the application.
// This is the only necessary code to run an app in the main.go.
func (app *Service) Run() {
	// Background goroutine that flips the drain flag on SIGTERM/SIGINT.
	// Runs in parallel with commons.RunApp (which blocks below) so /readyz
	// can report draining BEFORE lib-commons begins server.Shutdown().
	//
	// We deliberately use a separate signal channel rather than hooking into
	// lib-commons' shutdown so /readyz returns 503 the moment the signal
	// arrives — not after lib-commons has already torn down the listener.
	stopListener := app.StartDrainListener()
	defer stopListener()

	commons.NewLauncher(
		commons.WithLogger(app.Logger),
		commons.RunApp("HTTP Service", app.Server),
	).Run()

	app.Shutdown()
}

// StartDrainListener launches the background goroutine that flips the drain
// flag on SIGTERM/SIGINT so /readyz reports 503 draining BEFORE lib-commons
// begins server.Shutdown(). It returns a stop func that cancels the listener
// goroutine; callers must invoke it after the launcher terminates. When no
// drain state is configured the listener is not started and the stop func is
// a no-op. Exported so the unified app orchestrator can drive the manager
// surface from a shared launcher.
func (app *Service) StartDrainListener() (stop func()) {
	listenerCtx, cancelListener := context.WithCancel(context.Background())

	if app.drainState != nil {
		go app.runDrainListener(listenerCtx)
	}

	return cancelListener
}

// HTTPApp exposes the manager HTTP server as a libCommons.App so a shared
// launcher can register and run it alongside other surfaces. The server owns
// its own SIGTERM-driven graceful shutdown inside Run().
func (app *Service) HTTPApp() commons.App {
	return app.Server
}

// Shutdown runs the manager's consolidated resource cleanup (Redis ->
// RabbitMQ -> MongoDB -> Telemetry, in reverse init order) and flushes the
// logger. Invoked by Run() after the launcher unblocks, and by the unified
// app orchestrator after the shared launcher terminates.
func (app *Service) Shutdown() {
	// Graceful shutdown
	app.Info("Starting graceful shutdown...")

	if app.cleanup != nil {
		app.cleanup()
	}

	app.Info("Graceful shutdown complete")

	// Flush buffered records after the Launcher and cleanup have logged their
	// final lines. Must be last so it captures the shutdown lines themselves.
	_ = app.Sync(context.Background())
}

// runDrainListener watches for SIGTERM/SIGINT and flips the shared drain
// flag so /readyz can report 503 draining before the underlying server
// begins to shut down. It exits when ctx is cancelled (Run's defer
// cancelListener) so it never outlives the process.
func (app *Service) runDrainListener(ctx context.Context) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sigs)

	select {
	case <-sigs:
		app.drainState.StartDraining()
		app.Log(ctx, log.LevelInfo, "drain_started")
	case <-ctx.Done():
		// Run() returned before any signal arrived — nothing to do.
	}
}
