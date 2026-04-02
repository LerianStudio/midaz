// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

// flushedBatch records a single flush invocation for assertion.
type flushedBatch struct {
	keys []string
	at   time.Time
}

// flushRecorder is a concurrency-safe recorder for flush calls.
type flushRecorder struct {
	mu      sync.Mutex
	batches []flushedBatch
}

func (r *flushRecorder) record(_ context.Context, keys []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]string, len(keys))
	copy(cp, keys)

	r.batches = append(r.batches, flushedBatch{keys: cp, at: time.Now()})

	return true
}

func (r *flushRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.batches)
}

func (r *flushRecorder) allKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []string
	for _, b := range r.batches {
		all = append(all, b.keys...)
	}

	return all
}

func (r *flushRecorder) getBatches() []flushedBatch {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]flushedBatch, len(r.batches))
	copy(cp, r.batches)

	return cp
}

// fetchFunc builds a FetchKeysFunc that reads from a channel of key-slices.
// When the channel is empty it blocks until a value arrives or the context is cancelled.
// On context cancellation it returns nil, ctx.Err().
func fetchFunc(ch <-chan []string) FetchKeysFunc {
	return func(ctx context.Context, _ int64) ([]string, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case keys, ok := <-ch:
			if !ok {
				// Channel closed — return empty from now on
				return nil, nil
			}

			return keys, nil
		}
	}
}

// fetchFuncImmediate returns a FetchKeysFunc that pulls the next value
// from a channel without blocking (returns empty slice if nothing ready).
func fetchFuncImmediate(ch <-chan []string) FetchKeysFunc {
	return func(ctx context.Context, _ int64) ([]string, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		select {
		case keys := <-ch:
			return keys, nil
		default:
			return nil, nil
		}
	}
}

// waitForNextShutdown returns a WaitForNextFunc that always returns true (shutdown).
func waitForNextShutdown() WaitForNextFunc {
	return func(_ context.Context) bool {
		return true
	}
}

// waitForNextResume returns a WaitForNextFunc that blocks on a channel.
// Send to the channel to unblock (returns false = continue).
// If ctx is cancelled it returns true.
func waitForNextResume(ch <-chan struct{}) WaitForNextFunc {
	return func(ctx context.Context) bool {
		select {
		case <-ctx.Done():
			return true
		case <-ch:
			return false
		}
	}
}

// nopLogger returns a no-op logger for tests.
func nopLogger() libLog.Logger {
	return libLog.NewNop()
}

// --------------------------------------------------------------------
// Constructor defaults
// --------------------------------------------------------------------

func TestNewBalanceSyncCollector_Defaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		batchSize        int
		flushTimeout     time.Duration
		pollInterval     time.Duration
		idleBackoff      time.Duration
		wantBatchSize    int
		wantFlushTimeout time.Duration
		wantPollInterval time.Duration
		wantIdleBackoff  time.Duration
	}{
		{
			name:             "all zero values get safe defaults",
			batchSize:        0,
			flushTimeout:     0,
			pollInterval:     0,
			idleBackoff:      0,
			wantBatchSize:    50,
			wantFlushTimeout: 500 * time.Millisecond,
			wantPollInterval: 50 * time.Millisecond,
			wantIdleBackoff:  10 * time.Second,
		},
		{
			name:             "all negative values get safe defaults",
			batchSize:        -1,
			flushTimeout:     -1,
			pollInterval:     -1,
			idleBackoff:      -1,
			wantBatchSize:    50,
			wantFlushTimeout: 500 * time.Millisecond,
			wantPollInterval: 50 * time.Millisecond,
			wantIdleBackoff:  10 * time.Second,
		},
		{
			name:             "positive values are preserved",
			batchSize:        10,
			flushTimeout:     2 * time.Second,
			pollInterval:     100 * time.Millisecond,
			idleBackoff:      5 * time.Second,
			wantBatchSize:    10,
			wantFlushTimeout: 2 * time.Second,
			wantPollInterval: 100 * time.Millisecond,
			wantIdleBackoff:  5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := NewBalanceSyncCollector(tt.batchSize, tt.flushTimeout, tt.pollInterval, tt.idleBackoff, nopLogger())

			require.NotNil(t, c)
			assert.Equal(t, tt.wantBatchSize, c.batchSize)
			assert.Equal(t, tt.wantFlushTimeout, c.flushTimeout)
			assert.Equal(t, tt.wantPollInterval, c.pollInterval)
			assert.Equal(t, tt.wantIdleBackoff, c.idleBackoff)
			assert.NotNil(t, c.buffer)
			assert.Equal(t, 0, len(c.buffer))
		})
	}
}

func TestNewBalanceSyncCollector_BufferCapacity(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(25, time.Second, time.Second, time.Second, nopLogger())

	assert.Equal(t, 25, cap(c.buffer), "buffer capacity should equal batchSize")
}

// --------------------------------------------------------------------
// SetFlushCallback
// --------------------------------------------------------------------

func TestSetFlushCallback(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(10, time.Second, time.Second, time.Second, nopLogger())

	assert.Nil(t, c.flushFn, "flushFn should be nil before SetFlushCallback")

	called := false
	c.SetFlushCallback(func(_ context.Context, _ []string) bool {
		called = true
		return true
	})

	require.NotNil(t, c.flushFn)

	c.flushFn(context.Background(), []string{"k"})
	assert.True(t, called)
}

// --------------------------------------------------------------------
// Size
// --------------------------------------------------------------------

func TestSize_ReflectsBuffer(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(10, time.Second, time.Second, time.Second, nopLogger())

	assert.Equal(t, 0, c.Size())

	c.mu.Lock()
	c.buffer = append(c.buffer, "a", "b", "c")
	c.mu.Unlock()

	assert.Equal(t, 3, c.Size())
}

// --------------------------------------------------------------------
// flush (internal)
// --------------------------------------------------------------------

func TestFlush_EmptyBuffer(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(10, time.Second, time.Second, time.Second, nopLogger())
	c.SetFlushCallback(rec.record)

	c.flush(context.Background())

	assert.Equal(t, 0, rec.count(), "should not call flushFn when buffer is empty")
}

func TestFlush_WithItems(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(10, time.Second, time.Second, time.Second, nopLogger())
	c.SetFlushCallback(rec.record)

	c.mu.Lock()
	c.buffer = append(c.buffer, "key1", "key2")
	c.mu.Unlock()

	c.flush(context.Background())

	assert.Equal(t, 1, rec.count())
	assert.Equal(t, []string{"key1", "key2"}, rec.allKeys())
	assert.Equal(t, 0, c.Size(), "buffer should be empty after flush")
}

func TestFlush_NilCallback(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(10, time.Second, time.Second, time.Second, nopLogger())
	// Deliberately do NOT set a callback.

	c.mu.Lock()
	c.buffer = append(c.buffer, "k")
	c.mu.Unlock()

	// Should not panic.
	c.flush(context.Background())

	assert.Equal(t, 0, c.Size(), "buffer should be drained even without callback")
}

// --------------------------------------------------------------------
// waitOrShutdown (package-level helper)
// --------------------------------------------------------------------

func TestWaitOrShutdown_TimerExpires(t *testing.T) {
	t.Parallel()

	result := waitOrShutdown(context.Background(), 1*time.Millisecond)

	assert.False(t, result, "should return false when timer fires")
}

func TestWaitOrShutdown_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := waitOrShutdown(ctx, time.Hour)

	assert.True(t, result, "should return true when context is cancelled")
}

// --------------------------------------------------------------------
// Run — Size trigger
// --------------------------------------------------------------------

func TestRun_SizeTrigger_FlushesWhenBatchFull(t *testing.T) {
	t.Parallel()

	const batchSize = 3
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		10*time.Second, // large timeout so only size triggers
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)

	// Feed exactly batchSize keys in one shot
	keyCh <- []string{"a", "b", "c"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	// Wait for the flush to happen
	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected size-triggered flush")

	cancel()
	<-done

	batches := rec.getBatches()
	require.GreaterOrEqual(t, len(batches), 1)
	assert.Equal(t, []string{"a", "b", "c"}, batches[0].keys)
}

func TestRun_SizeTrigger_AccumulatesAcrossPolls(t *testing.T) {
	t.Parallel()

	const batchSize = 4
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		10*time.Second, // large timeout
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)

	// Feed keys in two separate polls that together reach batchSize
	keyCh <- []string{"a", "b"}
	keyCh <- []string{"c", "d"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected size-triggered flush after accumulation")

	cancel()
	<-done

	allKeys := rec.allKeys()
	assert.Contains(t, allKeys, "a")
	assert.Contains(t, allKeys, "b")
	assert.Contains(t, allKeys, "c")
	assert.Contains(t, allKeys, "d")
}

// --------------------------------------------------------------------
// Run — Timeout trigger
// --------------------------------------------------------------------

func TestRun_TimeoutTrigger_FlushesPartialBatch(t *testing.T) {
	t.Parallel()

	const batchSize = 100 // large so size trigger never fires
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		200*time.Millisecond, // short timeout to trigger quickly
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)

	// Feed a partial batch, then nothing — timeout should flush
	keyCh <- []string{"x", "y"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected timeout-triggered flush")

	cancel()
	<-done

	assert.Equal(t, []string{"x", "y"}, rec.getBatches()[0].keys)
}

// --------------------------------------------------------------------
// Run — Empty buffer does NOT flush on timeout
// --------------------------------------------------------------------

func TestRun_EmptyBuffer_NoFlushOnTimeout(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		10,
		100*time.Millisecond,
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// fetchKeys always returns empty — collector enters idle mode,
	// waitForNext sees cancellation.
	emptyFetch := func(_ context.Context, _ int64) ([]string, error) {
		return nil, nil
	}

	c.Run(ctx, emptyFetch, func(ctx context.Context) bool {
		// Block until context is cancelled
		<-ctx.Done()
		return true
	})

	assert.Equal(t, 0, rec.count(), "should never flush when buffer is always empty")
}

// --------------------------------------------------------------------
// Run — Idle mode enters waitForNext
// --------------------------------------------------------------------

func TestRun_IdleMode_CallsWaitForNext(t *testing.T) {
	t.Parallel()

	var waitCalled atomic.Int32

	c := NewBalanceSyncCollector(10, time.Second, 50*time.Millisecond, 10*time.Second, nopLogger())
	c.SetFlushCallback(func(_ context.Context, _ []string) bool { return true })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emptyFetch := func(ctx context.Context, _ int64) ([]string, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, nil
	}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, emptyFetch, func(ctx context.Context) bool {
			waitCalled.Add(1)
			// First call: return false (continue loop) to verify re-entry
			if waitCalled.Load() <= 1 {
				return false
			}
			// Second call: signal shutdown
			return true
		})
		close(done)
	}()

	<-done

	assert.GreaterOrEqual(t, int(waitCalled.Load()), 2, "waitForNext should be called at least twice")
}

// --------------------------------------------------------------------
// Run — Context cancellation → final flush
// --------------------------------------------------------------------

func TestRun_ContextCancellation_FinalFlush(t *testing.T) {
	t.Parallel()

	const batchSize = 100 // large so size trigger doesn't fire
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		10*time.Second, // long timeout
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())

	keyCh := make(chan []string, 10)

	// Feed keys that won't fill the batch
	keyCh <- []string{"final1", "final2"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	// Wait for the keys to be consumed into the buffer
	require.Eventually(t, func() bool {
		return c.Size() >= 2
	}, 3*time.Second, 10*time.Millisecond, "keys should be accumulated in buffer")

	// Now cancel — the deferred final flush should fire
	cancel()
	<-done

	allKeys := rec.allKeys()
	assert.Contains(t, allKeys, "final1")
	assert.Contains(t, allKeys, "final2")
}

// --------------------------------------------------------------------
// Run — Error handling: transient fetch errors don't kill the loop
// --------------------------------------------------------------------

func TestRun_TransientFetchError_ContinuesLoop(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		3,
		10*time.Second,
		10*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount atomic.Int32

	fetchFn := func(ctx context.Context, limit int64) ([]string, error) {
		n := callCount.Add(1)
		switch {
		case n <= 2:
			// First two calls: transient error
			return nil, errors.New("redis timeout")
		case n == 3:
			// Third call: success
			return []string{"a", "b", "c"}, nil
		default:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			return nil, nil
		}
	}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFn, waitForNextShutdown())
		close(done)
	}()

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 5*time.Second, 10*time.Millisecond, "should flush despite earlier transient errors")

	cancel()
	<-done

	assert.Equal(t, []string{"a", "b", "c"}, rec.getBatches()[0].keys)
	assert.GreaterOrEqual(t, int(callCount.Load()), 3, "fetch should have been called at least 3 times (2 errors + 1 success)")
}

// --------------------------------------------------------------------
// Run — Busy mode: continuous key flow
// --------------------------------------------------------------------

func TestRun_BusyMode_MultipleBatches(t *testing.T) {
	t.Parallel()

	const batchSize = 2
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		10*time.Second,
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)

	// Feed multiple full batches back-to-back
	keyCh <- []string{"a1", "a2"}
	keyCh <- []string{"b1", "b2"}
	keyCh <- []string{"c1", "c2"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	require.Eventually(t, func() bool {
		return rec.count() >= 3
	}, 5*time.Second, 10*time.Millisecond, "expected 3 size-triggered flushes")

	cancel()
	<-done

	allKeys := rec.allKeys()
	assert.Len(t, allKeys, 6)
}

// --------------------------------------------------------------------
// Run — Mixed trigger: partial batch flushed by timeout,
// then new keys arrive and fill a batch by size.
// --------------------------------------------------------------------

func TestRun_MixedTrigger_TimeoutThenSize(t *testing.T) {
	t.Parallel()

	const batchSize = 3
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		200*time.Millisecond, // short timeout
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)
	waitCh := make(chan struct{}, 5)

	// Phase 1: partial batch — should trigger timeout flush
	keyCh <- []string{"p1"}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextResume(waitCh))
		close(done)
	}()

	// Wait for the timeout-triggered flush of the partial batch
	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected timeout-triggered flush of partial batch")

	// Phase 2: full batch — load keys then resume from idle
	keyCh <- []string{"s1", "s2", "s3"}
	waitCh <- struct{}{}

	require.Eventually(t, func() bool {
		return rec.count() >= 2
	}, 3*time.Second, 10*time.Millisecond, "expected size-triggered flush of full batch")

	cancel()
	<-done

	batches := rec.getBatches()
	require.GreaterOrEqual(t, len(batches), 2)
	assert.Equal(t, []string{"p1"}, batches[0].keys, "first flush should be the partial batch")
	assert.Equal(t, []string{"s1", "s2", "s3"}, batches[1].keys, "second flush should be the full batch")
}

// --------------------------------------------------------------------
// Run — waitForNext resumes polling after idle
// --------------------------------------------------------------------

func TestRun_IdleResumeAfterWaitForNext(t *testing.T) {
	t.Parallel()

	const batchSize = 2
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		200*time.Millisecond,
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)
	waitCh := make(chan struct{}, 5)

	var fetchCallCount atomic.Int32

	fetchFn := func(ctx context.Context, limit int64) ([]string, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		n := fetchCallCount.Add(1)
		if n == 1 {
			// First round: nothing — will enter idle
			return nil, nil
		}

		// After resume: check channel for keys
		select {
		case keys := <-keyCh:
			return keys, nil
		default:
			return nil, nil
		}
	}

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFn, waitForNextResume(waitCh))
		close(done)
	}()

	// Wait for collector to reach idle and call waitForNext
	time.Sleep(200 * time.Millisecond)

	// Now push keys and resume from idle
	keyCh <- []string{"k1", "k2"}
	waitCh <- struct{}{}

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "should flush after resuming from idle")

	cancel()
	<-done

	assert.Contains(t, rec.allKeys(), "k1")
	assert.Contains(t, rec.allKeys(), "k2")
}

// --------------------------------------------------------------------
// Run — No flushFn set: Run should not panic
// --------------------------------------------------------------------

func TestRun_NilFlushCallback_NoPanic(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(2, 100*time.Millisecond, 50*time.Millisecond, 10*time.Second, nopLogger())
	// Deliberately do NOT call SetFlushCallback.

	ctx, cancel := context.WithCancel(context.Background())

	keyCh := make(chan []string, 10)
	keyCh <- []string{"a", "b"} // will trigger size flush with nil callback

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	// Let it run a bit to hit the flush path
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	// No panic = test passed.
}

// --------------------------------------------------------------------
// Run — Context cancelled before first iteration
// --------------------------------------------------------------------

func TestRun_ImmediateCancel_ExitsCleanly(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(10, time.Second, 50*time.Millisecond, 10*time.Second, nopLogger())
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Pre-load buffer to verify final flush
	c.mu.Lock()
	c.buffer = append(c.buffer, "leftover")
	c.mu.Unlock()

	c.Run(ctx, func(_ context.Context, _ int64) ([]string, error) {
		return nil, nil
	}, waitForNextShutdown())

	// Final flush should have drained the buffer
	assert.Equal(t, 1, rec.count(), "final flush should fire for pre-loaded buffer")
	assert.Equal(t, []string{"leftover"}, rec.allKeys())
}

// --------------------------------------------------------------------
// Run — Final flush on shutdown with empty buffer: no flush
// --------------------------------------------------------------------

func TestRun_FinalFlush_EmptyBuffer_NoFlush(t *testing.T) {
	t.Parallel()

	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(10, time.Second, 50*time.Millisecond, 10*time.Second, nopLogger())
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c.Run(ctx, func(_ context.Context, _ int64) ([]string, error) {
		return nil, nil
	}, waitForNextShutdown())

	assert.Equal(t, 0, rec.count(), "no flush when buffer is empty at shutdown")
}

// --------------------------------------------------------------------
// Run — Timer reset: first keys in empty buffer reset the timer
// --------------------------------------------------------------------

func TestRun_TimerResetOnFirstKeys(t *testing.T) {
	t.Parallel()

	const batchSize = 100 // large so size never triggers
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		300*time.Millisecond,
		50*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 10)
	waitCh := make(chan struct{}, 5)

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextResume(waitCh))
		close(done)
	}()

	// The collector starts, polls empty, enters idle, blocks on waitForNext.
	// Delay before sending keys — the flush should happen ~300ms after
	// the keys arrive, not ~300ms after Run started.
	time.Sleep(200 * time.Millisecond)
	keyCh <- []string{"late-key"}
	waitCh <- struct{}{} // resume from idle
	sendTime := time.Now()

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected timeout flush")

	batches := rec.getBatches()
	flushDelay := batches[0].at.Sub(sendTime)

	// The flush should happen approximately flushTimeout after the keys arrived.
	// Allow generous tolerance for CI environments.
	assert.Greater(t, flushDelay, 100*time.Millisecond, "flush should not happen immediately")
	assert.Less(t, flushDelay, 2*time.Second, "flush should happen within reasonable timeout window")

	cancel()
	<-done
}

// --------------------------------------------------------------------
// Run — Concurrent safety: multiple goroutines checking Size()
// --------------------------------------------------------------------

func TestRun_ConcurrentSizeAccess(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(100, 200*time.Millisecond, 20*time.Millisecond, 10*time.Second, nopLogger())
	c.SetFlushCallback(func(_ context.Context, _ []string) bool { return true })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 100)

	// Feed keys continuously
	go func() {
		for i := 0; i < 50; i++ {
			select {
			case keyCh <- []string{"k"}:
			case <-ctx.Done():
				return
			}

			time.Sleep(5 * time.Millisecond)
		}
	}()

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextShutdown())
		close(done)
	}()

	// Concurrently read Size() while Run is active
	var wg sync.WaitGroup

	for range 5 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 20 {
				_ = c.Size()
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	cancel()
	<-done
	// No race condition = test passed.
}

// --------------------------------------------------------------------
// Run — fetchKeys blocking: uses channel-based fetchFunc
// (verifies that a blocking fetch properly yields on ctx cancellation)
// --------------------------------------------------------------------

func TestRun_BlockingFetch_ContextCancellation(t *testing.T) {
	t.Parallel()

	c := NewBalanceSyncCollector(10, time.Second, 50*time.Millisecond, 10*time.Second, nopLogger())
	c.SetFlushCallback(func(_ context.Context, _ []string) bool { return true })

	ctx, cancel := context.WithCancel(context.Background())

	// fetchFunc blocks waiting on channel
	keyCh := make(chan []string)

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFunc(keyCh), waitForNextShutdown())
		close(done)
	}()

	// Cancel while fetch is blocked
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Clean exit
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit within timeout after context cancellation")
	}
}

// --------------------------------------------------------------------
// Run — Multiple flush cycles
// --------------------------------------------------------------------

func TestRun_MultipleFlushCycles(t *testing.T) {
	t.Parallel()

	const batchSize = 2
	rec := &flushRecorder{}

	c := NewBalanceSyncCollector(
		batchSize,
		150*time.Millisecond,
		20*time.Millisecond,
		10*time.Second,
		nopLogger(),
	)
	c.SetFlushCallback(rec.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyCh := make(chan []string, 20)
	waitCh := make(chan struct{}, 10)

	done := make(chan struct{})

	go func() {
		c.Run(ctx, fetchFuncImmediate(keyCh), waitForNextResume(waitCh))
		close(done)
	}()

	// Cycle 1: size trigger
	keyCh <- []string{"c1a", "c1b"}

	require.Eventually(t, func() bool {
		return rec.count() >= 1
	}, 3*time.Second, 10*time.Millisecond)

	// Cycle 2: timeout trigger (partial batch)
	// Load keys then resume from idle
	keyCh <- []string{"c2a"}
	waitCh <- struct{}{}

	require.Eventually(t, func() bool {
		return rec.count() >= 2
	}, 3*time.Second, 10*time.Millisecond)

	// Cycle 3: size trigger again
	keyCh <- []string{"c3a", "c3b"}
	waitCh <- struct{}{}

	require.Eventually(t, func() bool {
		return rec.count() >= 3
	}, 3*time.Second, 10*time.Millisecond)

	cancel()
	<-done

	batches := rec.getBatches()
	require.GreaterOrEqual(t, len(batches), 3)
	assert.Equal(t, []string{"c1a", "c1b"}, batches[0].keys)
	assert.Equal(t, []string{"c2a"}, batches[1].keys)
	assert.Equal(t, []string{"c3a", "c3b"}, batches[2].keys)
}
