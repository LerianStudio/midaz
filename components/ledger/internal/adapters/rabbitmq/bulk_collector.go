// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// BulkCollector errors.
var (
	// ErrCollectorNotStarted is returned when Add is called before Start.
	ErrCollectorNotStarted = errors.New("bulk collector not started")

	// ErrCollectorStopped is returned when Add is called after Stop.
	ErrCollectorStopped = errors.New("bulk collector stopped")

	// ErrCollectorAlreadyStarted is returned when Start is called more than once.
	ErrCollectorAlreadyStarted = errors.New("bulk collector already started")

	// ErrNoFlushCallback is returned when Start is called without setting a flush callback.
	ErrNoFlushCallback = errors.New("flush callback not set")
)

// BulkCollector accumulates RabbitMQ messages for bulk processing.
// It triggers a flush when the bulk size is reached or a timeout elapses.
// Thread-safe for single goroutine use (designed for worker pattern).
// The collector is single-use: once Stop() is called, it cannot be restarted.
type BulkCollector struct {
	mu                   sync.Mutex
	messages             []amqp.Delivery
	bulkSize             int
	flushTimeout         time.Duration
	flushCallback        FlushCallbackFunc
	flushErrorHandler    FlushErrorHandler
	contextCancelHandler ContextCancelHandler
	firstMsgTime         time.Time
	inputChan            chan amqp.Delivery
	done                 chan struct{}
	started              bool
	stopped              bool // terminal state, set by Stop()
}

// FlushCallbackFunc is called when the collector flushes accumulated messages.
// It receives the batch of messages and returns results and any error.
type FlushCallbackFunc func(ctx context.Context, messages []amqp.Delivery) error

// FlushErrorHandler is called when the flush callback returns an error.
// It receives the context, the messages that failed, and the error.
type FlushErrorHandler func(ctx context.Context, messages []amqp.Delivery, err error)

// ContextCancelHandler is called when context cancellation interrupts processing.
// It receives the context and the count of messages that were left unacked.
type ContextCancelHandler func(ctx context.Context, messageCount int)

// NewBulkCollector creates a new BulkCollector with the given bulk size and flush timeout.
// bulkSize: maximum number of messages before triggering a flush
// flushTimeout: maximum duration to wait before flushing an incomplete batch
func NewBulkCollector(bulkSize int, flushTimeout time.Duration) *BulkCollector {
	if bulkSize <= 0 {
		bulkSize = 50 // default bulk size
	}

	if flushTimeout <= 0 {
		flushTimeout = 100 * time.Millisecond // default timeout
	}

	return &BulkCollector{
		messages:     make([]amqp.Delivery, 0, bulkSize),
		bulkSize:     bulkSize,
		flushTimeout: flushTimeout,
		inputChan:    make(chan amqp.Delivery, bulkSize),
		done:         make(chan struct{}),
	}
}

// SetFlushCallback sets the callback function invoked on each flush.
// Must be called before Start.
func (bc *BulkCollector) SetFlushCallback(callback FlushCallbackFunc) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.flushCallback = callback
}

// SetFlushErrorHandler sets the error handler function invoked when flush fails.
// Must be called before Start. Optional - if not set, errors are logged but not propagated.
func (bc *BulkCollector) SetFlushErrorHandler(handler FlushErrorHandler) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.flushErrorHandler = handler
}

// SetContextCancelHandler sets the handler called when context cancellation
// interrupts processing. The handler receives the count of messages left unacked.
// Must be called before Start. Optional.
func (bc *BulkCollector) SetContextCancelHandler(handler ContextCancelHandler) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.contextCancelHandler = handler
}

// Add queues a message for bulk processing.
// Returns ErrCollectorNotStarted if Start has not been called.
// Returns ErrCollectorStopped if the collector has been stopped.
func (bc *BulkCollector) Add(msg amqp.Delivery) error {
	bc.mu.Lock()

	if bc.stopped {
		bc.mu.Unlock()
		return ErrCollectorStopped
	}

	if !bc.started {
		bc.mu.Unlock()
		return ErrCollectorNotStarted
	}

	bc.mu.Unlock()

	select {
	case bc.inputChan <- msg:
		return nil
	case <-bc.done:
		return ErrCollectorStopped
	}
}

// Start begins the collector's processing loop.
// It processes messages from the input channel and flushes based on size or timeout.
// Blocks until context is cancelled or Stop is called.
func (bc *BulkCollector) Start(ctx context.Context) error {
	if err := bc.initStart(); err != nil {
		return err
	}

	return bc.runLoop(ctx)
}

// initStart validates and initializes the collector for starting.
// Returns ErrCollectorStopped if the collector was previously stopped.
func (bc *BulkCollector) initStart() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.stopped {
		return ErrCollectorStopped
	}

	if bc.started {
		return ErrCollectorAlreadyStarted
	}

	if bc.flushCallback == nil {
		return ErrNoFlushCallback
	}

	bc.started = true

	return nil
}

// runLoop is the main processing loop that handles messages and triggers flushes.
func (bc *BulkCollector) runLoop(ctx context.Context) error {
	var timer *time.Timer

	var timerChan <-chan time.Time

	defer bc.cleanupOnExit(ctx, timer)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-bc.done:
			return nil

		case msg, ok := <-bc.inputChan:
			if !ok {
				return nil
			}

			timer, timerChan = bc.handleMessage(ctx, msg, timer, timerChan)

		case <-timerChan:
			bc.handleTimeout(ctx)

			timerChan = nil
		}
	}
}

// handleMessage processes an incoming message and returns updated timer state.
func (bc *BulkCollector) handleMessage(
	ctx context.Context,
	msg amqp.Delivery,
	timer *time.Timer,
	timerChan <-chan time.Time,
) (*time.Timer, <-chan time.Time) {
	bc.mu.Lock()

	if len(bc.messages) == 0 {
		bc.firstMsgTime = time.Now()

		timer, timerChan = bc.startOrResetTimer(timer)
	}

	bc.messages = append(bc.messages, msg)

	if len(bc.messages) >= bc.bulkSize {
		messages := bc.messages
		bc.messages = make([]amqp.Delivery, 0, bc.bulkSize)
		bc.mu.Unlock()

		if timer != nil {
			if !timer.Stop() {
				// Drain any stale value from timer channel to prevent unexpected flush
				select {
				case <-timer.C:
				default:
				}
			}
		}

		bc.executeFlush(ctx, messages)

		return timer, nil
	}

	bc.mu.Unlock()

	return timer, timerChan
}

// startOrResetTimer starts a new timer or resets an existing one.
func (bc *BulkCollector) startOrResetTimer(timer *time.Timer) (*time.Timer, <-chan time.Time) {
	if timer == nil {
		timer = time.NewTimer(bc.flushTimeout)
	} else {
		timer.Reset(bc.flushTimeout)
	}

	return timer, timer.C
}

// handleTimeout flushes messages when the timeout is reached.
func (bc *BulkCollector) handleTimeout(ctx context.Context) {
	bc.mu.Lock()

	if len(bc.messages) > 0 {
		messages := bc.messages
		bc.messages = make([]amqp.Delivery, 0, bc.bulkSize)
		bc.mu.Unlock()

		bc.executeFlush(ctx, messages)

		return
	}

	bc.mu.Unlock()
}

// cleanupOnExit performs final flush and cleanup when the loop exits.
// Drains any remaining messages from inputChan before performing the final flush.
// If the original context was cancelled (channel closure), messages are NOT flushed
// to avoid acking with stale delivery tags - they are left for RabbitMQ redelivery.
func (bc *BulkCollector) cleanupOnExit(ctx context.Context, timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}

	// Check if context was cancelled (indicates channel closure)
	if ctx.Err() != nil {
		// Drain input channel to prevent goroutine leak
		bc.drainInputChan()

		bc.mu.Lock()
		messageCount := len(bc.messages)
		// Clear messages - they will be redelivered by RabbitMQ
		bc.messages = make([]amqp.Delivery, 0, bc.bulkSize)
		handler := bc.contextCancelHandler
		bc.started = false
		bc.mu.Unlock()

		// Notify handler about skipped messages (for metrics/logging)
		if handler != nil && messageCount > 0 {
			handler(ctx, messageCount)
		}

		return
	}

	// Normal shutdown path - drain and flush remaining messages
	bc.drainInputChan()

	bc.mu.Lock()

	if len(bc.messages) > 0 {
		messages := bc.messages
		bc.messages = make([]amqp.Delivery, 0, bc.bulkSize)
		bc.mu.Unlock()

		// Create a fresh context with timeout for the final flush
		// This ensures the final batch is not dropped due to cancelled parent context
		const finalFlushTimeout = 30 * time.Second

		flushCtx, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
		defer cancel()

		bc.executeFlush(flushCtx, messages)
	} else {
		bc.mu.Unlock()
	}

	bc.mu.Lock()
	bc.started = false
	bc.mu.Unlock()
}

// drainInputChan reads all remaining messages from inputChan and appends them to bc.messages.
// This ensures no messages are lost when the collector shuts down.
func (bc *BulkCollector) drainInputChan() {
	for {
		select {
		case msg, ok := <-bc.inputChan:
			if !ok {
				return
			}

			bc.mu.Lock()
			bc.messages = append(bc.messages, msg)
			bc.mu.Unlock()
		default:
			// Channel is empty
			return
		}
	}
}

// executeFlush invokes the flush callback with the given messages.
// If the flush fails and an error handler is set, it will be called.
func (bc *BulkCollector) executeFlush(ctx context.Context, messages []amqp.Delivery) {
	if bc.flushCallback != nil && len(messages) > 0 {
		if err := bc.flushCallback(ctx, messages); err != nil {
			if bc.flushErrorHandler != nil {
				bc.flushErrorHandler(ctx, messages, err)
			}
		}
	}
}

// Stop signals the collector to stop processing.
// Any remaining messages will be flushed before returning.
// Once stopped, the collector cannot be restarted.
func (bc *BulkCollector) Stop() {
	bc.mu.Lock()

	if bc.stopped || !bc.started {
		bc.mu.Unlock()
		return
	}

	bc.stopped = true
	bc.mu.Unlock()

	select {
	case <-bc.done:
		// Already closed
	default:
		close(bc.done)
	}
}

// Flush manually triggers a flush of accumulated messages.
// Returns the messages that were flushed.
func (bc *BulkCollector) Flush() []amqp.Delivery {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.messages) == 0 {
		return nil
	}

	messages := bc.messages
	bc.messages = make([]amqp.Delivery, 0, bc.bulkSize)

	return messages
}

// Size returns the current number of accumulated messages.
func (bc *BulkCollector) Size() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	return len(bc.messages)
}

// BulkSize returns the configured bulk size threshold.
func (bc *BulkCollector) BulkSize() int {
	return bc.bulkSize
}

// FlushTimeout returns the configured flush timeout.
func (bc *BulkCollector) FlushTimeout() time.Duration {
	return bc.flushTimeout
}
