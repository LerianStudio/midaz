// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/readyz"

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
	listenerCtx, cancelListener := context.WithCancel(context.Background())
	defer cancelListener()

	if app.drainState != nil {
		go app.runDrainListener(listenerCtx)
	}

	commons.NewLauncher(
		commons.WithLogger(app.Logger),
		commons.RunApp("HTTP Service", app.Server),
	).Run()

	// Graceful shutdown
	app.Info("Starting graceful shutdown...")

	if app.cleanup != nil {
		app.cleanup()
	}

	app.Info("Graceful shutdown complete")
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
