// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	tmconsumer "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/consumer"
	tmwatcher "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/watcher"
)

// Service is the unified ledger service that owns all infrastructure directly.
type Service struct {
	UnifiedServer         *UnifiedServer
	MultiQueueConsumer    *MultiQueueConsumer
	MultiTenantConsumer   *tmconsumer.MultiTenantConsumer
	RedisQueueConsumer    *RedisQueueConsumer
	BalanceSyncWorker     *BalanceSyncWorker
	SettingsWatcher       *tmwatcher.SettingsWatcher
	CircuitBreakerManager *CircuitBreakerManager
	Logger                libLog.Logger
	Telemetry             *libOpentelemetry.Telemetry
	metricsFactory        *metrics.MetricsFactory
}

// Run starts the unified ledger service with all APIs on a single port.
// Workers (RabbitMQ, Redis consumers, balance sync) are started directly.
func (s *Service) Run() {
	s.Logger.Log(context.Background(), libLog.LevelInfo, "Running unified ledger service with single-port mode")

	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.Logger),
		libCommons.RunApp("Unified HTTP Server", s.UnifiedServer),
	}

	// RabbitMQ consumer (single-tenant or multi-tenant)
	if s.MultiQueueConsumer != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("RabbitMQ Consumer", s.MultiQueueConsumer))
	}

	if s.MultiTenantConsumer != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Multi-Tenant RabbitMQ Consumer",
			&multiTenantConsumerRunnable{consumer: s.MultiTenantConsumer, metricsFactory: s.metricsFactory}))
	}

	// Redis queue consumer
	if s.RedisQueueConsumer != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Redis Queue Consumer", s.RedisQueueConsumer))
	}

	// Balance sync worker (optional, started when configured)
	if s.BalanceSyncWorker != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Balance Sync Worker", s.BalanceSyncWorker))
	}

	// Settings watcher (multi-tenant only: revalidates PG pool settings periodically)
	if s.SettingsWatcher != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Settings Watcher",
			&settingsWatcherRunnable{watcher: s.SettingsWatcher, logger: s.Logger}))
	}

	// Circuit breaker health checker
	if s.CircuitBreakerManager != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Circuit Breaker Health Checker",
			NewCircuitBreakerRunnable(s.CircuitBreakerManager)))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// settingsWatcherRunnable adapts *tmwatcher.SettingsWatcher to the
// mbootstrap.Runnable interface so the Launcher can manage its lifecycle.
// It starts the watcher in the foreground and stops it gracefully on
// SIGINT/SIGTERM, matching the shutdown pattern of other runnables in
// this package (multiTenantConsumerRunnable, RedisQueueConsumer).
type settingsWatcherRunnable struct {
	watcher *tmwatcher.SettingsWatcher
	logger  libLog.Logger
}

// Run implements mbootstrap.Runnable.
// It starts the SettingsWatcher which periodically revalidates PostgreSQL
// connection pool settings for all registered managers. The watcher is
// stopped gracefully on SIGINT/SIGTERM.
func (r *settingsWatcherRunnable) Run(_ *libCommons.Launcher) error {
	if r.watcher == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	r.watcher.Start(ctx)

	r.logger.Log(ctx, libLog.LevelInfo, "SettingsWatcher started for PostgreSQL pool settings revalidation")

	<-ctx.Done()
	stop()

	r.watcher.Stop()

	r.logger.Log(context.Background(), libLog.LevelInfo, "SettingsWatcher stopped")

	return nil
}
