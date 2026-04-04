// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	redisTransaction "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
)

// BalanceSyncCollector accumulates Redis ZSET keys for batch processing.
// It triggers a flush when the batch size is reached or a timeout elapses,
// whichever comes first (dual-trigger pattern).
//
// Unlike the RabbitMQ BulkCollector which receives messages via a Go channel (push),
// this collector polls a Redis ZSET for eligible keys (pull). It adapts the same
// dual-trigger concept to a polling-based data source.
//
// Lifecycle: continuous loop with idle backoff (not single-use like BulkCollector).
type BalanceSyncCollector struct {
	mu           sync.Mutex
	batchSize    int
	flushTimeout time.Duration
	pollInterval time.Duration
	idleBackoff  time.Duration
	buffer       []redisTransaction.SyncKey
	flushFn      FlushFunc
	logger       libLog.Logger
}

// FlushFunc is called when the collector flushes accumulated keys.
// It receives the batch of keys and returns true if any processing occurred.
type FlushFunc func(ctx context.Context, keys []redisTransaction.SyncKey) bool

// FetchKeysFunc fetches eligible keys from the ZSET schedule.
// limit is the maximum number of keys to return.
type FetchKeysFunc func(ctx context.Context, limit int64) ([]redisTransaction.SyncKey, error)

// WaitForNextFunc waits until the next scheduled key is due or a backoff period elapses.
// Returns true if shutdown was requested during the wait.
type WaitForNextFunc func(ctx context.Context) bool

// NewBalanceSyncCollector creates a new collector with the given configuration.
func NewBalanceSyncCollector(
	batchSize int,
	flushTimeout time.Duration,
	pollInterval time.Duration,
	idleBackoff time.Duration,
	logger libLog.Logger,
) *BalanceSyncCollector {
	if batchSize <= 0 {
		batchSize = 50
	}

	if flushTimeout <= 0 {
		flushTimeout = 500 * time.Millisecond
	}

	if pollInterval <= 0 {
		pollInterval = 50 * time.Millisecond
	}

	if idleBackoff <= 0 {
		idleBackoff = 10 * time.Second
	}

	return &BalanceSyncCollector{
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		pollInterval: pollInterval,
		idleBackoff:  idleBackoff,
		buffer:       make([]redisTransaction.SyncKey, 0, batchSize),
		logger:       logger,
	}
}

// SetFlushCallback sets the function called when the collector flushes accumulated keys.
func (c *BalanceSyncCollector) SetFlushCallback(fn FlushFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.flushFn = fn
}

// Run starts the collector's main loop. It polls for eligible keys and flushes
// based on the dual-trigger (size OR timeout). Blocks until context is cancelled.
//
// The loop has three modes:
//   - Busy mode: keys are available, tight poll loop to accumulate quickly
//   - Draining mode: buffer has items but no new keys, wait for timeout or poll
//   - Idle mode: nothing to do, sleep until next scheduled key
func (c *BalanceSyncCollector) Run(ctx context.Context, fetchKeys FetchKeysFunc, waitForNext WaitForNextFunc) {
	timer := time.NewTimer(c.flushTimeout)
	pollTimer := time.NewTimer(c.pollInterval)

	defer func() {
		timer.Stop()
		pollTimer.Stop()
		c.flushRemaining(ctx)
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		// Poll: fetch eligible keys from ZSET
		c.mu.Lock()
		remaining := c.batchSize - len(c.buffer)
		c.mu.Unlock()

		keys, err := fetchKeys(ctx, int64(remaining))
		if err != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncCollector: fetch keys error: "+err.Error())

			// If the buffer already has items, skip the sleep and re-enter the
			// draining path so the timeout trigger can still flush on time.
			// Only sleep when the buffer is empty to avoid a tight error loop.
			c.mu.Lock()
			hasItems := len(c.buffer) > 0
			c.mu.Unlock()

			if hasItems {
				continue
			}

			if waitOrShutdown(ctx, c.pollInterval) {
				return
			}

			continue
		}

		if len(keys) > 0 {
			if c.handleBusyMode(ctx, keys, timer) {
				continue
			}
		}

		// No keys fetched from ZSET
		c.mu.Lock()
		bufLen := len(c.buffer)
		c.mu.Unlock()

		if bufLen > 0 {
			c.handleDrainingMode(ctx, bufLen, timer, pollTimer)
			continue
		}

		if c.handleIdleMode(ctx, timer, waitForNext) {
			return // shutdown requested
		}
	}
}

// handleBusyMode processes newly fetched keys: appends to buffer, flushes on SIZE trigger,
// or continues the tight poll loop. Returns true if the main loop should continue immediately.
func (c *BalanceSyncCollector) handleBusyMode(ctx context.Context, keys []redisTransaction.SyncKey, timer *time.Timer) bool {
	c.mu.Lock()
	wasEmpty := len(c.buffer) == 0
	c.buffer = append(c.buffer, keys...)
	bufLen := len(c.buffer)
	c.mu.Unlock()

	c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: fetched %d keys, buffer=%d/%d", len(keys), bufLen, c.batchSize))

	// Reset timer when first keys arrive in a previously empty buffer
	if wasEmpty {
		stopAndDrain(timer)
		timer.Reset(c.flushTimeout)
	}

	// SIZE trigger: buffer full → flush immediately
	if bufLen >= c.batchSize {
		c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: SIZE trigger fired (buffer=%d >= batch_size=%d), flushing now", bufLen, c.batchSize))
		c.flush(ctx)
		stopAndDrain(timer)
		timer.Reset(c.flushTimeout)
	}

	return true // always continue tight loop after fetching keys
}

// handleDrainingMode waits for either the TIMEOUT trigger or the poll interval
// when the buffer has items but no new keys are arriving.
// pollTimer is a reusable timer for the poll interval (avoids time.After allocation leak).
func (c *BalanceSyncCollector) handleDrainingMode(ctx context.Context, bufLen int, timer *time.Timer, pollTimer *time.Timer) {
	stopAndDrain(pollTimer)
	pollTimer.Reset(c.pollInterval)

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: TIMEOUT trigger fired (%v elapsed, buffer=%d), flushing now", c.flushTimeout, bufLen))
		c.flush(ctx)
		timer.Reset(c.flushTimeout)
	case <-pollTimer.C:
		// Poll again to check for new keys
	}
}

// handleIdleMode stops the timer and waits for the next scheduled key or shutdown.
// Returns true if shutdown was requested.
func (c *BalanceSyncCollector) handleIdleMode(ctx context.Context, timer *time.Timer, waitForNext WaitForNextFunc) bool {
	c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: idle mode — no keys in ZSET, waiting for next scheduled key")
	stopAndDrain(timer)

	if waitForNext(ctx) {
		return true
	}

	c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: woke up from idle, resuming polling")
	timer.Reset(c.flushTimeout)

	return false
}

// flushRemaining drains any leftover buffer on shutdown.
// ctx carries context values (tenant ID, PG connection) needed by the flush callback.
// The cancellation signal is stripped via context.WithoutCancel so the final flush
// can complete even after the parent context has been cancelled.
func (c *BalanceSyncCollector) flushRemaining(ctx context.Context) {
	c.mu.Lock()
	remaining := c.buffer
	c.buffer = make([]redisTransaction.SyncKey, 0, c.batchSize)
	c.mu.Unlock()

	if len(remaining) > 0 && c.flushFn != nil {
		c.logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: shutdown — final flush of %d remaining keys", len(remaining)))

		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		c.flushFn(flushCtx, remaining)
	}
}

// flush drains the buffer and calls the flush callback.
func (c *BalanceSyncCollector) flush(ctx context.Context) {
	c.mu.Lock()

	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}

	keys := c.buffer
	c.buffer = make([]redisTransaction.SyncKey, 0, c.batchSize)
	c.mu.Unlock()

	if c.flushFn != nil {
		c.flushFn(ctx, keys)
	}
}

// Size returns the current number of accumulated keys.
func (c *BalanceSyncCollector) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.buffer)
}

// stopAndDrain stops a timer and drains its channel if needed.
func stopAndDrain(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// waitOrShutdown waits for duration d or returns true if ctx is cancelled.
func waitOrShutdown(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}
