// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBulkCollector_Defaults(t *testing.T) {
	t.Parallel()

	// Test with zero values - should use defaults
	bc := NewBulkCollector(0, 0)

	assert.Equal(t, 50, bc.BulkSize())
	assert.Equal(t, 100*time.Millisecond, bc.FlushTimeout())
	assert.Equal(t, 0, bc.Size())
}

func TestNewBulkCollector_CustomValues(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(100, 500*time.Millisecond)

	assert.Equal(t, 100, bc.BulkSize())
	assert.Equal(t, 500*time.Millisecond, bc.FlushTimeout())
}

func TestBulkCollector_AddBeforeStart(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)

	msg := amqp.Delivery{Body: []byte("test")}
	err := bc.Add(msg)

	assert.Error(t, err)
	assert.Equal(t, ErrCollectorNotStarted, err)
}

func TestBulkCollector_StartWithoutCallback(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := bc.Start(ctx)

	assert.Error(t, err)
	assert.Equal(t, ErrNoFlushCallback, err)
}

func TestBulkCollector_StartTwice(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start in goroutine
	go func() {
		_ = bc.Start(ctx)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Try to start again
	err := bc.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrCollectorAlreadyStarted, err)
}

func TestBulkCollector_SizeBasedFlush(t *testing.T) {
	t.Parallel()

	bulkSize := 5
	var flushedCount int32
	var flushedMessages []amqp.Delivery
	var mu sync.Mutex

	bc := NewBulkCollector(bulkSize, 5*time.Second) // Long timeout to ensure size triggers
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushedCount, 1)
		mu.Lock()
		flushedMessages = append(flushedMessages, messages...)
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Add exactly bulkSize messages
	for i := 0; i < bulkSize; i++ {
		msg := amqp.Delivery{Body: []byte{byte(i)}}
		err := bc.Add(msg)
		require.NoError(t, err)
	}

	// Wait for flush to complete
	time.Sleep(50 * time.Millisecond)

	// Verify flush happened
	assert.Equal(t, int32(1), atomic.LoadInt32(&flushedCount))
	mu.Lock()
	assert.Len(t, flushedMessages, bulkSize)
	mu.Unlock()
}

func TestBulkCollector_TimeBasedFlush(t *testing.T) {
	t.Parallel()

	bulkSize := 100 // Large size to ensure time triggers first
	flushTimeout := 50 * time.Millisecond
	var flushedCount int32
	var flushedMessages []amqp.Delivery
	var mu sync.Mutex

	bc := NewBulkCollector(bulkSize, flushTimeout)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushedCount, 1)
		mu.Lock()
		flushedMessages = append(flushedMessages, messages...)
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Add fewer messages than bulk size
	for i := 0; i < 3; i++ {
		msg := amqp.Delivery{Body: []byte{byte(i)}}
		err := bc.Add(msg)
		require.NoError(t, err)
	}

	// Wait for timeout flush
	time.Sleep(flushTimeout + 50*time.Millisecond)

	// Verify timeout flush happened
	assert.Equal(t, int32(1), atomic.LoadInt32(&flushedCount))
	mu.Lock()
	assert.Len(t, flushedMessages, 3)
	mu.Unlock()
}

func TestBulkCollector_GracefulShutdown(t *testing.T) {
	t.Parallel()

	bulkSize := 100 // Large size to avoid size-based flush
	var flushedCount int32
	var flushedMessages []amqp.Delivery
	var mu sync.Mutex

	bc := NewBulkCollector(bulkSize, 5*time.Second) // Long timeout
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushedCount, 1)
		mu.Lock()
		flushedMessages = append(flushedMessages, messages...)
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	var startErr error
	done := make(chan struct{})

	// Start collector in background
	go func() {
		startErr = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Add some messages
	for i := 0; i < 3; i++ {
		msg := amqp.Delivery{Body: []byte{byte(i)}}
		err := bc.Add(msg)
		require.NoError(t, err)
	}

	// Wait for messages to be processed by collector
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for collector to stop
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("collector did not stop")
	}

	// Verify final flush happened
	assert.Equal(t, int32(1), atomic.LoadInt32(&flushedCount))
	mu.Lock()
	assert.Len(t, flushedMessages, 3)
	mu.Unlock()
	assert.Equal(t, context.Canceled, startErr)
}

func TestBulkCollector_EmptyBulkOnShutdown(t *testing.T) {
	t.Parallel()

	var flushedCount int32

	bc := NewBulkCollector(10, 5*time.Second)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushedCount, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Don't add any messages, just shutdown
	cancel()

	// Wait for collector to stop
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("collector did not stop")
	}

	// Verify no flush happened (empty buffer)
	assert.Equal(t, int32(0), atomic.LoadInt32(&flushedCount))
}

func TestBulkCollector_Stop(t *testing.T) {
	t.Parallel()

	var flushedCount int32
	var flushedMessages []amqp.Delivery
	var mu sync.Mutex

	bc := NewBulkCollector(100, 5*time.Second)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushedCount, 1)
		mu.Lock()
		flushedMessages = append(flushedMessages, messages...)
		mu.Unlock()
		return nil
	})

	ctx := context.Background()

	done := make(chan struct{})

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Add some messages
	for i := 0; i < 3; i++ {
		msg := amqp.Delivery{Body: []byte{byte(i)}}
		err := bc.Add(msg)
		require.NoError(t, err)
	}

	// Wait for messages to be processed by collector
	time.Sleep(50 * time.Millisecond)

	// Stop the collector
	bc.Stop()

	// Wait for collector to stop
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("collector did not stop")
	}

	// Verify final flush happened
	assert.Equal(t, int32(1), atomic.LoadInt32(&flushedCount))
	mu.Lock()
	assert.Len(t, flushedMessages, 3)
	mu.Unlock()
}

func TestBulkCollector_AddAfterStop(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		return nil
	})

	ctx := context.Background()

	done := make(chan struct{})

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Stop the collector
	bc.Stop()

	// Wait for collector to stop
	<-done

	// Wait a bit more to ensure state is updated
	time.Sleep(20 * time.Millisecond)

	// Try to add after stop
	msg := amqp.Delivery{Body: []byte("test")}
	err := bc.Add(msg)

	// Should return ErrCollectorNotStarted since collector has stopped
	assert.Error(t, err)
	assert.Equal(t, ErrCollectorNotStarted, err)
}

func TestBulkCollector_ManualFlush(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(100, 5*time.Second)

	// Manually add messages using internal state (for testing Flush method)
	bc.mu.Lock()
	bc.messages = []amqp.Delivery{
		{Body: []byte("msg1")},
		{Body: []byte("msg2")},
		{Body: []byte("msg3")},
	}
	bc.mu.Unlock()

	// Manual flush
	messages := bc.Flush()

	assert.Len(t, messages, 3)
	assert.Equal(t, 0, bc.Size())
}

func TestBulkCollector_ManualFlushEmpty(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(100, 5*time.Second)

	// Manual flush on empty buffer
	messages := bc.Flush()

	assert.Nil(t, messages)
	assert.Equal(t, 0, bc.Size())
}

func TestBulkCollector_MultipleSizeBasedFlushes(t *testing.T) {
	t.Parallel()

	bulkSize := 3
	var flushedBatches [][]amqp.Delivery
	var mu sync.Mutex

	bc := NewBulkCollector(bulkSize, 5*time.Second)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		mu.Lock()
		batchCopy := make([]amqp.Delivery, len(messages))
		copy(batchCopy, messages)
		flushedBatches = append(flushedBatches, batchCopy)
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Add 9 messages (should trigger 3 flushes)
	for i := 0; i < 9; i++ {
		msg := amqp.Delivery{Body: []byte{byte(i)}}
		err := bc.Add(msg)
		require.NoError(t, err)
		// Small delay to ensure order
		time.Sleep(time.Millisecond)
	}

	// Wait for flushes to complete
	time.Sleep(100 * time.Millisecond)

	// Verify 3 flush batches
	mu.Lock()
	assert.Len(t, flushedBatches, 3)
	for _, batch := range flushedBatches {
		assert.Len(t, batch, bulkSize)
	}
	mu.Unlock()
}

func TestBulkCollector_StopIdempotent(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		return nil
	})

	ctx := context.Background()

	done := make(chan struct{})

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Stop multiple times - should not panic
	bc.Stop()
	bc.Stop()
	bc.Stop()

	// Wait for collector to stop
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("collector did not stop")
	}
}

func TestBulkCollector_StopBeforeStart(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(10, 100*time.Millisecond)

	// Stop before start - should not panic or block
	bc.Stop()

	// Should still be able to set callback and start
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		return nil
	})

	// Create new collector since done channel might be closed
	bc2 := NewBulkCollector(10, 100*time.Millisecond)
	bc2.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := bc2.Start(ctx)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestBulkCollector_Size(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(100, 5*time.Second)

	assert.Equal(t, 0, bc.Size())

	// Manually add messages for Size test
	bc.mu.Lock()
	bc.messages = []amqp.Delivery{
		{Body: []byte("msg1")},
		{Body: []byte("msg2")},
	}
	bc.mu.Unlock()

	assert.Equal(t, 2, bc.Size())
}

func TestBulkCollector_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	bc := NewBulkCollector(100, 100*time.Millisecond)
	var flushCount int32
	bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
		atomic.AddInt32(&flushCount, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	// Start collector in background
	go func() {
		_ = bc.Start(ctx)
		close(done)
	}()

	// Wait for collector to start
	time.Sleep(20 * time.Millisecond)

	// Concurrent adds
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				msg := amqp.Delivery{Body: []byte{byte(idx*10 + j)}}
				_ = bc.Add(msg)
			}
		}(i)
	}

	wg.Wait()

	// Stop to trigger final flush
	cancel()
	<-done

	// Verify at least one flush happened (from shutdown with remaining messages)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&flushCount), int32(1))
}
