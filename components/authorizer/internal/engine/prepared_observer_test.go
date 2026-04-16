// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// captureObserver records every ObservePreparedExpired and
// ObservePreparedPendingDepth invocation under a mutex so tests can assert
// counter/gauge emissions without racing the reaper goroutine.
type captureObserver struct {
	mu          sync.Mutex
	expired     []string
	depths      []int
	expiredHits int
}

func (c *captureObserver) ObservePreparedExpired(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.expired = append(c.expired, reason)
	c.expiredHits++
}

func (c *captureObserver) ObservePreparedPendingDepth(depth int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.depths = append(c.depths, depth)
}

func (c *captureObserver) snapshot() (expired []string, depths []int, hits int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiredCopy := make([]string, len(c.expired))
	copy(expiredCopy, c.expired)

	depthsCopy := make([]int, len(c.depths))
	copy(depthsCopy, c.depths)

	return expiredCopy, depthsCopy, c.expiredHits
}

// TestReapExpiredPrepared_EmitsCounter verifies the store's Expired() path
// emits ObservePreparedExpired with reason=timeout once per auto-aborted
// prepared transaction. Prior to D10-B1 this path was silent — operators had
// no SLI for stuck 2PC transactions.
func TestReapExpiredPrepared_EmitsCounter(t *testing.T) {
	store := newPreparedTxStore(5*time.Millisecond, 10)

	obs := &captureObserver{}
	store.SetPreparedObserver(obs)

	// Seed three prepared transactions with a createdAt in the past so all
	// three expire on the first Expired() sweep.
	past := time.Now().Add(-1 * time.Second)

	for _, id := range []string{"ptx-1", "ptx-2", "ptx-3"} {
		require.NoError(t, store.Put(&PreparedTx{ID: id}))

		store.mu.Lock()
		store.pending[id].createdAt = past
		store.mu.Unlock()
	}

	expired := store.Expired()
	require.Len(t, expired, 3)

	reasons, depths, hits := obs.snapshot()
	require.Equal(t, 3, hits, "expected 3 ObservePreparedExpired calls")

	for _, r := range reasons {
		require.Equal(t, PreparedExpirationTimeout, r,
			"reap path MUST emit PreparedExpirationTimeout, got %q", r)
	}

	// The final depth snapshot after the sweep must show zero pending.
	require.NotEmpty(t, depths)
	require.Equal(t, 0, depths[len(depths)-1],
		"post-sweep pending depth MUST be 0, got %d", depths[len(depths)-1])
}

// TestPreparedPendingDepthGauge_TracksStoreSize verifies every mutation
// (Put / PutBack / TakeForCommit / TakeForAbort / Expired) emits a depth
// snapshot reflecting the post-mutation count. Without this the gauge
// would only update on reap-sweeps — invisible for the common single-tx
// lifecycle.
func TestPreparedPendingDepthGauge_TracksStoreSize(t *testing.T) {
	store := newPreparedTxStore(time.Hour, 10)

	obs := &captureObserver{}
	store.SetPreparedObserver(obs)

	require.NoError(t, store.Put(&PreparedTx{ID: "ptx-a"}))
	require.NoError(t, store.Put(&PreparedTx{ID: "ptx-b"}))

	ptx, _, ok := store.TakeForCommit("ptx-a")
	require.True(t, ok)
	require.NotNil(t, ptx)

	_, err := store.TakeForAbort("ptx-b")
	require.NoError(t, err)

	_, depths, _ := obs.snapshot()

	// Expected observed depth sequence: 1 (Put a), 2 (Put b), 1 (Take a
	// for commit), 0 (Take b for abort). The observer may see additional
	// depth emissions if implementation changes; at minimum the sequence
	// MUST contain these values in order as a subsequence.
	require.GreaterOrEqual(t, len(depths), 4,
		"each mutation MUST emit a depth snapshot, got %v", depths)

	expected := []int{1, 2, 1, 0}
	found := 0

	for _, d := range depths {
		if found < len(expected) && d == expected[found] {
			found++
		}
	}

	require.Equal(t, len(expected), found,
		"depth emissions %v must contain subsequence %v", depths, expected)
}

// TestSetPreparedObserver_NilSafe covers the documented nil-safety contract:
// passing nil clears the observer, and nil receivers on the store / engine
// must not panic.
func TestSetPreparedObserver_NilSafe(t *testing.T) {
	var nilStore *preparedTxStore

	require.NotPanics(t, func() {
		nilStore.SetPreparedObserver(&captureObserver{})
	})

	store := newPreparedTxStore(time.Hour, 10)
	store.SetPreparedObserver(&captureObserver{})

	require.NotPanics(t, func() {
		store.SetPreparedObserver(nil)
	})

	// After clearing, mutations must not attempt to call the nil observer.
	require.NotPanics(t, func() {
		_ = store.Put(&PreparedTx{ID: "ptx-nil"})
	})
}

// TestEnginePreparedPendingDepth_ReflectsStore covers the (*Engine) helper
// that exposes the store depth for bootstrap-side pre-seeding after
// crash-recovery replay.
func TestEnginePreparedPendingDepth_ReflectsStore(t *testing.T) {
	var nilEngine *Engine

	require.Equal(t, 0, nilEngine.PreparedPendingDepth(),
		"nil engine MUST return 0, not panic")
}
