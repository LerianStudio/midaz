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
	timerActive := true

	defer func() {
		timer.Stop()

		// Final flush on shutdown
		c.mu.Lock()
		remaining := c.buffer
		c.buffer = make([]redisTransaction.SyncKey, 0, c.batchSize)
		c.mu.Unlock()

		if len(remaining) > 0 && c.flushFn != nil {
			c.logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: shutdown — final flush of %d remaining keys", len(remaining)))

			// Use a short timeout context for final flush
			flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			c.flushFn(flushCtx, remaining)
		}
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		// 1. Poll: fetch eligible keys from ZSET
		c.mu.Lock()
		remaining := c.batchSize - len(c.buffer)
		c.mu.Unlock()

		keys, err := fetchKeys(ctx, int64(remaining))
		if err != nil {
			// Log but don't exit — transient Redis errors should not kill the worker
			c.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncCollector: fetch keys error: "+err.Error())

			// Brief pause before retry to avoid tight error loop
			if waitOrShutdown(ctx, c.pollInterval) {
				return
			}

			continue
		}

		if len(keys) > 0 {
			c.mu.Lock()
			c.buffer = append(c.buffer, keys...)
			bufLen := len(c.buffer)
			c.mu.Unlock()

			c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: fetched %d keys, buffer=%d/%d", len(keys), bufLen, c.batchSize))

			// Reset timer when first key arrives in an empty buffer
			if bufLen == len(keys) && timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				timer.Reset(c.flushTimeout)
			}

			// 2. SIZE trigger: buffer full → flush immediately
			if bufLen >= c.batchSize {
				c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: SIZE trigger fired (buffer=%d >= batch_size=%d), flushing now", bufLen, c.batchSize))
				c.flush(ctx)

				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				timer.Reset(c.flushTimeout)

				timerActive = true

				continue // tight loop — fetch more immediately
			}

			// 3. Got keys but not enough yet — tight loop (busy mode)
			continue
		}

		// No keys fetched from ZSET

		// 4. Draining mode: buffer has items but no new keys arriving
		c.mu.Lock()
		bufLen := len(c.buffer)
		c.mu.Unlock()

		if bufLen > 0 {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				// TIMEOUT trigger: flush what we have
				c.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncCollector: TIMEOUT trigger fired (%v elapsed, buffer=%d), flushing now", c.flushTimeout, bufLen))
				c.flush(ctx)

				timer.Reset(c.flushTimeout)

				timerActive = true
			case <-time.After(c.pollInterval):
				// Poll again to check for new keys
			}

			continue
		}

		// 5. Idle mode: buffer empty and nothing in ZSET
		c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: idle mode — no keys in ZSET, waiting for next scheduled key")
		timer.Stop()

		timerActive = false

		if waitForNext(ctx) {
			return // shutdown requested
		}

		c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: woke up from idle, resuming polling")

		// Re-arm timer for next accumulation cycle
		timer.Reset(c.flushTimeout)

		timerActive = true
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
