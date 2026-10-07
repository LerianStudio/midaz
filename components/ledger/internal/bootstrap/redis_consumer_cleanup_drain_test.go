// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	transaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

// drainQueue serves a scripted sequence of cleanup pass results.
type drainQueue struct {
	transaction.RedisRepository
	results []transaction.RecoveryCleanupResult
	errAt   int
	calls   int
	limits  []int
}

func (queue *drainQueue) CleanupEngineRecovery(_ context.Context, _ time.Time, limit int) (transaction.RecoveryCleanupResult, error) {
	call := queue.calls
	queue.calls++
	queue.limits = append(queue.limits, limit)

	if queue.errAt > 0 && call+1 == queue.errAt {
		return transaction.RecoveryCleanupResult{}, errors.New("cleanup failed")
	}

	if call < len(queue.results) {
		return queue.results[call], nil
	}

	return transaction.RecoveryCleanupResult{}, nil
}

func fullCleanedPage() transaction.RecoveryCleanupResult {
	return transaction.RecoveryCleanupResult{Scanned: recoveryCleanupBatchSize, Cleaned: recoveryCleanupBatchSize}
}

func newDrainCompleter(queue transaction.RedisRepository, clock func() time.Time) *recoveryRecordCompleter {
	return (&RedisQueueConsumer{Logger: recoveryQuietLogger{}, queue: queue}).
		WithRecoveryClock(clock).
		newRecoveryRecordCompleter()
}

func fixedDrainClock() func() time.Time {
	fixed := time.Date(2042, time.May, 6, 7, 8, 9, 0, time.UTC)
	return func() time.Time { return fixed }
}

func TestRecoveryCleanupDrainsFullPagesUntilShortPage(t *testing.T) {
	queue := &drainQueue{results: []transaction.RecoveryCleanupResult{
		fullCleanedPage(),
		fullCleanedPage(),
		{Scanned: 7, Cleaned: 7},
	}}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(t.Context())

	require.Equal(t, 3, queue.calls)
	for _, limit := range queue.limits {
		require.Equal(t, recoveryCleanupBatchSize, limit)
	}
}

func TestRecoveryCleanupCountsRescheduledAndStaleAsProgress(t *testing.T) {
	queue := &drainQueue{results: []transaction.RecoveryCleanupResult{
		{Scanned: recoveryCleanupBatchSize, Rescheduled: recoveryCleanupBatchSize},
		{Scanned: recoveryCleanupBatchSize, Stale: recoveryCleanupBatchSize},
		{},
	}}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(t.Context())

	require.Equal(t, 3, queue.calls)
}

func TestRecoveryCleanupStopsOnFullPageWithoutProgress(t *testing.T) {
	queue := &drainQueue{results: []transaction.RecoveryCleanupResult{
		fullCleanedPage(),
		{Scanned: recoveryCleanupBatchSize},
		fullCleanedPage(),
	}}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(t.Context())

	require.Equal(t, 2, queue.calls)
}

func TestRecoveryCleanupStopsOnError(t *testing.T) {
	queue := &drainQueue{
		results: []transaction.RecoveryCleanupResult{fullCleanedPage(), fullCleanedPage(), fullCleanedPage()},
		errAt:   2,
	}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(t.Context())

	require.Equal(t, 2, queue.calls)
}

func TestRecoveryCleanupStopsAtMaxPasses(t *testing.T) {
	results := make([]transaction.RecoveryCleanupResult, recoveryCleanupMaxPasses+10)
	for index := range results {
		results[index] = fullCleanedPage()
	}

	queue := &drainQueue{results: results}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(t.Context())

	require.Equal(t, recoveryCleanupMaxPasses, queue.calls)
}

func TestRecoveryCleanupStopsWhenDrainBudgetElapses(t *testing.T) {
	start := time.Date(2042, time.June, 7, 8, 9, 10, 0, time.UTC)
	ticks := 0
	// Each clock read advances 30s: the deadline read, then one read per pass.
	clock := func() time.Time {
		current := start.Add(time.Duration(ticks) * 30 * time.Second)
		ticks++

		return current
	}

	results := make([]transaction.RecoveryCleanupResult, 20)
	for index := range results {
		results[index] = fullCleanedPage()
	}

	queue := &drainQueue{results: results}

	newDrainCompleter(queue, clock).cleanup(t.Context())

	// Deadline = start+2m. Passes run at +30s, +60s, +90s; the read at +120s stops.
	require.Equal(t, 3, queue.calls)
}

func TestRecoveryCleanupStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	queue := &drainQueue{results: []transaction.RecoveryCleanupResult{fullCleanedPage()}}

	newDrainCompleter(queue, fixedDrainClock()).cleanup(ctx)

	require.Equal(t, 0, queue.calls)
}
