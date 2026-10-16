//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
)

// queueDepth bounds the attempts waiting for a worker: one fixed value for every compared writer and cell.
const queueDepth = 8192

var errWriterClosed = errors.New("writer closed")

// ownerlessWriter feeds prepared batchers, one goroutine each, from one bounded queue; an owner is its one fenced
// batcher. It owns only those goroutines: the caller closes the batchers' sessions and outbox after Close returns.
type ownerlessWriter struct {
	queue     chan attempt
	now       func() time.Time // the batchers' clock
	startedAt time.Time        // now before the workers start: each batch records its take as an offset from it
	maxBatch  int
	abort     context.CancelCauseFunc // ends every batch in flight
	g         *errgroup.Group
	workers   []workerStats // one slot per worker, read after the join
	entryLate atomic.Int64  // attempts already past their admission cutoff when submitted, queued or not
	closed    atomic.Bool
}

// attempt is one Submit; reply holds its answer, so a worker never waits on a caller that left.
type attempt struct {
	req   request
	reply chan result
}

// workerStats is one worker's batches as taken, their passes and deadlock retries, and its first batch error.
type workerStats struct {
	taken []batchTaken
	batchStats
	err error
}

// batchTaken is one batch as its worker took it from the queue, before running it: the instant as an offset from the
// writer's start, pointer-free so the GC never scans the records, and its attempts.
type batchTaken struct {
	at       time.Duration
	attempts int
}

// newOwnerlessWriter starts one worker per batcher under ctx, the writer's lifetime, never a caller's. Each worker
// takes up to maxBatch attempts already queued, without waiting to coalesce more.
func newOwnerlessWriter(ctx context.Context, batchers []*batcher, maxBatch int) *ownerlessWriter {
	lifetime, abort := context.WithCancelCause(ctx)
	g, gctx := errgroup.WithContext(lifetime)

	w := &ownerlessWriter{
		queue: make(chan attempt, queueDepth), now: batchers[0].now, startedAt: batchers[0].now(), maxBatch: maxBatch,
		abort: abort, g: g, workers: make([]workerStats, len(batchers)),
	}

	for i, b := range batchers {
		g.Go(func() error { return w.work(gctx, &w.workers[i], b) })
	}

	return w
}

// Submit waits up to req.deadline on the batchers' clock. A caller that leaves, or a deadline that passes, answers
// unknown: the attempt is not queued, or stays accepted and may still commit. An attempt late at entry is counted
// before admission, queued or not; only SQL proves a new identity before expiring it.
func (w *ownerlessWriter) Submit(ctx context.Context, req request) result {
	now := w.now()
	if !req.admissible(now) {
		w.entryLate.Add(1)
	}

	if w.closed.Load() {
		return result{requestID: req.id, outcome: outcomeUnknown, reason: errWriterClosed.Error()}
	}

	ctx, cancel := context.WithTimeout(ctx, req.deadline.Sub(now))
	defer cancel()

	a := attempt{req: req, reply: make(chan result, 1)}

	if ctx.Err() == nil {
		select {
		case w.queue <- a:
			select {
			case res := <-a.reply:
				return res
			case <-ctx.Done():
			}
		case <-ctx.Done():
		}
	}

	select {
	case res := <-a.reply: // an answer that raced the caller's exit wins
		return res
	default:
		return result{requestID: req.id, outcome: outcomeUnknown, reason: context.Cause(ctx).Error()}
	}
}

// work runs batches until the queue is closed and drained. A batch error answers its attempts unknown and is kept
// for Close, as is an owner lost while idle; returning either would cancel the other workers' commits.
func (w *ownerlessWriter) work(ctx context.Context, stats *workerStats, b *batcher) error {
	batch, reqs := make([]attempt, 0, w.maxBatch), make([]request, 0, w.maxBatch)

	for {
		first, ok := w.next(ctx, b)
		if !ok {
			if b.owner != nil && stats.err == nil {
				stats.err = b.owner.err
			}

			return nil
		}

		batch, reqs = w.take(append(batch[:0], first)), reqs[:0]
		stats.taken = append(stats.taken, batchTaken{w.now().Sub(w.startedAt), len(batch)})

		for _, a := range batch {
			reqs = append(reqs, a.req)
		}

		res, run, err := b.run(ctx, reqs)
		stats.passes, stats.deadlocks = stats.passes+run.passes, stats.deadlocks+run.deadlocks

		if err != nil && stats.err == nil {
			stats.err = err
		}

		for i, a := range batch {
			if err != nil {
				res[i].reason = err.Error()
			}

			a.reply <- res[i]
		}
	}
}

// next waits for a queued attempt, renewing an owner's fence on each tick meanwhile. It returns false once the
// queue is closed and drained, and stops the owner's ticks then.
func (w *ownerlessWriter) next(ctx context.Context, b *batcher) (attempt, bool) {
	var ticks <-chan time.Time // nil without an owner: never ready
	if b.owner != nil {
		ticks = b.owner.ticker.C
	}

	for {
		select {
		case <-ticks:
			b.owner.renew(ctx, b.sess)
		case a, ok := <-w.queue:
			if !ok && b.owner != nil {
				b.owner.ticker.Stop()
			}

			return a, ok
		}
	}
}

// take appends attempts already queued to batch, up to maxBatch.
func (w *ownerlessWriter) take(batch []attempt) []attempt {
	for len(batch) < w.maxBatch {
		select {
		case a, ok := <-w.queue:
			if !ok {
				return batch
			}

			batch = append(batch, a)
		default:
			return batch
		}
	}

	return batch
}

// Close stops admission and waits until every accepted attempt is answered. Only ctx, or the writer's lifetime,
// ending first aborts the batches in flight, whose answers stay unknown, never proof of rollback. Called once, after
// every Submit returned.
func (w *ownerlessWriter) Close(ctx context.Context) error {
	defer w.abort(errWriterClosed)

	w.closed.Store(true)
	close(w.queue)

	stop := context.AfterFunc(ctx, func() { w.abort(context.Cause(ctx)) })
	errs := []error{w.g.Wait()}

	if !stop() {
		errs = append(errs, fmt.Errorf("close aborted the writer: %w", context.Cause(ctx)))
	}

	for i, s := range w.workers {
		if s.err != nil {
			errs = append(errs, fmt.Errorf("worker %d: %w", i, s.err))
		}
	}

	return errors.Join(errs...)
}

// occupancy is the batches taken in a window, their attempts and the largest: dequeued work, failed batches
// included, never approvals or time spent running.
type occupancy struct{ batches, attempts, largest int }

// occupancy sums every worker's batches taken in [from, to) on the batchers' clock. Call it after Close joined the
// workers.
func (w *ownerlessWriter) occupancy(from, to time.Time) occupancy {
	var o occupancy

	lo, hi := from.Sub(w.startedAt), to.Sub(w.startedAt)

	for _, s := range w.workers {
		for _, b := range s.taken {
			if b.at >= lo && b.at < hi {
				o.batches, o.attempts, o.largest = o.batches+1, o.attempts+b.attempts, max(o.largest, b.attempts)
			}
		}
	}

	return o
}
