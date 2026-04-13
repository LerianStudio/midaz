// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
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

	return &BalanceSyncCollector{
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		pollInterval: pollInterval,
		buffer:       make([]redisTransaction.SyncKey, 0, batchSize),
		logger:       logger,
	}
}

// Run starts the collector's main loop. It polls for eligible keys and flushes
// based on the dual-trigger (size OR timeout). Blocks until context is cancelled.
//
// The loop has three modes:
//   - Busy mode: keys are available, tight poll loop to accumulate quickly
//   - Draining mode: buffer has items but no new keys, wait for timeout or poll
//   - Idle mode: nothing to do, sleep until next scheduled key
func (c *BalanceSyncCollector) Run(ctx context.Context, flushFn FlushFunc, fetchKeys FetchKeysFunc, waitForNext WaitForNextFunc) {
	c.flushFn = flushFn
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

		// Calculate how many keys to request: only ask for enough to fill the
		// buffer up to batchSize. This avoids over-fetching and claiming more
		// keys than we can flush in one batch.
		// The mutex guards against concurrent reads from Size() which is public
		// and may be called from another goroutine (e.g., monitoring, tests).
		c.mu.Lock()
		remaining := c.batchSize - len(c.buffer)
		c.mu.Unlock()

		keys, err := fetchKeys(ctx, int64(remaining))
		if err != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncCollector: fetch keys error", libLog.Err(err))

			// If the buffer already has items, skip the sleep and re-enter the
			// draining path so the timeout trigger can still flush on time.
			// Only sleep when the buffer is empty to avoid a tight error loop.
			c.mu.Lock()
			hasItems := len(c.buffer) > 0
			c.mu.Unlock()

			if hasItems {
				continue
			}

			if waitOrDone(ctx, c.pollInterval, c.logger) {
				return
			}

			continue
		}

		if len(keys) > 0 {
			c.handleBusyMode(ctx, keys, timer)
			continue
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

// handleBusyMode processes newly fetched keys: appends to buffer and flushes on
// SIZE trigger. After this, the main loop always continues immediately (tight poll).
func (c *BalanceSyncCollector) handleBusyMode(ctx context.Context, keys []redisTransaction.SyncKey, timer *time.Timer) {
	c.mu.Lock()
	wasEmpty := len(c.buffer) == 0
	c.buffer = append(c.buffer, keys...)
	bufLen := len(c.buffer)
	c.mu.Unlock()

	c.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncCollector: fetched keys",
		libLog.Int("fetched", len(keys)),
		libLog.Int("buffer", bufLen),
		libLog.Int("batch_size", c.batchSize),
	)

	// Start the flush timeout window when the first keys arrive in an empty buffer.
	// The timer is NOT reset on subsequent fetches — otherwise a steady trickle of
	// keys (few per poll) would keep pushing the deadline forward and the TIMEOUT
	// trigger would never fire, leaving partial batches stuck indefinitely.
	if wasEmpty {
		stopAndDrain(timer)
		timer.Reset(c.flushTimeout)
	}

	// SIZE trigger: buffer full → flush immediately and reset the timeout
	// window for the next batch cycle.
	if bufLen >= c.batchSize {
		c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: SIZE trigger fired, flushing now",
			libLog.Int("buffer", bufLen),
			libLog.Int("batch_size", c.batchSize),
		)
		c.flush(ctx)
		stopAndDrain(timer)
		timer.Reset(c.flushTimeout)
	}
}

// handleDrainingMode is entered when the buffer has items but the last fetch
// returned no new keys. It blocks on a select between three events:
//   - ctx.Done: shutdown requested, return and let flushRemaining handle the buffer
//   - timer.C: flush timeout elapsed (TIMEOUT trigger), flush the partial batch
//   - pollTimer.C: poll interval elapsed, return to the loop to try fetching again
//
// pollTimer is reusable across iterations (avoids time.After allocation leak).
func (c *BalanceSyncCollector) handleDrainingMode(ctx context.Context, bufLen int, timer *time.Timer, pollTimer *time.Timer) {
	stopAndDrain(pollTimer)
	pollTimer.Reset(c.pollInterval)

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		c.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncCollector: TIMEOUT trigger fired, flushing now",
			libLog.String("flush_timeout", c.flushTimeout.String()),
			libLog.Int("buffer", bufLen),
		)
		c.flush(ctx)
		timer.Reset(c.flushTimeout)
	case <-pollTimer.C:
		// No action — return to the main loop to try fetching again
	}
}

// handleIdleMode is entered when both the buffer and the ZSET are empty —
// there is nothing to flush and nothing to fetch. The flush timer is stopped
// (no point counting a timeout with an empty buffer) and the collector sleeps
// via waitForNext until either new keys arrive or shutdown is requested.
// Returns true if shutdown was requested during the wait.
func (c *BalanceSyncCollector) handleIdleMode(ctx context.Context, timer *time.Timer, waitForNext WaitForNextFunc) bool {
	c.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncCollector: idle mode, waiting for new keys")
	stopAndDrain(timer)

	if waitForNext(ctx) {
		return true // shutdown requested
	}

	c.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncCollector: woke up from idle, resuming polling")
	timer.Reset(c.flushTimeout)

	return false
}

// flushRemaining drains any leftover buffer on shutdown.
// ctx carries context values (tenant ID, PG connection) needed by the flush callback.
// The cancellation signal is stripped via context.WithoutCancel so the final flush
// can complete even after the parent context has been cancelled.
const shutdownFlushTimeout = 30 * time.Second

func (c *BalanceSyncCollector) flushRemaining(ctx context.Context) {
	c.mu.Lock()
	remaining := c.buffer
	c.buffer = make([]redisTransaction.SyncKey, 0, c.batchSize)
	c.mu.Unlock()

	if len(remaining) > 0 && c.flushFn != nil {
		// Use WithoutCancel to preserve context values (tenant ID, PG connection)
		// while removing the cancellation signal that already fired.
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownFlushTimeout)
		defer cancel()

		c.logger.Log(flushCtx, libLog.LevelInfo, "BalanceSyncCollector: shutdown — final flush",
			libLog.Int("remaining_keys", len(remaining)),
		)

		c.flushFn(flushCtx, remaining)
	}
}

// flush swaps the buffer for a fresh slice under the mutex, then calls the flush
// callback outside the lock. This swap-and-release pattern keeps the critical section
// short (no I/O under lock) while the callback may perform expensive work
// (Redis MGET, PostgreSQL batch update, conditional ZREM).
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

// stopAndDrain stops a timer and drains any stale event from its channel,
// making it safe to call Reset afterwards. This is necessary because
// time.Timer.Reset does NOT clear the channel — if the timer already fired
// but nobody read timer.C, the old event stays buffered and the next
// select { case <-timer.C } would fire immediately with the stale event
// instead of waiting for the new deadline. The select-with-default makes the
// drain non-blocking in case the channel is already empty.
func stopAndDrain(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
