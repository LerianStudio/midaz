// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bulkTestLogger is initialized once for bulk collector tests.
var (
	bulkTestLogger     libLog.Logger
	bulkTestLoggerOnce sync.Once
)

func getBulkTestLogger() libLog.Logger {
	bulkTestLoggerOnce.Do(func() {
		logger, err := libZap.New(libZap.Config{
			Environment:     libZap.EnvironmentLocal,
			Level:           "error", // Suppress debug logs in tests
			OTelLibraryName: "midaz-bulk-collector-tests",
		})
		if err != nil {
			panic(err)
		}

		bulkTestLogger = logger
	})

	return bulkTestLogger
}

// =============================================================================
// UNIT TESTS - NewBulkCollector Configuration Validation
// =============================================================================

func TestNewBulkCollector_ConfigValidation(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()
	noopFlushFunc := func(_ context.Context, _ []amqp.Delivery) error { return nil }

	tests := []struct {
		name        string
		cfg         BulkCollectorConfig
		expectError error
	}{
		{
			name: "valid_config",
			cfg: BulkCollectorConfig{
				Size:         50,
				FlushTimeout: 100 * time.Millisecond,
				FlushFunc:    noopFlushFunc,
				Logger:       logger,
			},
			expectError: nil,
		},
		{
			name: "zero_size_returns_error",
			cfg: BulkCollectorConfig{
				Size:         0,
				FlushTimeout: 100 * time.Millisecond,
				FlushFunc:    noopFlushFunc,
				Logger:       logger,
			},
			expectError: ErrInvalidBulkSize,
		},
		{
			name: "negative_size_returns_error",
			cfg: BulkCollectorConfig{
				Size:         -1,
				FlushTimeout: 100 * time.Millisecond,
				FlushFunc:    noopFlushFunc,
				Logger:       logger,
			},
			expectError: ErrInvalidBulkSize,
		},
		{
			name: "zero_timeout_returns_error",
			cfg: BulkCollectorConfig{
				Size:         50,
				FlushTimeout: 0,
				FlushFunc:    noopFlushFunc,
				Logger:       logger,
			},
			expectError: ErrInvalidFlushTimeout,
		},
		{
			name: "negative_timeout_returns_error",
			cfg: BulkCollectorConfig{
				Size:         50,
				FlushTimeout: -1 * time.Millisecond,
				FlushFunc:    noopFlushFunc,
				Logger:       logger,
			},
			expectError: ErrInvalidFlushTimeout,
		},
		{
			name: "nil_flush_func_returns_error",
			cfg: BulkCollectorConfig{
				Size:         50,
				FlushTimeout: 100 * time.Millisecond,
				FlushFunc:    nil,
				Logger:       logger,
			},
			expectError: ErrNilFlushFunc,
		},
		{
			name: "nil_logger_returns_error",
			cfg: BulkCollectorConfig{
				Size:         50,
				FlushTimeout: 100 * time.Millisecond,
				FlushFunc:    noopFlushFunc,
				Logger:       nil,
			},
			expectError: ErrNilLogger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bc, err := NewBulkCollector(tt.cfg)

			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, bc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, bc)
			}
		})
	}
}

// =============================================================================
// UNIT TESTS - BulkCollector Size-Based Flush
// =============================================================================

func TestBulkCollector_SizeBasedFlush(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const bulkSize = 5
	flushCount := 0
	var flushMu sync.Mutex
	var receivedMessages []amqp.Delivery

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		flushCount++
		receivedMessages = append(receivedMessages, msgs...)

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         bulkSize,
		FlushTimeout: 10 * time.Second, // Long timeout to ensure size trigger
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create a channel with exactly bulkSize messages
	msgChan := make(chan amqp.Delivery, bulkSize)
	for i := 0; i < bulkSize; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	close(msgChan)

	// Run collector
	ctx := context.Background()
	bc.Run(ctx, msgChan)

	// Verify flush was called once with all messages
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Equal(t, 1, flushCount, "Expected exactly one flush")
	assert.Len(t, receivedMessages, bulkSize, "Expected all messages to be flushed")
}

// =============================================================================
// UNIT TESTS - BulkCollector Time-Based Flush
// =============================================================================

func TestBulkCollector_TimeBasedFlush(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const bulkSize = 100 // Large size to ensure timeout triggers first
	const messageCount = 3
	flushCount := 0
	var flushMu sync.Mutex
	var receivedMessages []amqp.Delivery

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		flushCount++
		receivedMessages = append(receivedMessages, msgs...)

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         bulkSize,
		FlushTimeout: 50 * time.Millisecond, // Short timeout
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create a channel with fewer messages than bulk size
	msgChan := make(chan amqp.Delivery, messageCount)
	for i := 0; i < messageCount; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	// Start collector in goroutine
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		bc.Run(ctx, msgChan)
	}()

	// Wait for timeout-based flush
	time.Sleep(150 * time.Millisecond)

	// Cancel to stop the collector
	cancel()
	close(msgChan)

	// Give collector time to process shutdown
	time.Sleep(50 * time.Millisecond)

	// Verify flush was called with partial messages
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.GreaterOrEqual(t, flushCount, 1, "Expected at least one flush")
	assert.Len(t, receivedMessages, messageCount, "Expected all messages to be flushed")
}

// =============================================================================
// UNIT TESTS - BulkCollector Graceful Shutdown
// =============================================================================

func TestBulkCollector_GracefulShutdown(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const bulkSize = 100 // Large size to prevent size-based flush
	const messageCount = 3
	var flushMu sync.Mutex
	var receivedMessages []amqp.Delivery

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		receivedMessages = append(receivedMessages, msgs...)

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         bulkSize,
		FlushTimeout: 10 * time.Second, // Long timeout
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create a channel with messages
	msgChan := make(chan amqp.Delivery, messageCount)
	for i := 0; i < messageCount; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	// Start collector in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		bc.Run(ctx, msgChan)
		close(done)
	}()

	// Let collector receive messages
	time.Sleep(50 * time.Millisecond)

	// Trigger graceful shutdown
	cancel()

	// Wait for collector to finish
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Collector did not shutdown within timeout")
	}

	// Verify pending messages were flushed
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Len(t, receivedMessages, messageCount, "Expected pending messages to be flushed on shutdown")
}

// =============================================================================
// UNIT TESTS - BulkCollector Empty Bulk Handling
// =============================================================================

func TestBulkCollector_EmptyBulk(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	flushCount := 0
	var flushMu sync.Mutex

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		flushCount++

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         50,
		FlushTimeout: 50 * time.Millisecond,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create and immediately close an empty channel
	msgChan := make(chan amqp.Delivery)
	close(msgChan)

	// Run collector
	bc.Run(context.Background(), msgChan)

	// Verify no flush was called for empty buffer
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Equal(t, 0, flushCount, "Expected no flush for empty buffer")
}

// =============================================================================
// UNIT TESTS - BulkCollector Channel Closure
// =============================================================================

func TestBulkCollector_ChannelClosure(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const messageCount = 3
	var flushMu sync.Mutex
	var receivedMessages []amqp.Delivery

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		receivedMessages = append(receivedMessages, msgs...)

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         100, // Large size
		FlushTimeout: 10 * time.Second,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create a channel with messages
	msgChan := make(chan amqp.Delivery, messageCount)
	for i := 0; i < messageCount; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	// Close channel to trigger flush
	close(msgChan)

	// Run collector
	bc.Run(context.Background(), msgChan)

	// Verify messages were flushed on channel closure
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Len(t, receivedMessages, messageCount, "Expected messages to be flushed on channel closure")
}

// =============================================================================
// UNIT TESTS - BulkCollector Len Method
// =============================================================================

func TestBulkCollector_Len(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         100,
		FlushTimeout: 10 * time.Second,
		FlushFunc:    func(_ context.Context, _ []amqp.Delivery) error { return nil },
		Logger:       logger,
	})
	require.NoError(t, err)

	// Initially empty
	assert.Equal(t, 0, bc.Len())

	// Add messages manually using the internal add method
	bc.add(amqp.Delivery{Body: []byte{1}})
	assert.Equal(t, 1, bc.Len())

	bc.add(amqp.Delivery{Body: []byte{2}})
	assert.Equal(t, 2, bc.Len())

	// Flush and verify empty
	bc.FlushNow(context.Background())
	assert.Equal(t, 0, bc.Len())
}

// =============================================================================
// UNIT TESTS - BulkCollector FlushNow
// =============================================================================

func TestBulkCollector_FlushNow(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const messageCount = 3
	var flushMu sync.Mutex
	var receivedMessages []amqp.Delivery
	flushTriggers := make([]string, 0)

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		receivedMessages = append(receivedMessages, msgs...)
		flushTriggers = append(flushTriggers, "manual")

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         100,
		FlushTimeout: 10 * time.Second,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Add messages manually
	for i := 0; i < messageCount; i++ {
		bc.add(amqp.Delivery{Body: []byte{byte(i)}})
	}

	// Manual flush
	bc.FlushNow(context.Background())

	// Verify
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Len(t, receivedMessages, messageCount)
	assert.Len(t, flushTriggers, 1)
}

// =============================================================================
// UNIT TESTS - BulkCollector FlushFunc Error Handling
// =============================================================================

func TestBulkCollector_FlushFuncError(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	flushError := errors.New("flush callback failed")
	flushCallCount := 0
	var flushMu sync.Mutex

	flushFunc := func(_ context.Context, _ []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		flushCallCount++

		return flushError
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         2,
		FlushTimeout: 10 * time.Second,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Add messages manually
	bc.add(amqp.Delivery{Body: []byte{1}})
	bc.add(amqp.Delivery{Body: []byte{2}})

	// Manual flush - should call flushFunc which returns error
	bc.FlushNow(context.Background())

	// Verify flush was called even though it returned error
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Equal(t, 1, flushCallCount, "FlushFunc should be called once")
	assert.Equal(t, 0, bc.Len(), "Buffer should be cleared even after flush error")
}

func TestBulkCollector_FlushFuncError_ContinuesProcessing(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const bulkSize = 2
	flushCallCount := 0
	var flushMu sync.Mutex

	// First flush fails, second succeeds
	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		flushCallCount++
		count := flushCallCount
		flushMu.Unlock()

		if count == 1 {
			return errors.New("first flush fails")
		}

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         bulkSize,
		FlushTimeout: 10 * time.Second,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create channel with enough messages for 2 flushes
	msgChan := make(chan amqp.Delivery, bulkSize*2)
	for i := 0; i < bulkSize*2; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	close(msgChan)

	// Run collector
	bc.Run(context.Background(), msgChan)

	// Verify both flushes were attempted (error didn't stop processing)
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Equal(t, 2, flushCallCount, "Both flushes should be attempted despite first error")
}

// =============================================================================
// UNIT TESTS - BulkCollector Multiple Bulk Cycles
// =============================================================================

func TestBulkCollector_MultipleBulkCycles(t *testing.T) {
	t.Parallel()

	logger := getBulkTestLogger()

	const bulkSize = 3
	const totalMessages = 10 // Should result in 4 flushes (3+3+3+1)
	flushCount := 0
	var flushMu sync.Mutex
	flushSizes := make([]int, 0)

	flushFunc := func(_ context.Context, msgs []amqp.Delivery) error {
		flushMu.Lock()
		defer flushMu.Unlock()

		flushCount++
		flushSizes = append(flushSizes, len(msgs))

		return nil
	}

	bc, err := NewBulkCollector(BulkCollectorConfig{
		Size:         bulkSize,
		FlushTimeout: 10 * time.Second,
		FlushFunc:    flushFunc,
		Logger:       logger,
	})
	require.NoError(t, err)

	// Create channel with total messages
	msgChan := make(chan amqp.Delivery, totalMessages)
	for i := 0; i < totalMessages; i++ {
		msgChan <- amqp.Delivery{Body: []byte{byte(i)}}
	}

	close(msgChan)

	// Run collector
	bc.Run(context.Background(), msgChan)

	// Verify multiple flush cycles
	flushMu.Lock()
	defer flushMu.Unlock()

	assert.Equal(t, 4, flushCount, "Expected 4 flush cycles (3+3+3+1)")

	// Verify flush sizes
	assert.Equal(t, []int{3, 3, 3, 1}, flushSizes)
}
