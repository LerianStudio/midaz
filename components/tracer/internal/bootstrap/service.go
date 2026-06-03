// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"

	"tracer/internal/adapters/http/in"
	"tracer/internal/services/workers"
)

// Service is the application glue where we put all top level components to be used.
//
// In single-tenant mode, cleanupWorker and syncWorker are populated and run as
// singletons via the Launcher. In multi-tenant mode those two fields are nil
// and the supervisor+listener pair drives per-tenant workers instead.
//
// healthChecker + config are wired here so Shutdown can flip /readyz into the
// draining state and honour the operator-tunable grace period (Gate 7).
type Service struct {
	*HTTPServer
	libLog.Logger
	postgresConn  *libPostgres.Client
	cleanupWorker *workers.UsageCleanupWorker
	syncWorker    *workers.RuleSyncWorker

	// Multi-tenant components (nil in single-tenant mode).
	pgManager     *tmpostgres.Manager
	supervisor    *workers.WorkerSupervisor
	eventListener *tenantListenerApp
	tmClient      *tmclient.Client

	// healthChecker is the SAME instance that's wired into routes.go via
	// RoutesDeps.HealthChecker. Shutdown calls MarkDraining on it BEFORE
	// any other shutdown step so /readyz starts returning 503 while the
	// pod is still serving in-flight requests. Nil in tests that bypass
	// the full bootstrap chain — Shutdown handles that case defensively.
	healthChecker *in.HealthChecker

	// config is retained for shutdown-time knob lookup (drain grace
	// duration). Holding the *Config rather than copying the field keeps a
	// single source of truth — env reloads (if ever introduced) propagate
	// without restarting Service.
	config *Config
}

// Run starts the application.
// This is the only necessary code to run an app in main.go.
//
// C1/H7: before delegating to the Launcher, Run installs an early SIGTERM
// handler in its own goroutine. The handler:
//
//  1. flips healthChecker.MarkDraining → /readyz starts returning 503
//     immediately;
//  2. sleeps READYZ_DRAIN_GRACE_SECONDS so K8s observes the 503 and
//     removes the pod from Service endpoints BEFORE the underlying
//     ServerManager begins tearing down sockets;
//  3. closes the cleanup chain (event listener → supervisor → pgManager →
//     tmClient → postgres pool) — same ordering as the legacy
//     Service.Shutdown but actually firing in production now.
//  4. re-raises SIGTERM so lib-commons' ServerManager picks it up and
//     drives the HTTP+worker shutdowns through its standard graceful
//     shutdown path. We can't pre-empt the Launcher's loop, only get
//     ahead of its signal handler.
//
// Without this, Service.Shutdown was never invoked in production:
// cmd/app/main.go calls only service.Run, which previously delegated
// straight to NewLauncher.Run with the sub-services registered via RunApp
// — Service itself was never registered, so MarkDraining + the 12s grace
// window never fired. /readyz never reported draining=true, K8s saw the
// pod healthy until the moment connections dropped, and rolling deploys
// killed in-flight requests. The READYZ_DRAIN_GRACE_SECONDS env var was
// inert.
func (app *Service) Run() {
	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("HTTP Service", app.HTTPServer),
	}

	// Multi-tenant: supervisor manages per-tenant workers; listener feeds it.
	// When these are wired, the singleton workers are intentionally nil.
	if app.supervisor != nil {
		opts = append(opts, libCommons.RunApp("Worker Supervisor", app.supervisor))
	}

	if app.eventListener != nil {
		opts = append(opts, libCommons.RunApp("Tenant Event Listener", app.eventListener))
	}

	// Single-tenant: register singleton workers with the Launcher.
	if app.cleanupWorker != nil {
		opts = append(opts, libCommons.RunApp("Usage Cleanup Worker", app.cleanupWorker))
	}

	if app.syncWorker != nil {
		opts = append(opts, libCommons.RunApp("Rule Sync Worker", app.syncWorker))
	}

	// Install the drain handler BEFORE the Launcher starts so its
	// signal.NotifyContext registration doesn't race the kubelet's
	// SIGTERM. installDrainHandler is a no-op when healthChecker is nil
	// (defensive — pure-test bootstrap paths can build a Service without
	// the readiness checker).
	stopDrain := app.installDrainHandler()
	defer stopDrain()

	// Run all services (blocks until shutdown).
	libCommons.NewLauncher(opts...).Run()
}

// installDrainHandler arms a SIGTERM/SIGINT pre-handler that runs the
// drain ordering (MarkDraining → grace window → ordered cleanup) before
// the Launcher's own shutdown path takes over. Returns a function that
// must be deferred by the caller — it stops the signal listener so the
// goroutine is reclaimed in tests that build a Service and never send a
// signal.
//
// The handler runs inside runtime.SafeGoWithContextAndComponent so a
// panic during shutdown still produces a panic_recovered_total counter
// and a log line — drain-time bugs must be observable in dashboards.
func (app *Service) installDrainHandler() func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	stopped := make(chan struct{})

	libRuntime.SafeGoWithContextAndComponent(
		context.Background(),
		app.Logger,
		"bootstrap.drain",
		"sigterm-pre-handler",
		libRuntime.KeepRunning,
		func(ctx context.Context) {
			defer close(stopped)

			sig, ok := <-sigCh
			if !ok {
				return
			}

			// Inherit ctx so the SafeGo span context propagates through
			// Shutdown's tracing surface — contextcheck rejects building a
			// fresh context.Background() chain when an upstream ctx exists.
			drainCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			if err := app.Shutdown(drainCtx); err != nil {
				app.Logger.With(
					libLog.String("error.message", err.Error()),
				).Log(ctx, libLog.LevelError, "drain handler: Shutdown returned error")
			}

			// Re-raise the signal so the Launcher's underlying
			// signal.NotifyContext registration fires and starts the
			// HTTP/worker shutdowns through the standard ServerManager
			// graceful-shutdown path. signal.Reset clears our own handler
			// so the second delivery hits the default handler chain.
			signal.Reset(sig)

			if process, err := os.FindProcess(os.Getpid()); err == nil {
				_ = process.Signal(sig)
			}
		},
	)

	return func() {
		signal.Stop(sigCh)
		close(sigCh)
		<-stopped
	}
}

// Shutdown gracefully shuts down the application.
//
// CRITICAL ORDER (Gate 7):
//  1. MarkDraining — flips /readyz to 503 immediately so K8s stops routing
//     new traffic to this pod. Without this, K8s sees the pod healthy until
//     the moment connections are torn down, killing in-flight requests.
//  2. Grace window — gives K8s readinessProbe time to observe the 503 and
//     update endpoints. Default 12s (configurable via
//     READYZ_DRAIN_GRACE_SECONDS) sized for periodSeconds=5 ×
//     failureThreshold=2 plus buffer.
//  3. ShutdownWithContext — stops accepting new HTTP requests; in-flight
//     requests get fiber's per-handler context to drain.
//  4. Worker / multi-tenant cleanup — supervisor, listener, pgManager, then
//     tmClient. The order matters (see inline comments below).
//  5. PostgreSQL pool close — last so any worker shutting down can still use
//     the pool until its goroutine exits.
//
// Reversing steps 1-3 is FORBIDDEN: it produces dropped in-flight requests
// during rolling deploys.
func (app *Service) Shutdown(ctx context.Context) error {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	// Step 1: flip drainingState. /readyz starts returning 503 immediately
	// so K8s removes the pod from service endpoints during the grace window.
	if app.healthChecker != nil {
		app.healthChecker.MarkDraining()
		logger.With(
			libLog.String("service.name", "HTTP Service"),
		).Log(ctx, libLog.LevelInfo, "draining_state_set")
	}

	// Step 2: grace window. select honors the parent context — operators
	// running an explicit cancel/SIGINT can cut the wait short rather than
	// being forced to oversleep the full grace period.
	graceDuration := drainGracePeriod(app.config)
	if graceDuration > 0 {
		logger.With(
			libLog.String("service.name", "HTTP Service"),
			libLog.String("grace.duration", graceDuration.String()),
		).Log(ctx, libLog.LevelInfo, "drain_grace_started")

		timer := time.NewTimer(graceDuration)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}

	if app.HTTPServer != nil && app.app != nil {
		if err := app.app.ShutdownWithContext(ctx); err != nil {
			logger.With(
				libLog.String("service.name", "HTTP Service"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "failed to shutdown HTTP server")

			return err
		}
	}

	// The cleanup worker uses signal.NotifyContext for graceful shutdown.
	// When running via Launcher, shutdown is coordinated through OS signals.
	// For programmatic shutdown scenarios, the worker stops when its context is cancelled.
	if app.cleanupWorker != nil {
		logger.With(
			libLog.String("service.name", "Usage Cleanup Worker"),
		).Log(ctx, libLog.LevelInfo, "cleanup worker shutdown is managed by Launcher via OS signals")
	}

	if app.syncWorker != nil {
		logger.With(
			libLog.String("service.name", "Rule Sync Worker"),
		).Log(ctx, libLog.LevelInfo, "rule sync worker shutdown is managed by Launcher via OS signals")
	}

	// Multi-tenant: stop the event listener (which unblocks its Run loop) and
	// the supervisor (which tears down every per-tenant worker set). Ordering
	// matters: stop the listener first so no new EnsureWorkers callbacks can
	// race with the supervisor shutting down.
	if app.eventListener != nil {
		app.eventListener.Shutdown()
	}

	if app.supervisor != nil {
		// Shutdown is intentionally context-less: it blocks until every
		// tenant's workers exit, which must not be cut short.
		//nolint:contextcheck
		app.supervisor.Shutdown()
	}

	if app.pgManager != nil {
		if err := app.pgManager.Close(ctx); err != nil {
			logger.With(
				libLog.String("service.name", "Tenant Postgres Manager"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to close tenant postgres manager")
		}
	}

	// Close the Tenant Manager HTTP client AFTER the pgManager. The pgManager's
	// LRU may evict tenant pools during Close and (in some lib-commons paths)
	// call back into the tmClient for metrics/telemetry; keeping the client
	// alive until the manager is fully drained avoids "use of closed client"
	// warnings during shutdown.
	if app.tmClient != nil {
		if err := app.tmClient.Close(); err != nil {
			logger.With(
				libLog.String("service.name", "Tenant Manager Client"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to close tenant-manager client")
		}
	}

	// Close the PostgreSQL connection pool to release database connections.
	// This is critical for repeated restarts (e.g., integration tests with
	// RestartServerWithConfig) to avoid exhausting the database's max_connections.
	if app.postgresConn != nil {
		if err := app.postgresConn.Close(); err != nil {
			logger.With(
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to close PostgreSQL connection pool")
		}
	}

	return nil
}
