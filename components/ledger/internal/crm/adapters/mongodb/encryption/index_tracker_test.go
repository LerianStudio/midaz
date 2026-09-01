// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexTracker_EnsureOnce_ExecutesOnceOnSuccess(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32

	for i := 0; i < 3; i++ {
		err := tracker.ensureOnce("test-db:test-collection", func() error {
			atomic.AddInt32(&callCount, 1)
			return nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "function should be called exactly once on success")
}

func TestIndexTracker_EnsureOnce_RetriesOnFailure(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32
	expectedErr := errors.New("index creation failed")

	// First call fails
	err := tracker.ensureOnce("test-db:test-collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return expectedErr
	})
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// Second call should retry (not skipped)
	err = tracker.ensureOnce("test-db:test-collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil // Now succeeds
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "function should be retried after failure")

	// Third call should be skipped (already succeeded)
	err = tracker.ensureOnce("test-db:test-collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "function should not be called again after success")
}

func TestIndexTracker_EnsureOnce_DifferentKeys(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCountA, callCountB int32

	// Key A
	err := tracker.ensureOnce("tenant-a:collection", func() error {
		atomic.AddInt32(&callCountA, 1)
		return nil
	})
	require.NoError(t, err)

	// Key B (different tenant)
	err = tracker.ensureOnce("tenant-b:collection", func() error {
		atomic.AddInt32(&callCountB, 1)
		return nil
	})
	require.NoError(t, err)

	// Key A again (should be skipped)
	err = tracker.ensureOnce("tenant-a:collection", func() error {
		atomic.AddInt32(&callCountA, 1)
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCountA), "tenant A should be called once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCountB), "tenant B should be called once")
}

func TestIndexTracker_EnsureOnce_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			_ = tracker.ensureOnce("concurrent-db:collection", func() error {
				atomic.AddInt32(&callCount, 1)
				return nil
			})
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "function should be called exactly once even with concurrent access")
}

func TestIndexTracker_EnsureOnce_ConcurrentAccessWithFailure(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32
	var failureCount int32
	const goroutines = 50

	// First, make the first call fail
	err := tracker.ensureOnce("concurrent-fail-db:collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return errors.New("initial failure")
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// Now run concurrent retries - only one should succeed in executing
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			err := tracker.ensureOnce("concurrent-fail-db:collection", func() error {
				atomic.AddInt32(&callCount, 1)
				return nil
			})
			if err != nil {
				atomic.AddInt32(&failureCount, 1)
			}
		}()
	}

	wg.Wait()

	// Due to mutex, only one goroutine should have actually executed
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "function should be retried exactly once after initial failure")
	assert.Equal(t, int32(0), atomic.LoadInt32(&failureCount), "all concurrent calls should succeed once retry succeeds")
}

func TestIndexTracker_Reset(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32

	// First call
	err := tracker.ensureOnce("reset-test:collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// Reset the key
	tracker.reset("reset-test:collection")

	// Call again - should execute
	err = tracker.ensureOnce("reset-test:collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "function should be called again after reset")
}

func TestIndexTracker_MultipleFailuresBeforeSuccess(t *testing.T) {
	t.Parallel()

	tracker := &indexTracker{}
	var callCount int32

	// Simulate multiple failures before success
	for i := 0; i < 5; i++ {
		_ = tracker.ensureOnce("multi-fail:collection", func() error {
			atomic.AddInt32(&callCount, 1)
			return errors.New("temporary failure")
		})
	}
	assert.Equal(t, int32(5), atomic.LoadInt32(&callCount), "function should be called on each retry")

	// Now succeed
	err := tracker.ensureOnce("multi-fail:collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(6), atomic.LoadInt32(&callCount))

	// Subsequent calls should be skipped
	err = tracker.ensureOnce("multi-fail:collection", func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(6), atomic.LoadInt32(&callCount), "function should not be called after success")
}
