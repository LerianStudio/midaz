// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGo_ExecutesFunction(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	var executed atomic.Bool

	var wg sync.WaitGroup

	wg.Add(1)

	Go(logger, func() {
		defer wg.Done()
		executed.Store(true)
	})

	wg.Wait()
	assert.True(t, executed.Load(), "function should have been executed")
}

func TestGo_RecoversPanic(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	done := make(chan struct{})

	Go(logger, func() {
		defer close(done)
		panic("test panic in Go")
	})

	select {
	case <-done:
		// Goroutine completed (panic was recovered)
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete - panic may not have been recovered")
	}
}

func TestGo_RecoversPanicWithoutCrashing(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	// Verify that a panicking goroutine does not crash the process
	done := make(chan struct{})

	Go(logger, func() {
		defer close(done)
		panic("should be recovered")
	})

	select {
	case <-done:
		// Success - panic was recovered and goroutine completed
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for goroutine to complete")
	}
}

func TestGoWithCleanup_ExecutesFunction(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	var executed atomic.Bool

	var wg sync.WaitGroup

	wg.Add(1)

	GoWithCleanup(logger, func() {
		defer wg.Done()
		executed.Store(true)
	}, func(_ any) {
		// cleanup should not be called for normal execution
	})

	wg.Wait()
	assert.True(t, executed.Load(), "function should have been executed")
}

func TestGoWithCleanup_RecoversPanicAndCallsCleanup(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	var cleanupCalled atomic.Bool

	var recoveredValue atomic.Value

	done := make(chan struct{})

	GoWithCleanup(logger, func() {
		panic("test cleanup panic")
	}, func(recovered any) {
		cleanupCalled.Store(true)
		recoveredValue.Store(recovered)
		close(done)
	})

	select {
	case <-done:
		assert.True(t, cleanupCalled.Load(), "cleanup function should have been called")
		assert.Equal(t, "test cleanup panic", recoveredValue.Load(), "cleanup should receive the panic value")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cleanup to complete")
	}
}

func TestGoWithCleanup_NilCleanupDoesNotPanic(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	done := make(chan struct{})

	GoWithCleanup(logger, func() {
		defer close(done)
		panic("test nil cleanup")
	}, nil)

	select {
	case <-done:
		// Success - nil cleanup did not cause a secondary panic
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for goroutine to complete with nil cleanup")
	}
}

func TestGoWithCleanup_CleanupNotCalledOnSuccess(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	var cleanupCalled atomic.Bool

	var wg sync.WaitGroup

	wg.Add(1)

	GoWithCleanup(logger, func() {
		defer wg.Done()
		// Normal execution, no panic
	}, func(_ any) {
		cleanupCalled.Store(true)
	})

	wg.Wait()

	// Give a small window for any spurious cleanup call
	time.Sleep(50 * time.Millisecond)

	assert.False(t, cleanupCalled.Load(), "cleanup should not be called when function succeeds")
}

func TestGoNamed_ExecutesFunction(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	var executed atomic.Bool

	var wg sync.WaitGroup

	wg.Add(1)

	GoNamed(logger, "test-goroutine", func() {
		defer wg.Done()
		executed.Store(true)
	})

	wg.Wait()
	assert.True(t, executed.Load(), "named goroutine function should have been executed")
}

func TestGoNamed_RecoversPanic(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	done := make(chan struct{})

	GoNamed(logger, "panicking-goroutine", func() {
		defer close(done)
		panic("named goroutine panic")
	})

	select {
	case <-done:
		// Success - panic was recovered
	case <-time.After(2 * time.Second):
		t.Fatal("named goroutine did not complete - panic may not have been recovered")
	}
}

func TestGoNamed_MultipleNamedGoroutines(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	const count = 5

	results := make([]atomic.Bool, count)

	var wg sync.WaitGroup

	wg.Add(count)

	for i := 0; i < count; i++ {
		idx := i
		GoNamed(logger, "worker", func() {
			defer wg.Done()
			results[idx].Store(true)
		})
	}

	wg.Wait()

	for i := 0; i < count; i++ {
		require.True(t, results[i].Load(), "goroutine %d should have completed", i)
	}
}
