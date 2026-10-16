//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type mix int

const (
	mixOutflow mix = iota // debit hot, credit a random ordinary balance
	mixInflow             // the reverse
	mixMixed              // either direction, drawn per request
)

// sampleEvery paces the open loop's in-flight samples.
const sampleEvery = time.Second

// workload derives each request from its run label and index alone, so warm-up and measurement use distinct labels.
type workload struct {
	run      string
	seed     uint64
	mix      mix
	feeLeg   bool
	ordinary int
	hot, fee posting
}

func newWorkload(run string, m mix, feeLeg bool, ordinary int) (*workload, error) {
	if run == "" || ordinary < 1 {
		return nil, fmt.Errorf("workload run %q over %d ordinary balances: want a label and a seeded balance", run, ordinary)
	}

	seed := uuid.NewSHA1(benchNS, []byte("workload/"+run))

	return &workload{
		run: run, seed: binary.BigEndian.Uint64(seed[:8]), mix: m, feeLeg: feeLeg, ordinary: ordinary,
		hot: fixturePosting(classHot, 0), fee: fixturePosting(classFee, 0),
	}, nil
}

func fixturePosting(class string, i int) posting {
	return posting{BalanceID: balanceID(class, i, constant.DefaultBalanceKey), AccountID: accountID(class, i)}
}

// request builds index i. Each call mints a new transaction ID, so building a run's index again conflicts under the
// same request ID and never pays twice; a retry resends the first request.
func (w *workload) request(i int64, scheduledAt time.Time, b budget) (request, error) {
	draw := rand.New(rand.NewPCG(w.seed, uint64(i)))

	low := int64(1)
	if w.feeLeg {
		low = 2 // the fee takes 1 of the credit, which keeps at least 1
	}

	amount := low + draw.Int64N(101-low)

	debit, credit := w.hot, fixturePosting(classOrdinary, draw.IntN(w.ordinary))
	if w.mix == mixInflow || (w.mix == mixMixed && draw.IntN(2) == 0) {
		debit, credit = credit, debit
	}

	debit.Amount, credit.Amount = -amount, amount
	postings := []posting{debit, credit}

	if w.feeLeg {
		fee := w.fee
		fee.Amount = 1
		postings[1].Amount--
		postings = append(postings, fee)
	}

	txID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return request{}, fmt.Errorf("transaction id: %w", err)
	}

	return newRequest(uuid.NewSHA1(benchNS, fmt.Appendf(nil, "workload/%s/request/%d", w.run, i)), txID, postings, scheduledAt, b)
}

// scheduleOf is index i's instant on the absolute schedule start + i/rate, exact in nanoseconds below i = 9.2e9.
func scheduleOf(start time.Time, i, rate int64) time.Time {
	return start.Add(time.Duration(i * int64(time.Second) / rate))
}

// due returns the end of the run of indices from next, capped at n, scheduled at or before now.
func due(start time.Time, rate, next, n int64, now time.Time) int64 {
	for next < n && !scheduleOf(start, next, rate).After(now) {
		next++
	}

	return next
}

// runOpen sends index i of n at start+i/rate, one goroutine per attempt. A late wake sends every due index with
// its own schedule, so lateness shows as lag. Cancelling ctx stops dispatch; attempts in flight still answer.
func runOpen(ctx context.Context, w writer, wl *workload, start time.Time, rate, n int64, b budget, rec *recorder) error {
	if rate < 1 || n < 0 {
		return fmt.Errorf("open loop at %d/s for %d requests: want a positive rate and a non-negative count", rate, n)
	}

	rec.begin(n, start)

	g, gctx := errgroup.WithContext(ctx)
	ticker := time.NewTicker(sampleEvery)
	wake := time.NewTimer(0)

	defer ticker.Stop()
	defer wake.Stop()

	var err error

	for next := int64(0); next < n && err == nil && gctx.Err() == nil; {
		for end := due(start, rate, next, n, time.Now()); next < end && err == nil && gctx.Err() == nil; next++ {
			var req request
			if req, err = wl.request(next, scheduleOf(start, next, rate), b); err == nil {
				rec.dispatch(req.scheduledAt, time.Now())
				g.Go(func() error {
					rec.observe(req, w.Submit(gctx, req), time.Now())
					return nil
				})
			}
		}

		if next < n {
			wake.Reset(time.Until(scheduleOf(start, next, rate)))

			select {
			case <-gctx.Done():
			case <-ticker.C:
				rec.sample()
			case <-wake.C:
			}
		}
	}

	rec.sample()

	return errors.Join(err, g.Wait(), rec.lateErr(), context.Cause(ctx))
}

// runClosed runs producers that each send one request, wait for its answer, then take the next shared index.
func runClosed(ctx context.Context, w writer, wl *workload, producers int, n int64, b budget, rec *recorder) error {
	if producers < 1 || n < 0 {
		return fmt.Errorf("closed loop with %d producers for %d requests: want a producer and a non-negative count", producers, n)
	}

	rec.begin(n, time.Now())

	g, gctx := errgroup.WithContext(ctx)

	var next atomic.Int64

	for range producers {
		g.Go(func() error {
			for i := next.Add(1) - 1; i < n && gctx.Err() == nil; i = next.Add(1) - 1 {
				req, err := wl.request(i, time.Now(), b)
				if err != nil {
					return err
				}

				rec.dispatch(req.scheduledAt, time.Now())
				rec.observe(req, w.Submit(gctx, req), time.Now())
			}

			return nil
		})
	}

	return errors.Join(g.Wait(), rec.lateErr(), context.Cause(ctx))
}
