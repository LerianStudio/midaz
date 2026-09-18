// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
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
func (r *RedisQueueConsumer) reconcileAccountClosings(ctx context.Context) {
	if r.Command == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, accountClosingReconcileTimeout)
	defer cancel()

	stats := r.Command.ReconcileAccountClosings(ctx)

	if stats.Scanned == 0 && stats.Ownerships == 0 {
		return
	}

	r.Logger.Log(ctx, libLog.LevelDebug, "Reconciled the account closing protection",
		libLog.Int("scanned_count", stats.Scanned),
		libLog.Int("completed_count", stats.Completed),
		libLog.Int("released_count", stats.Released),
		libLog.Int("retained_count", stats.Retained),
		libLog.Int("unreadable_count", stats.Unreadable),
		libLog.Int("ownership_backlog_count", stats.Ownerships))

	if stats.Retained == 0 && stats.Unreadable == 0 {
		return
	}

	// A retained closing is protection that outlived its attempt, so it is the
	// backlog an operator has to see; no account is named, because the reason is
	// what identifies the condition.
	r.Logger.Log(ctx, libLog.LevelWarn, "Account closing protection is waiting for reconciliation",
		libLog.Int("retained_count", stats.Retained),
		libLog.Int("unreadable_count", stats.Unreadable))
}
