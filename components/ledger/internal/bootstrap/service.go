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
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/lib-observability/metrics"
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

	// StreamingClose is the close hook for the lib-streaming producer. It
	// is non-nil for both the real producer and the NoopEmitter — callers
	// register it as a Launcher app so a graceful shutdown drains buffered
	// records before exit. Registered conditionally so the Launcher does
	// not start an entry that has nothing to do (e.g. when streaming is
	// disabled and the close func is the no-op).
	StreamingClose func() error
	// StreamingEnabled mirrors the config flag so Run() can decide whether
	// to register the producer-shutdown Launcher app.
	StreamingEnabled bool
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

	// Streaming producer: register only when streaming is actually enabled
	// AND we have a non-nil close hook. The NoopEmitter path is skipped to
	// keep the Launcher app list lean.
	if s.StreamingEnabled && s.StreamingClose != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Streaming Producer",
			&streamingProducerRunnable{close: s.StreamingClose, logger: s.Logger}))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// streamingProducerRunnable adapts the lib-streaming Producer's Close hook
// to the libCommons.App interface. It blocks until SIGINT/SIGTERM and then
// runs the producer's drain/flush close path so buffered records reach the
// broker before the process exits.
type streamingProducerRunnable struct {
	close  func() error
	logger libLog.Logger
}

// Run blocks until SIGINT/SIGTERM and then invokes the producer close hook.
// The close hook is responsible for draining records under
// STREAMING_CLOSE_TIMEOUT_S; a non-nil return is logged but not propagated
// because at shutdown the Launcher cannot meaningfully react.
func (r *streamingProducerRunnable) Run(_ *libCommons.Launcher) error {
	if r == nil || r.close == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	if err := r.close(); err != nil && r.logger != nil {
		r.logger.Log(context.Background(), libLog.LevelWarn,
			"streaming producer Close returned error",
			libLog.String("error", err.Error()),
		)
	}

	return nil
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
