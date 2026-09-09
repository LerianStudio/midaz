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
// Budget is deliberately LONGER than the tracer's default reservation lifetime
// (five minutes for a direct transaction). A retry landing after the hold has
// already expired is not wasted: the tracer settles a late confirm by counting
// the spend without touching the capacity its expiry sweep already returned. So
// the ledger would rather deliver the confirm late — the spend counted, the
// window logged — than stop trying because the hold is gone.
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
}

// newReservationRetrier builds a retrier for the given policy.
func newReservationRetrier(policy reservationRetryPolicy) *reservationRetrier {
	if policy.MaxInFlight <= 0 {
		policy.MaxInFlight = 1
	}

	return &reservationRetrier{
		policy: policy,
		slots:  make(chan struct{}, policy.MaxInFlight),
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
			"Tracer reservation transition dropped: too many retries already in flight; the spend will not be counted against the limit",
			append(transition.logFields(),
				libLog.Int("retries_in_flight", r.policy.MaxInFlight),
				libLog.Err(cause)))

		return
	}

	detached := context.WithoutCancel(ctx)

	r.wg.Add(1)

	libRuntime.SafeGoWithContextAndComponent(detached, logger, reservationRetryComponent,
		"reservation.retry_transition", libRuntime.KeepRunning, func(c context.Context) {
			defer r.wg.Done()
			defer func() { <-r.slots }()

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

			// Worth a Warn rather than a Debug: between the failed inline
			// attempt and this one the limit was under-enforced, because the
			// capacity was held (or, past the expiry, already returned) while
			// the spend went uncounted.
			logger.Log(ctx, libLog.LevelWarn,
				"Tracer reservation transition delivered on retry; the limit was under-enforced until now",
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
// It is an Error, not a Warn: for a confirm it means a transaction whose money
// moved will never be counted against the customer's spending limit, so the
// limit under-enforces until someone acts on this line.
func (r *reservationRetrier) reportExhausted(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, cause error, attempts int, started time.Time) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" could not be delivered", cause)
	span.SetAttributes(attribute.Bool("app.reservation.retry_exhausted", true))

	logger.Log(ctx, libLog.LevelError,
		"Tracer reservation transition could not be delivered; the spend will not be counted against the limit",
		append(transition.logFields(),
			libLog.Int("attempts", attempts),
			libLog.String("elapsed", time.Since(started).String()),
			libLog.Err(cause)))
}
