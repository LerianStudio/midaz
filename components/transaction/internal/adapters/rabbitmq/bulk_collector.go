// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	amqp "github.com/rabbitmq/amqp091-go"
)

// BulkCollector configuration errors.
var (
	// ErrInvalidBulkSize is returned when bulk size is less than or equal to 0.
	ErrInvalidBulkSize = errors.New("bulk size must be greater than 0")

	// ErrInvalidFlushTimeout is returned when flush timeout is less than or equal to 0.
	ErrInvalidFlushTimeout = errors.New("flush timeout must be greater than 0")

	// ErrNilFlushFunc is returned when flush function is nil.
	ErrNilFlushFunc = errors.New("flush function must not be nil")

	// ErrNilLogger is returned when logger is nil.
	ErrNilLogger = errors.New("logger must not be nil")
)

// BulkFlushFunc is called when the bulk is ready to be processed.
// It receives the collected messages and returns an error if processing fails.
// The function is responsible for acknowledging or rejecting the messages.
type BulkFlushFunc func(ctx context.Context, messages []amqp.Delivery) error

// BulkCollector accumulates RabbitMQ messages until either the bulk size threshold
// is reached or the flush timeout expires, then calls the flush callback.
//
// Thread-safety: BulkCollector is designed to be used by a single goroutine.
// The Run method blocks and processes messages sequentially from the input channel.
// Do not call Add from multiple goroutines concurrently.
type BulkCollector struct {
	size         int
	flushTimeout time.Duration
	flushFunc    BulkFlushFunc
	logger       libLog.Logger
	messages     []amqp.Delivery
	firstArrival time.Time
	mu           sync.Mutex
}

// BulkCollectorConfig holds configuration for the BulkCollector.
type BulkCollectorConfig struct {
	// Size is the number of messages to collect before flushing.
	// Must be greater than 0.
	Size int

	// FlushTimeout is the maximum time to wait before flushing an incomplete bulk.
	// Must be greater than 0.
	FlushTimeout time.Duration

	// FlushFunc is called when the bulk is ready to be processed.
	// Must not be nil.
	FlushFunc BulkFlushFunc

	// Logger for logging bulk operations.
	// Must not be nil.
	Logger libLog.Logger
}

// NewBulkCollector creates a new BulkCollector with the given configuration.
// Returns an error if the configuration is invalid.
func NewBulkCollector(cfg BulkCollectorConfig) (*BulkCollector, error) {
	if cfg.Size <= 0 {
		return nil, ErrInvalidBulkSize
	}

	if cfg.FlushTimeout <= 0 {
		return nil, ErrInvalidFlushTimeout
	}

	if cfg.FlushFunc == nil {
		return nil, ErrNilFlushFunc
	}

	if cfg.Logger == nil {
		return nil, ErrNilLogger
	}

	return &BulkCollector{
		size:         cfg.Size,
		flushTimeout: cfg.FlushTimeout,
		flushFunc:    cfg.FlushFunc,
		logger:       cfg.Logger,
		messages:     make([]amqp.Delivery, 0, cfg.Size),
	}, nil
}

// Run starts the bulk collector loop, reading from the messages channel until
// the context is cancelled or the channel is closed.
//
// The collector will flush when:
// - The bulk size threshold is reached (size-based flush)
// - The flush timeout expires since the first message arrived (time-based flush)
// - The context is cancelled (graceful shutdown)
// - The messages channel is closed (channel closure)
//
// This method blocks until the context is cancelled or the channel is closed.
func (bc *BulkCollector) Run(ctx context.Context, messages <-chan amqp.Delivery) {
	bc.logger.Log(ctx, libLog.LevelInfo, "BulkCollector started",
		libLog.Int("bulkSize", bc.size),
		libLog.Any("flushTimeout", bc.flushTimeout))

	// Use a reusable timer to avoid memory leak from time.After in hot loop
	timer := time.NewTimer(bc.flushTimeout)
	defer timer.Stop()

	for {
		// Calculate timeout for select and reset timer
		timeout := bc.calculateTimeout()

		// Stop and drain timer before reset to avoid race
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		timer.Reset(timeout)

		select {
		case <-ctx.Done():
			// Graceful shutdown: flush any pending messages
			bc.logger.Log(ctx, libLog.LevelInfo, "BulkCollector shutting down, flushing pending messages",
				libLog.Int("pendingCount", bc.Len()))
			bc.flush(ctx, "shutdown")

			return

		case msg, ok := <-messages:
			if !ok {
				// Channel closed: flush any pending messages
				bc.logger.Log(ctx, libLog.LevelInfo, "BulkCollector channel closed, flushing pending messages",
					libLog.Int("pendingCount", bc.Len()))
				bc.flush(ctx, "channel_closed")

				return
			}

			bc.add(msg)

			// Check if size threshold reached (use locked accessor)
			if bc.Len() >= bc.size {
				bc.flush(ctx, "size_threshold")
			}

		case <-timer.C:
			// Timeout expired: flush if we have pending messages (use locked accessor)
			if bc.Len() > 0 {
				bc.flush(ctx, "timeout")
			}
		}
	}
}

// add appends a message to the bulk buffer.
func (bc *BulkCollector) add(msg amqp.Delivery) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.messages) == 0 {
		bc.firstArrival = time.Now()
	}

	bc.messages = append(bc.messages, msg)
}

// flush processes the accumulated messages and resets the buffer.
func (bc *BulkCollector) flush(ctx context.Context, trigger string) {
	bc.mu.Lock()
	if len(bc.messages) == 0 {
		bc.mu.Unlock()
		return
	}

	// Take ownership of messages and reset buffer
	messagesToFlush := bc.messages
	bc.messages = make([]amqp.Delivery, 0, bc.size)
	bc.firstArrival = time.Time{}
	bc.mu.Unlock()

	bc.logger.Log(ctx, libLog.LevelDebug, "BulkCollector flushing",
		libLog.String("trigger", trigger),
		libLog.Int("messageCount", len(messagesToFlush)))

	// Call the flush function with the collected messages
	if err := bc.flushFunc(ctx, messagesToFlush); err != nil {
		bc.logger.Log(ctx, libLog.LevelError, "BulkCollector flush callback failed",
			libLog.String("trigger", trigger),
			libLog.Int("messageCount", len(messagesToFlush)),
			libLog.Err(err))
		// Note: Error handling (ack/nack) is the responsibility of the flush function
	}
}

// calculateTimeout returns the time until the next timeout-based flush should occur.
// Returns flushTimeout if no messages are pending, otherwise returns the remaining
// time until the timeout expires since the first message arrived.
func (bc *BulkCollector) calculateTimeout() time.Duration {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.messages) == 0 {
		// No messages: use full timeout (will be reset when first message arrives)
		return bc.flushTimeout
	}

	// Calculate remaining time until timeout
	elapsed := time.Since(bc.firstArrival)
	remaining := bc.flushTimeout - elapsed

	if remaining <= 0 {
		// Timeout already expired, return minimal duration to trigger immediate flush
		return time.Millisecond
	}

	return remaining
}

// Len returns the current number of messages in the buffer.
// This is primarily useful for testing and monitoring.
func (bc *BulkCollector) Len() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	return len(bc.messages)
}

// FlushNow forces an immediate flush of pending messages.
// This is primarily useful for testing.
func (bc *BulkCollector) FlushNow(ctx context.Context) {
	bc.flush(ctx, "manual")
}
