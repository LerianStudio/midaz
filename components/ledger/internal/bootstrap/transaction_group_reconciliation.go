// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"
)

// transactionGroupReconcileTimeout bounds one reconciliation pass so a slow
// dependency cannot hold the recovery cycle. What a pass does not reach is picked
// up by the next one.
const transactionGroupReconcileTimeout = 30 * time.Second

// reconcileTransactionGroups aligns PENDING cross-ledger group rows with their
// members, once per recovery cycle.
//
// It rides the existing runner for the same reasons as the account closing
// reconciliation: the leader lock keeps two pods from aligning the same row, and
// the per-tenant context the runner already resolved scopes the group scan to
// that tenant's database. The pass never fails the cycle, and the use case is the
// single point that reports its result.
func (r *RedisQueueConsumer) reconcileTransactionGroups(ctx context.Context) {
	if r.Command == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, transactionGroupReconcileTimeout)
	defer cancel()

	r.Command.ReconcileTransactionGroups(ctx)
}
