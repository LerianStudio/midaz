// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// fakeLeaseHandle is a renewable lock handle whose Extend answers are scripted.
type fakeLeaseHandle struct {
	extendRenewed bool
	extendErr     error
	extends       int
	unlocks       int
	unlockErr     error
	unlockCtxErr  error
}

func (h *fakeLeaseHandle) Unlock(ctx context.Context) error {
	h.unlocks++
	h.unlockCtxErr = ctx.Err()

	return h.unlockErr
}

func (h *fakeLeaseHandle) Extend(context.Context) (bool, error) {
	h.extends++

	return h.extendRenewed, h.extendErr
}

// unextendableHandle is a lock handle without the renewal capability.
type unextendableHandle struct{ unlocks int }

func (h *unextendableHandle) Unlock(context.Context) error {
	h.unlocks++
	return nil
}

// fakeLockManager hands out the scripted handles in order and records each call.
type fakeLockManager struct {
	handles  []libRedis.LockHandle
	busy     bool
	err      error
	attempts int
	keys     []string
	opts     []libRedis.LockOptions
}

func (m *fakeLockManager) TryLockWithOptions(_ context.Context, key string, opts libRedis.LockOptions) (libRedis.LockHandle, bool, error) {
	m.attempts++
	m.keys = append(m.keys, key)
	m.opts = append(m.opts, opts)

	if m.err != nil {
		return nil, false, m.err
	}

	if m.busy || len(m.handles) == 0 {
		return nil, false, nil
	}

	handle := m.handles[0]
	m.handles = m.handles[1:]

	return handle, true, nil
}

func TestReconcilerLeaseFirstHoldAcquiresFresh(t *testing.T) {
	handle := &fakeLeaseHandle{extendRenewed: true}
	manager := &fakeLockManager{handles: []libRedis.LockHandle{handle}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	held, fresh, err := lease.Hold(t.Context())

	require.NoError(t, err)
	assert.True(t, held)
	assert.True(t, fresh)
	assert.Equal(t, []string{utils.BalanceReconcilerLockKey()}, manager.keys)
	assert.Equal(t, 1, manager.opts[0].Tries, "a busy lease is never waited for")
	assert.Zero(t, handle.extends)
}

func TestReconcilerLeaseSecondHoldExtends(t *testing.T) {
	handle := &fakeLeaseHandle{extendRenewed: true}
	manager := &fakeLockManager{handles: []libRedis.LockHandle{handle}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	held, fresh, err := lease.Hold(t.Context())

	require.NoError(t, err)
	assert.True(t, held)
	assert.False(t, fresh, "a renewed lease is the same tenure")
	assert.Equal(t, 1, handle.extends)
	assert.Equal(t, 1, manager.attempts)
}

func TestReconcilerLeaseLostLeaseIsReacquiredFresh(t *testing.T) {
	lost := &fakeLeaseHandle{extendRenewed: false}
	next := &fakeLeaseHandle{extendRenewed: true}
	manager := &fakeLockManager{handles: []libRedis.LockHandle{lost, next}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	held, fresh, err := lease.Hold(t.Context())

	require.NoError(t, err)
	assert.True(t, held)
	assert.True(t, fresh)
	assert.Equal(t, 2, manager.attempts)

	_, _, err = lease.Hold(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, lost.extends, "the lost handle is dropped")
	assert.Equal(t, 1, next.extends)
}

func TestReconcilerLeaseLostLeaseTakenElsewhereIsNotHeld(t *testing.T) {
	lost := &fakeLeaseHandle{extendRenewed: false}
	manager := &fakeLockManager{handles: []libRedis.LockHandle{lost}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	manager.busy = true

	held, fresh, err := lease.Hold(t.Context())

	require.NoError(t, err)
	assert.False(t, held)
	assert.False(t, fresh)
}

func TestReconcilerLeaseExtendErrorKeepsTheHandle(t *testing.T) {
	handle := &fakeLeaseHandle{extendErr: errors.New("redis did not answer")}
	manager := &fakeLockManager{handles: []libRedis.LockHandle{handle}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	held, fresh, err := lease.Hold(t.Context())

	require.ErrorIs(t, err, handle.extendErr)
	assert.False(t, held)
	assert.False(t, fresh)

	handle.extendErr = nil
	handle.extendRenewed = true

	held, fresh, err = lease.Hold(t.Context())

	require.NoError(t, err)
	assert.True(t, held)
	assert.False(t, fresh, "the lease that survived the fault is the same tenure")
	assert.Equal(t, 2, handle.extends)
	assert.Equal(t, 1, manager.attempts, "a kept handle is extended, never re-acquired")
}

func TestReconcilerLeaseBusyAndErrorAreNotHeld(t *testing.T) {
	busy := newRedisReconcilerLease(&fakeLockManager{busy: true}, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	held, fresh, err := busy.Hold(t.Context())
	require.NoError(t, err)
	assert.False(t, held)
	assert.False(t, fresh)

	failing := &fakeLockManager{err: errors.New("redis unavailable")}
	broken := newRedisReconcilerLease(failing, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	held, fresh, err = broken.Hold(t.Context())
	require.ErrorIs(t, err, failing.err)
	assert.False(t, held)
	assert.False(t, fresh)
}

func TestReconcilerLeaseRejectsAHandleThatCannotBeExtended(t *testing.T) {
	handle := &unextendableHandle{}
	lease := newRedisReconcilerLease(&fakeLockManager{handles: []libRedis.LockHandle{handle}}, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	held, _, err := lease.Hold(t.Context())

	require.ErrorIs(t, err, errReconcilerLeaseNotExtendable)
	assert.False(t, held)
	assert.Equal(t, 1, handle.unlocks, "an unusable handle is released at once")
}

func TestReconcilerLeaseReleaseUnlocksOnce(t *testing.T) {
	handle := &fakeLeaseHandle{extendRenewed: true}
	lease := newRedisReconcilerLease(&fakeLockManager{handles: []libRedis.LockHandle{handle}}, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	lease.Release(ctx)
	lease.Release(ctx)

	assert.Equal(t, 1, handle.unlocks)
	assert.NoError(t, handle.unlockCtxErr, "the release outlives the cancelled shutdown context")
}

func TestReconcilerLeaseReleaseWithoutLeadershipIsANoOp(t *testing.T) {
	lease := newRedisReconcilerLease(&fakeLockManager{busy: true}, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	require.NotPanics(t, func() { lease.Release(t.Context()) })
}

func TestReconcilerLeaseReleaseFailureIsOnlyLogged(t *testing.T) {
	handle := &fakeLeaseHandle{unlockErr: errors.New("lock not held")}
	lease := newRedisReconcilerLease(&fakeLockManager{handles: []libRedis.LockHandle{handle}}, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)

	lease.Release(t.Context())

	assert.Equal(t, 1, handle.unlocks)
}

func TestReconcilerLeaseExpiryFollowsTheConfiguration(t *testing.T) {
	cfg := BalanceReconcilerConfig{IntervalMs: 900000, PassBudgetMs: 60000}
	expiry := balanceReconcilerLeaseExpiry(cfg)

	// 15 min interval + 60 s budget + 30 s page timeout + 15 s margin.
	assert.Equal(t, 15*time.Minute+time.Minute+30*time.Second+15*time.Second, expiry)

	manager := &fakeLockManager{handles: []libRedis.LockHandle{&fakeLeaseHandle{}}}
	lease := newRedisReconcilerLease(manager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), expiry)

	_, _, err := lease.Hold(t.Context())
	require.NoError(t, err)
	require.Len(t, manager.opts, 1)
	assert.Equal(t, expiry, manager.opts[0].Expiry)
}

func TestReconcilerLeaseLocksTheKeyItWasBuiltWith(t *testing.T) {
	reconcilerManager := &fakeLockManager{handles: []libRedis.LockHandle{&fakeLeaseHandle{}}}
	probeManager := &fakeLockManager{handles: []libRedis.LockHandle{&fakeLeaseHandle{}}}

	reconciler := newRedisReconcilerLease(reconcilerManager, libLog.NewNop(), utils.BalanceReconcilerLockKey(), time.Minute)
	probe := newRedisReconcilerLease(probeManager, libLog.NewNop(), utils.TenantHealthProbeLockKey(), time.Minute)

	_, _, err := reconciler.Hold(t.Context())
	require.NoError(t, err)

	_, _, err = probe.Hold(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{utils.BalanceReconcilerLockKey()}, reconcilerManager.keys)
	assert.Equal(t, []string{utils.TenantHealthProbeLockKey()}, probeManager.keys)
	assert.NotEqual(t, utils.BalanceReconcilerLockKey(), utils.TenantHealthProbeLockKey())
}
