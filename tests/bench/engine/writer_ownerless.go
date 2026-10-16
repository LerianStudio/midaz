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

// ownerlessWriter feeds prepared batchers, one goroutine each, from one bounded queue. It owns only those goroutines:
// the caller closes the batchers' sessions and outbox after Close returns.
type ownerlessWriter struct {
	queue     chan attempt
	now       func() time.Time // the batchers' clock
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

// workerStats is one worker's batches, their passes and deadlock retries, and its first batch error.
type workerStats struct {
	batches int
	batchStats
	err error
}

// newOwnerlessWriter starts one worker per batcher under ctx, the writer's lifetime, never a caller's. Each worker
// takes up to maxBatch attempts already queued, without waiting to coalesce more.
func newOwnerlessWriter(ctx context.Context, batchers []*batcher, maxBatch int) *ownerlessWriter {
	lifetime, abort := context.WithCancelCause(ctx)
	g, gctx := errgroup.WithContext(lifetime)

	w := &ownerlessWriter{
		queue: make(chan attempt, queueDepth), now: batchers[0].now, maxBatch: maxBatch,
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
// for Close; returning it would cancel the other workers' commits.
func (w *ownerlessWriter) work(ctx context.Context, stats *workerStats, b *batcher) error {
	batch, reqs := make([]attempt, 0, w.maxBatch), make([]request, 0, w.maxBatch)

	for first := range w.queue {
		batch, reqs = w.take(append(batch[:0], first)), reqs[:0]
		for _, a := range batch {
			reqs = append(reqs, a.req)
		}

		res, run, err := b.run(ctx, reqs)
		stats.batches, stats.passes, stats.deadlocks = stats.batches+1, stats.passes+run.passes, stats.deadlocks+run.deadlocks

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

	return nil
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
