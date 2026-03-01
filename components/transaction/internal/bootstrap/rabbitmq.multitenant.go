// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/lib-commons/v3/commons/opentelemetry/metrics"
	tmconsumer "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/consumer"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// multiTenantConsumerRunnable adapts *tmconsumer.MultiTenantConsumer to the
// mbootstrap.Runnable interface so the Launcher can manage its lifecycle.
type multiTenantConsumerRunnable struct {
	consumer       *tmconsumer.MultiTenantConsumer
	metricsFactory *metrics.MetricsFactory // nil when telemetry disabled; used for tenant_consumers_active gauge
}

// Run implements mbootstrap.Runnable.
// It starts the multi-tenant consumer which discovers tenants and spawns
// per-tenant consumer goroutines in lazy mode. The consumer is stopped
// gracefully on SIGINT/SIGTERM, matching the shutdown pattern of other
// runnables in this package (RedisQueueConsumer, BalanceSyncWorker).
func (r *multiTenantConsumerRunnable) Run(_ *libCommons.Launcher) error {
	if r.consumer == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := r.consumer.Run(ctx); err != nil {
		stop()
		return err
	}

	// Emit tenant_consumers_active gauge: 1 = consumer running
	if r.metricsFactory != nil {
		r.metricsFactory.Gauge(utils.TenantConsumersActive).Set(ctx, 1)
	}

	<-ctx.Done()
	stop()

	// Emit tenant_consumers_active gauge: 0 = consumer stopping
	if r.metricsFactory != nil {
		r.metricsFactory.Gauge(utils.TenantConsumersActive).Set(context.Background(), 0)
	}

	return r.consumer.Close()
}
