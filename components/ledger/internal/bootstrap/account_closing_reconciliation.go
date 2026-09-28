// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"
)

// accountClosingReconcileTimeout bounds one reconciliation pass so a slow
// dependency cannot hold the recovery cycle. What a pass does not reach keeps its
// protection and is picked up by the next one.
const accountClosingReconcileTimeout = 30 * time.Second

// reconcileAccountClosings resolves the closing protection earlier attempts left
// behind, once per recovery cycle.
//
// It rides the existing runner rather than a scheduler of its own, which is what
// gives it the leader lock, the per-tenant context and the shutdown handling this
// work needs: the scan is tenant-scoped, so running it under the same context the
// recovery consumers already resolved is what keeps one tenant's reconciliation
// out of another's keys.
//
// The pass never fails the cycle. A closing it cannot resolve keeps its
// protection, which is the outcome the reconciliation exists to preserve.
//
// The runner does not report the result. The use case is the single point that
// can: it knows which steps failed and whether both namespace walks completed,
// and a second report from here would describe the same pass with less of it.
func (r *RedisQueueConsumer) reconcileAccountClosings(ctx context.Context) {
	if r.Command == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, accountClosingReconcileTimeout)
	defer cancel()

	r.Command.ReconcileAccountClosings(ctx)
}
