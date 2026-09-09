// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// reservationRetryComponent scopes the retrier's panic-observability signals
// (panic_recovered_total, structured logs, span events) in dashboards.
const reservationRetryComponent = "ledger.tracer-reservation-retry"

// reservationRetryPolicy bounds how hard, and for how long, the ledger keeps
// trying to hand the tracer a confirm or release the first attempt could not
// deliver.
//
// Budget is sized against the DIRECT transaction's hold, which the tracer gives
// five minutes. Six minutes deliberately outlasts it, because a retry landing
// after the hold expired is not wasted: the tracer settles a late confirm by
// counting the spend without touching the capacity its expiry sweep already
// returned. The ledger would rather deliver the confirm late — the spend
// counted, the window logged — than stop trying because the hold is gone.
//
// It does NOT outlast a PENDING transaction's hold, and nothing sensible could:
// that one is thirty days by default (RESERVATION_LONG_LIVED_TTL_HOURS). Those
// are the transitions /commit and /cancel carry by transaction id, and if the
// budget runs out on one, the row simply stays RESERVED. That is the safe
// direction while it lasts — the capacity is still held, so the limit
// over-enforces rather than under-enforces — but it is not a fix: when the
// tracer's own sweep finally expires the row, a confirm that was never
// delivered becomes a spend that is never counted. Six minutes covers a tracer
// restart, which is the failure this retrier exists for; a pending transition
// lost for longer needs the durable store, not a longer timer.
type reservationRetryPolicy struct {
	// MaxAttempts is how many further attempts follow the inline one.
	MaxAttempts int
	// BaseDelay is the first backoff step; each attempt doubles it.
	BaseDelay time.Duration
	// MaxDelay caps one backoff step before jitter is applied.
	MaxDelay time.Duration
	// Budget is the wall-clock ceiling on the whole retry sequence.
	Budget time.Duration
	// MaxInFlight bounds concurrent retry sequences process-wide. It is the
	// backstop against a tracer outage turning every transaction into a
	// long-lived goroutine.
	MaxInFlight int
}

// defaultReservationRetryPolicy is the shipped budget: roughly six minutes of
// trying, spread over twenty attempts, with at most 256 sequences alive at once.
var defaultReservationRetryPolicy = reservationRetryPolicy{
	MaxAttempts: 20,
	BaseDelay:   250 * time.Millisecond,
	MaxDelay:    45 * time.Second,
	Budget:      6 * time.Minute,
	MaxInFlight: 256,
}

// reservationRetrier redelivers reservation transitions the inline attempt could
// not place, off the request path.
//
// It never blocks a caller: the transaction's money has already moved and the
// response is owed now, so the retry runs on a detached goroutine and the
// request returns regardless of whether the tracer is reachable. What the
// retrier guarantees is that a transient tracer — a restart, a rollout, a
// network blip, a 5xx — no longer costs the ledger a spend, and that the
// transitions it genuinely cannot place are named in the log instead of
// disappearing.
//
// Concurrency is capped by a counting semaphore rather than by a queue. A
// sequence holds its slot for as long as it keeps retrying, so under a total
// tracer outage the first MaxInFlight transitions are retried and every further
// one is reported immediately as undeliverable. That is a deliberate trade:
// bounded memory and a loud, honest log beats an unbounded goroutine population
// on the money path.
type reservationRetrier struct {
	policy reservationRetryPolicy
	slots  chan struct{}
	wg     sync.WaitGroup

	// inFlight names the transitions still being retried, so a shutdown can
	// report the ones it is about to abandon. Without it a rolling deploy drops
	// up to MaxInFlight confirms with no log line at all, which is the one hole
	// the "never lose a spend silently" contract cannot afford.
	inFlightMu sync.Mutex
	inFlight   map[uint64]reservationTransition
	nextSeq    uint64
}

// newReservationRetrier builds a retrier for the given policy.
func newReservationRetrier(policy reservationRetryPolicy) *reservationRetrier {
	if policy.MaxInFlight <= 0 {
		policy.MaxInFlight = 1
	}

	return &reservationRetrier{
		policy:   policy,
		slots:    make(chan struct{}, policy.MaxInFlight),
		inFlight: make(map[uint64]reservationTransition),
	}
}

// track registers a sequence as in flight and returns the function that
// deregisters it.
func (r *reservationRetrier) track(transition reservationTransition) func() {
	r.inFlightMu.Lock()

	r.nextSeq++
	seq := r.nextSeq
	r.inFlight[seq] = transition

	r.inFlightMu.Unlock()

	return func() {
		r.inFlightMu.Lock()
		delete(r.inFlight, seq)
		r.inFlightMu.Unlock()
	}
}

// reportOutstanding names every transition still being retried. It is called at
// the end of the graceful-drain window, when anything still here is about to die
// with the process: the retry is in-process, so a restart loses it. Losing it is
// a known residual; losing it WITHOUT a log line is not, because the whole point
// of this path is that a spend never goes missing quietly.
func (r *reservationRetrier) reportOutstanding(ctx context.Context, logger libLog.Logger) {
	r.inFlightMu.Lock()
	outstanding := make([]reservationTransition, 0, len(r.inFlight))

	for _, transition := range r.inFlight {
		outstanding = append(outstanding, transition)
	}

	r.inFlightMu.Unlock()

	if len(outstanding) == 0 {
		return
	}

	logger.Log(ctx, libLog.LevelError,
		"Shutting down with tracer reservation transitions still undelivered; they are abandoned here",
		libLog.Int("undelivered", len(outstanding)))

	for _, transition := range outstanding {
		logger.Log(ctx, libLog.LevelError,
			"Tracer reservation transition abandoned at shutdown; "+transition.lossConsequence(),
			transition.logFields())
	}
}

// sharedReservationRetrier is the process-wide retrier the transaction seams
// use. The concurrency cap is a property of the process, not of a request, so
// the semaphore is shared.
var sharedReservationRetrier = newReservationRetrier(defaultReservationRetryPolicy)

// scheduleReservationRetry hands a failed transition to the shared retrier. A
// nil reserver means the tracer integration is off and there is nothing to
// redeliver.
func (uc *UseCase) scheduleReservationRetry(ctx context.Context, logger libLog.Logger, transition reservationTransition, cause error) {
	if uc.TracerReserver == nil {
		return
	}

	sharedReservationRetrier.schedule(ctx, uc.TracerReserver, logger, transition, cause)
}

// schedule starts a retry sequence for one transition, or reports it as
// undeliverable when the process is already at its concurrency cap.
//
// The context is detached from the request with context.WithoutCancel: the
// request's context is cancelled the moment the response is written, and a
// confirm the ledger owes the tracer must outlive the request that created it.
// Detaching keeps the values the transport needs — the tenant the tracer client
// reads off the context, and the trace correlation — while dropping only the
// cancellation.
func (r *reservationRetrier) schedule(ctx context.Context, reserver TracerReserver, logger libLog.Logger, transition reservationTransition, cause error) {
	select {
	case r.slots <- struct{}{}:
	default:
		// At capacity. Say so instead of silently dropping: this is the one
		// place the ledger knowingly gives up on a spend, and an operator has
		// to see it as a saturation signal, not as a one-off.
		logger.Log(ctx, libLog.LevelError,
			"Tracer reservation transition dropped: too many retries already in flight; "+transition.lossConsequence(),
			append(transition.logFields(),
				libLog.Int("retries_in_flight", r.policy.MaxInFlight),
				libLog.Err(cause)))

		return
	}

	detached := context.WithoutCancel(ctx)

	r.wg.Add(1)

	untrack := r.track(transition)

	libRuntime.SafeGoWithContextAndComponent(detached, logger, reservationRetryComponent,
		"reservation.retry_transition", libRuntime.KeepRunning, func(c context.Context) {
			defer r.wg.Done()
			defer func() { <-r.slots }()
			defer untrack()

			c, cancel := context.WithTimeout(c, r.policy.Budget)
			defer cancel()

			r.run(c, reserver, logger, transition, cause)
		})
}

// run is the retry sequence for one transition. It returns as soon as the
// tracer accepts the transition, and otherwise keeps trying until the attempt
// count or the wall-clock budget runs out.
func (r *reservationRetrier) run(ctx context.Context, reserver TracerReserver, logger libLog.Logger, transition reservationTransition, cause error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reservation_retry")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.reservation.action", transition.Action),
		attribute.String("app.request.transaction_id", transition.TransactionID.String()),
	)

	lastErr := cause
	started := time.Now()

	for attempt := 1; attempt <= r.policy.MaxAttempts; attempt++ {
		if waitErr := libBackoff.WaitContext(ctx, r.delay(attempt)); waitErr != nil {
			r.reportExhausted(ctx, span, logger, transition, lastErr, attempt-1, started)

			return
		}

		err := r.deliver(ctx, reserver, transition)
		if err == nil {
			span.SetAttributes(attribute.Int("app.reservation.retry_attempts", attempt))

			// Worth a Warn rather than a Debug: the limit did not match reality
			// in the window between the failed inline attempt and this one, and
			// which way it was wrong depends on the action.
			logger.Log(ctx, libLog.LevelWarn,
				"Tracer reservation transition delivered on retry; "+transition.delayConsequence(),
				append(transition.logFields(),
					libLog.Int("attempts", attempt),
					libLog.String("elapsed", time.Since(started).String())))

			return
		}

		lastErr = err
	}

	r.reportExhausted(ctx, span, logger, transition, lastErr, r.policy.MaxAttempts, started)
}

// deliver makes one attempt, choosing the address the transition carries. Both
// forms are idempotent on the tracer's side, which is what makes retrying safe:
// a confirm only settles a reservation still in a settleable state, so a repeat
// after a response the ledger never saw moves no counter a second time.
func (r *reservationRetrier) deliver(ctx context.Context, reserver TracerReserver, transition reservationTransition) error {
	if transition.byTransaction() {
		if transition.Action == reservationActionRelease {
			return reserver.ReleaseByTransaction(ctx, transition.TransactionID)
		}

		return reserver.ConfirmByTransaction(ctx, transition.TransactionID)
	}

	if transition.Action == reservationActionRelease {
		return reserver.Release(ctx, transition.ReservationID)
	}

	return reserver.Confirm(ctx, transition.ReservationID)
}

// delay is the wait before the given attempt: exponential from BaseDelay,
// capped at MaxDelay before full jitter spreads a fleet's retries out.
func (r *reservationRetrier) delay(attempt int) time.Duration {
	step := libBackoff.Exponential(r.policy.BaseDelay, attempt-1)
	if r.policy.MaxDelay > 0 && step > r.policy.MaxDelay {
		step = r.policy.MaxDelay
	}

	return libBackoff.FullJitter(step)
}

// reportExhausted is the last word on a transition the ledger could not place.
// It is an Error, not a Warn, and it says which of the two failures happened:
// a confirm that never landed is money that moved and will never be counted, a
// release that never landed is capacity held against a transaction that moved
// nothing. They need opposite remediations, so the message names the direction
// rather than assuming the confirm case.
func (r *reservationRetrier) reportExhausted(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, cause error, attempts int, started time.Time) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" could not be delivered", cause)
	span.SetAttributes(attribute.Bool("app.reservation.retry_exhausted", true))

	logger.Log(ctx, libLog.LevelError,
		"Tracer reservation transition could not be delivered; "+transition.lossConsequence(),
		append(transition.logFields(),
			libLog.Int("attempts", attempts),
			libLog.String("elapsed", time.Since(started).String()),
			libLog.Err(cause)))
}

// ReportOutstandingReservationRetries names every confirm or release the ledger
// still owes the tracer, at the point the process is about to stop trying. The
// composition root calls it at the end of the graceful-drain window.
func ReportOutstandingReservationRetries(ctx context.Context, logger libLog.Logger) {
	sharedReservationRetrier.reportOutstanding(ctx, logger)
}
