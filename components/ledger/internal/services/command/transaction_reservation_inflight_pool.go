// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
)

// inFlightPool is the bounded, detached goroutine pool the reservation
// background paths run on. A counting semaphore caps concurrency, and every
// running transition is registered so a shutdown can name the ones it is about
// to abandon: the work is in-process, so a restart loses it, and losing it
// without a log line would hide a spend or a hold nobody settled.
type inFlightPool struct {
	slots chan struct{}
	wg    sync.WaitGroup

	inFlightMu sync.Mutex
	inFlight   map[uint64]reservationTransition
	nextSeq    uint64
}

// newInFlightPool builds a pool of maxInFlight slots; a non-positive bound
// means one slot, so a misconfigured pool still makes progress.
func newInFlightPool(maxInFlight int) *inFlightPool {
	if maxInFlight <= 0 {
		maxInFlight = 1
	}

	return &inFlightPool{
		slots:    make(chan struct{}, maxInFlight),
		inFlight: make(map[uint64]reservationTransition),
	}
}

// capacity is the number of slots the pool holds.
func (p *inFlightPool) capacity() int {
	return cap(p.slots)
}

// start runs fn for transition on a detached goroutine when a slot is free and
// reports whether it did. It never blocks: a full pool returns false and the
// caller decides how to report the drop. The context is detached from the
// request with context.WithoutCancel, which keeps the tenant and trace
// correlation while dropping the cancellation that fires once the response is
// written. The slot and the registration are released when fn returns, panic
// included.
func (p *inFlightPool) start(
	ctx context.Context,
	logger libLog.Logger,
	component, operation string,
	transition reservationTransition,
	fn func(context.Context),
) bool {
	select {
	case p.slots <- struct{}{}:
	default:
		return false
	}

	detached := context.WithoutCancel(ctx)

	p.wg.Add(1)

	untrack := p.track(transition)

	libRuntime.SafeGoWithContextAndComponent(detached, logger, component, operation, libRuntime.KeepRunning,
		func(c context.Context) {
			defer p.wg.Done()
			defer func() { <-p.slots }()
			defer untrack()

			fn(c)
		})

	return true
}

// track registers a transition as in flight and returns the function that
// deregisters it.
func (p *inFlightPool) track(transition reservationTransition) func() {
	p.inFlightMu.Lock()

	p.nextSeq++
	seq := p.nextSeq
	p.inFlight[seq] = transition

	p.inFlightMu.Unlock()

	return func() {
		p.inFlightMu.Lock()
		delete(p.inFlight, seq)
		p.inFlightMu.Unlock()
	}
}

// outstanding is a snapshot of every transition still running.
func (p *inFlightPool) outstanding() []reservationTransition {
	p.inFlightMu.Lock()
	defer p.inFlightMu.Unlock()

	snapshot := make([]reservationTransition, 0, len(p.inFlight))

	for _, transition := range p.inFlight {
		snapshot = append(snapshot, transition)
	}

	return snapshot
}
