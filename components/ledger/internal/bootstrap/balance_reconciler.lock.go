// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// errReconcilerLeaseNotExtendable reports a lock handle without the renewal
// capability the lease needs to stay with one leader across passes.
var errReconcilerLeaseNotExtendable = errors.New("balance reconciler lease: lock handle cannot be extended")

// reconcilerLease keeps one reconciler leader across passes.
type reconcilerLease interface {
	// Hold keeps or takes leadership for one pass. fresh reports leadership
	// newly taken by this call: scan state from an earlier tenure may be stale.
	Hold(ctx context.Context) (held, fresh bool, err error)
	// Release gives leadership up; called once on shutdown.
	Release(ctx context.Context)
}

// reconcilerLockManager is the distributed-lock surface the lease uses.
// *libRedis.RedisLockManager satisfies it.
type reconcilerLockManager interface {
	TryLockWithOptions(ctx context.Context, lockKey string, opts libRedis.LockOptions) (libRedis.LockHandle, bool, error)
}

// reconcilerLeaseHandle is an acquired lock that can be renewed.
type reconcilerLeaseHandle interface {
	libRedis.LockHandle
	libRedis.LockExtender
}

// redisReconcilerLease is a reconcilerLease over a Redis distributed lock. The
// leader renews its lock at the start of every pass, so the same pod keeps the
// in-memory scan cursors until it stops or loses the lock. It is used only by
// the runner goroutine and holds no mutex.
type redisReconcilerLease struct {
	manager reconcilerLockManager
	logger  libLog.Logger
	expiry  time.Duration
	handle  reconcilerLeaseHandle
}

// newRedisReconcilerLease builds a lease whose lock lives for expiry after each
// acquisition or renewal.
func newRedisReconcilerLease(manager reconcilerLockManager, logger libLog.Logger, expiry time.Duration) reconcilerLease {
	return &redisReconcilerLease{manager: manager, logger: logger, expiry: expiry}
}

// balanceReconcilerLeaseExpiry is how long the leader lock survives without a
// renewal: one interval until the next pass renews it, plus that pass's budget,
// the page still in progress when the budget ends, and a margin for tenant
// resolution and clock skew between pods.
func balanceReconcilerLeaseExpiry(cfg BalanceReconcilerConfig) time.Duration {
	return time.Duration(cfg.IntervalMs)*time.Millisecond +
		time.Duration(cfg.PassBudgetMs)*time.Millisecond +
		balanceReconcilerPageTimeout +
		balanceReconcilerLockMargin
}

// Hold renews the lease this pod already holds, or tries once to take it. A
// renewal that fails with an error keeps the handle: the lease may still be ours
// and the next pass renews it again. A lease found lost is dropped and taken anew
// when free.
func (l *redisReconcilerLease) Hold(ctx context.Context) (bool, bool, error) {
	if l.handle != nil {
		renewed, err := l.handle.Extend(ctx)
		if err != nil {
			return false, false, err
		}

		if renewed {
			return true, false, nil
		}

		l.handle = nil
	}

	opts := libRedis.DefaultLockOptions()
	opts.Expiry = l.expiry
	opts.Tries = 1

	handle, acquired, err := l.manager.TryLockWithOptions(ctx, utils.BalanceReconcilerLockKey(), opts)
	if err != nil {
		return false, false, err
	}

	if !acquired {
		return false, false, nil
	}

	leaseHandle, ok := handle.(reconcilerLeaseHandle)
	if !ok {
		l.unlock(ctx, handle)
		return false, false, errReconcilerLeaseNotExtendable
	}

	l.handle = leaseHandle

	return true, true, nil
}

// Release unlocks the lease when this pod holds it. A failed unlock is logged;
// the expiry bounds how long the stale lock survives.
func (l *redisReconcilerLease) Release(ctx context.Context) {
	if l.handle == nil {
		return
	}

	l.unlock(ctx, l.handle)
	l.handle = nil
}

func (l *redisReconcilerLease) unlock(ctx context.Context, handle libRedis.LockHandle) {
	releaseCtx := context.WithoutCancel(ctx)
	if err := handle.Unlock(releaseCtx); err != nil {
		l.logger.Log(releaseCtx, libLog.LevelWarn, "Failed to release balance reconciler lease", libLog.Err(err))
	}
}
