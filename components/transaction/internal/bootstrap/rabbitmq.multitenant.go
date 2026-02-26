// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

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
// per-tenant consumer goroutines in lazy mode.
func (r *multiTenantConsumerRunnable) Run(_ *libCommons.Launcher) error {
	if r.consumer == nil {
		return nil
	}

	return r.consumer.Run(context.Background())
}
