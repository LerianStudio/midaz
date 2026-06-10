// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/pdf"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"

	"github.com/LerianStudio/lib-commons/v5/commons"
	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*MultiQueueConsumer
	log.Logger
	healthChecker      *pkg.HealthChecker
	healthServer       *HealthServer
	mongoConnection    *libMongo.MongoConnection
	rabbitMQConnection *libRabbitMQ.RabbitMQConnection
	pdfPool            *pdf.WorkerPool
	telemetry          *libOtel.Telemetry
	// mtConsumer is the multi-tenant consumer for per-tenant vhost isolation.
	// When non-nil, it takes precedence over the static RabbitMQ connection.
	// Shutdown of mtConsumer is handled by MultiQueueConsumer.Run() when context is canceled.
	mtConsumer MultiTenantConsumerInterface
	// mtCleanup is the cleanup function for multi-tenant resources (Redis, etc.)
	mtCleanup func()
	// eventListenerCleanup stops the tenant event listener and closes its dedicated Redis
	// Pub/Sub client. Nil when MULTI_TENANT_REDIS_HOST is not configured (lazy-load only).
	eventListenerCleanup func()
	// drainState is the shared graceful-shutdown flag, plumbed into both
	// MultiQueueConsumer and HealthServer so /readyz reports 503 draining
	// during shutdown.
	drainState *readyz.DrainState
	// selfProbeState gates the /health endpoint. Initialized at bootstrap
	// by readyz.RunSelfProbe: success → MarkHealthy() flips /health to
	// 200; failure → state stays unhealthy and K8s livenessProbe restarts
	// the pod. Plumbed into HealthServer via HealthServerConfig.
	selfProbeState *readyz.SelfProbeState
}

func (app *Service) Info(message string) {
	app.Log(context.Background(), log.LevelInfo, message)
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	app.StartHealthServer()

	commons.NewLauncher(
		commons.WithLogger(app.Logger),
		commons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
	).Run()

	app.Shutdown()
}

// StartHealthServer starts the worker health/readyz server before the
// consumer so probes are available immediately. Safe to call when no health
// server is configured (no-op).
func (app *Service) StartHealthServer() {
	if app.healthServer != nil {
		app.healthServer.Start()
	}
}

// ConsumerApp exposes the RabbitMQ consumer as a libCommons.App so a shared
// launcher can register and run it alongside other surfaces. The consumer
// owns its own SIGTERM-driven drain inside Run().
func (app *Service) ConsumerApp() commons.App {
	return app.MultiQueueConsumer
}

// Shutdown closes worker resources in reverse initialization order. It is
// invoked by Run() after the launcher unblocks, and by the unified app
// orchestrator after the shared launcher terminates. Every teardown step
// (health checker, health server, PDF pool, event listener, multi-tenant
// resources, RabbitMQ, MongoDB, telemetry flush) is preserved here so a
// graceful SIGTERM drains the same way under either caller.
func (app *Service) Shutdown() {
	// Graceful shutdown - close resources in reverse initialization order
	app.Info("Starting graceful shutdown...")

	// Stop health checker
	if app.healthChecker != nil {
		app.Info("Stopping health checker...")
		app.healthChecker.Stop()
	}

	// Stop health HTTP server
	if app.healthServer != nil {
		app.Info("Stopping health server...")
		app.healthServer.Shutdown()
		app.Info("Health server stopped")
	}

	// Close PDF worker pool (waits for in-progress tasks to complete)
	if app.pdfPool != nil {
		app.Info("Closing PDF worker pool...")
		app.pdfPool.Close()
		app.Info("PDF worker pool closed")
	}

	// Stop tenant event listener before closing infrastructure.
	// This prevents new tenant lifecycle events from triggering EnsureConsumerStarted
	// after the consumer has already been shut down.
	if app.eventListenerCleanup != nil {
		app.Info("Stopping tenant event listener...")
		app.eventListenerCleanup()
		app.Info("Tenant event listener stopped")
	}

	// Close multi-tenant bootstrap resources if present.
	// mtConsumer.Close() is handled by MultiQueueConsumer.Run() on context cancellation.
	// mtCleanup closes the tenant RabbitMQ manager and Redis connection.
	if app.mtCleanup != nil {
		app.Info("Closing multi-tenant bootstrap resources...")
		app.mtCleanup()
		app.Info("Multi-tenant resources closed")
	}

	// Close RabbitMQ connection (only for single-tenant mode)
	// In multi-tenant mode, connections are managed by tmrabbitmq.Manager
	if app.rabbitMQConnection != nil && app.mtConsumer == nil {
		app.Info("Closing RabbitMQ connection...")

		if app.rabbitMQConnection.Channel != nil {
			if err := app.rabbitMQConnection.Channel.Close(); err != nil {
				app.Log(context.Background(), log.LevelError, "Failed to close RabbitMQ channel", log.Err(err))
			}
		}

		if app.rabbitMQConnection.Connection != nil && !app.rabbitMQConnection.Connection.IsClosed() {
			if err := app.rabbitMQConnection.Connection.Close(); err != nil {
				app.Log(context.Background(), log.LevelError, "Failed to close RabbitMQ connection", log.Err(err))
			} else {
				app.Info("RabbitMQ connection closed")
			}
		}
	}

	// Close MongoDB connection
	if app.mongoConnection != nil {
		app.Info("Closing MongoDB connection...")

		if err := app.mongoConnection.Close(); err != nil {
			app.Log(context.Background(), log.LevelError, "Failed to close MongoDB connection", log.Err(err))
		} else {
			app.Info("MongoDB connection closed")
		}
	}

	// Flush telemetry (must be last to capture shutdown spans)
	if app.telemetry != nil {
		app.Info("Flushing telemetry...")
		app.telemetry.ShutdownTelemetry()
		app.Info("Telemetry flushed")
	}

	app.Info("Graceful shutdown complete")

	// Flush buffered records after the Launcher and cleanup have logged their
	// final lines. Must be last so it captures the shutdown lines themselves.
	_ = app.Sync(context.Background())
}
