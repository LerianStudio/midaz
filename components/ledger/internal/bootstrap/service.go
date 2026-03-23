// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	tmconsumer "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/consumer"
)

// Service is the unified ledger service that owns all infrastructure directly.
type Service struct {
	UnifiedServer         *UnifiedServer
	MultiQueueConsumer    *MultiQueueConsumer
	MultiTenantConsumer   *tmconsumer.MultiTenantConsumer
	RedisQueueConsumer    *RedisQueueConsumer
	BalanceSyncWorker     *BalanceSyncWorker
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

	// Balance sync worker (always enabled)
	if s.BalanceSyncWorker != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Balance Sync Worker", s.BalanceSyncWorker))
	}

	// Circuit breaker health checker
	if s.CircuitBreakerManager != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Circuit Breaker Health Checker",
			NewCircuitBreakerRunnable(s.CircuitBreakerManager)))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}
