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
	tmconsumer "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/consumer"
)

// multiTenantConsumerRunnable adapts *tmconsumer.MultiTenantConsumer to the
// mbootstrap.Runnable interface so the Launcher can manage its lifecycle.
type multiTenantConsumerRunnable struct {
	consumer *tmconsumer.MultiTenantConsumer
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

	<-ctx.Done()
	stop()

	return r.consumer.Close()
}
