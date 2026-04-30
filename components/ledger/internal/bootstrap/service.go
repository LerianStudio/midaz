// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
)

// Service is the unified ledger service that owns all infrastructure directly.
type Service struct {
	UnifiedServer            *UnifiedServer
	MultiQueueConsumer       *MultiQueueConsumer
	MultiTenantConsumer      *tmconsumer.MultiTenantConsumer
	RedisQueueConsumer       *RedisQueueConsumer
	BalanceSyncWorker        *BalanceSyncWorker
	LegacyBalanceSyncDrainer *LegacyBalanceSyncDrainer
	EventListener            *tmevent.TenantEventListener
	CircuitBreakerManager    *CircuitBreakerManager
	Logger                   libLog.Logger
	Telemetry                *libOpentelemetry.Telemetry
	metricsFactory           *metrics.MetricsFactory
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

	// Legacy balance sync drainer — drains pre-v3.6.2 ZSET entries
	if s.LegacyBalanceSyncDrainer != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Legacy Balance Sync Drainer", s.LegacyBalanceSyncDrainer))
	}

	// Tenant event listener (Redis Pub/Sub)
	if s.EventListener != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Tenant Event Listener",
			&eventListenerRunnable{listener: s.EventListener}))
	}

	// Circuit breaker health checker
	if s.CircuitBreakerManager != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Circuit Breaker Health Checker",
			NewCircuitBreakerRunnable(s.CircuitBreakerManager)))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// eventListenerRunnable adapts a TenantEventListener to the libCommons.App interface.
// It starts the Redis Pub/Sub listener and blocks until SIGINT/SIGTERM is received,
// matching the shutdown pattern of other runnables in this package.
type eventListenerRunnable struct {
	listener *tmevent.TenantEventListener
}

// Run starts the event listener and blocks until SIGINT/SIGTERM.
func (r *eventListenerRunnable) Run(_ *libCommons.Launcher) error {
	if r.listener == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := r.listener.Start(ctx); err != nil {
		stop()

		return err
	}

	<-ctx.Done()
	stop()

	return r.listener.Stop()
}
