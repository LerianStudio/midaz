// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// unansweredSettleComponent scopes the queue's panic-observability signals.
const unansweredSettleComponent = "ledger.tracer-unanswered-settle"

// tracerReserveLockWait is the tracer's reserve lock_timeout: the longest a
// reserve that outlived its caller can still wait on its scope lock before it
// commits or gives up. It mirrors the tracer's reserveLockTimeout and must
// change with it.
const tracerReserveLockWait = 3 * time.Second

// unansweredSettleAttempts is how many times one unanswered settle is offered:
// once after the delay, and once more if the first attempt settled nothing.
const unansweredSettleAttempts = 2

// unansweredSettlePolicy bounds the dedicated queue that settles unanswered
// reserves.
type unansweredSettlePolicy struct {
	// MaxInFlight bounds concurrent settles process-wide. A full queue drops
	// the settle rather than block the request or borrow the shared retrier.
	MaxInFlight int
}

// defaultUnansweredSettlePolicy is the shipped bound: a quarter of the shared
// retrier's, because an unanswered settle is a best-effort correction and must
// never crowd out a confirm the tracer is known to owe.
var defaultUnansweredSettlePolicy = unansweredSettlePolicy{MaxInFlight: 64}

// unansweredSettleQueue settles, by transaction, the reserves that went
// unanswered. It is deliberately separate from the shared reservation retrier:
// an unanswered reserve most often means the tracer is down, and every
// transaction then produces one, so on the shared retrier they would saturate
// the pool and push out the by-id confirms of movements that did apply.
//
// Each settle waits a fixed delay before every attempt, long enough for a
// reserve still running on the tracer to finish, so the settle does not race
// the hold it is meant to settle.
type unansweredSettleQueue struct {
	*inFlightPool

	// wait blocks for one delay; tests replace it so no test sleeps.
	wait func(ctx context.Context, delay time.Duration) error
}

// newUnansweredSettleQueue builds a queue for the given policy.
func newUnansweredSettleQueue(policy unansweredSettlePolicy) *unansweredSettleQueue {
	return &unansweredSettleQueue{
		inFlightPool: newInFlightPool(policy.MaxInFlight),
		wait:         libBackoff.WaitContext,
	}
}

// sharedUnansweredSettleQueue is the process-wide queue; like the retrier's
// semaphore, its bound is a property of the process.
var sharedUnansweredSettleQueue = newUnansweredSettleQueue(defaultUnansweredSettlePolicy)

// scheduleUnansweredSettle hands an unanswered reserve's settle to the dedicated
// queue and returns at once. The request span records that it was scheduled.
func (uc *UseCase) scheduleUnansweredSettle(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle, action string) {
	if uc.TracerReserver == nil {
		return
	}

	span.AddEvent(unansweredSettleSpanEvent, trace.WithAttributes(
		attribute.String("app.reservation.action", action),
	))

	uc.unansweredSettleQueue().schedule(ctx, uc.TracerReserver, uc.MetricsFactory, logger,
		handle.transitionByTransaction(action), uc.unansweredSettleDelay())
}

// unansweredSettleQueue returns the queue this use case settles through.
func (uc *UseCase) unansweredSettleQueue() *unansweredSettleQueue {
	if uc.unansweredSettles != nil {
		return uc.unansweredSettles
	}

	return sharedUnansweredSettleQueue
}

// unansweredSettleDelay is the wait before each attempt: the tracer's reserve
// lock wait plus the client timeout that bounded the reserve, which is the
// longest the unanswered reserve can still be running on the tracer.
func (uc *UseCase) unansweredSettleDelay() time.Duration {
	clientTimeout := uc.TracerClientTimeout
	if clientTimeout <= 0 {
		clientTimeout = tracer.DefaultOperationTimeout
	}

	return tracerReserveLockWait + clientTimeout
}

// schedule starts the settle for one transition, or drops it with a Warn when
// the queue is full. The context is detached from the request so the settle
// outlives the response while keeping the tenant and trace correlation.
func (q *unansweredSettleQueue) schedule(
	ctx context.Context,
	reserver TracerReserver,
	factory *metrics.MetricsFactory,
	logger libLog.Logger,
	transition reservationTransition,
	delay time.Duration,
) {
	started := q.start(ctx, logger, unansweredSettleComponent, "reservation.unanswered_settle", transition,
		func(c context.Context) {
			q.run(c, reserver, factory, logger, transition, delay)
		})
	if started {
		return
	}

	logger.Log(ctx, libLog.LevelWarn,
		"Unanswered tracer reservation settle dropped: too many settles already queued",
		libLog.String("reservation_action", transition.Action),
		libLog.String("transaction_id", transition.TransactionID.String()))
}

// run offers the settle up to unansweredSettleAttempts times, each after the
// fixed delay. A confirm is offered again when it failed or settled nothing; a
// release only when it failed. What is still unsettled after the last attempt
// is reported by transaction id alone.
func (q *unansweredSettleQueue) run(
	ctx context.Context,
	reserver TracerReserver,
	factory *metrics.MetricsFactory,
	logger libLog.Logger,
	transition reservationTransition,
	delay time.Duration,
) {
	_, settleTracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := settleTracer.Start(ctx, "command.reservation_unanswered_settle")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.reservation.action", transition.Action),
		attribute.String("app.request.transaction_id", transition.TransactionID.String()),
	)

	var lastErr error

	for attempt := 1; attempt <= unansweredSettleAttempts; attempt++ {
		if err := q.wait(ctx, delay); err != nil {
			lastErr = err

			break
		}

		settled, err := q.attempt(ctx, span, reserver, factory, logger, transition)
		if settled {
			span.SetAttributes(attribute.Int("app.reservation.settle_attempts", attempt))

			return
		}

		if err == nil && lastErr != nil && transition.Action != reservationActionRelease {
			q.reportPossiblySettled(ctx, logger, transition)

			return
		}

		lastErr = err
	}

	q.reportUnsettled(ctx, span, logger, transition, lastErr)
}

// attempt makes one settle attempt and reports whether it is final. A confirm
// is final once the tracer settled a reservation or found one released; a
// release is final once the tracer accepted it.
func (q *unansweredSettleQueue) attempt(
	ctx context.Context,
	span trace.Span,
	reserver TracerReserver,
	factory *metrics.MetricsFactory,
	logger libLog.Logger,
	transition reservationTransition,
) (bool, error) {
	if transition.Action == reservationActionRelease {
		err := reserver.ReleaseByTransaction(ctx, transition.TransactionID)

		return err == nil, err
	}

	outcome, err := reserver.ConfirmByTransaction(ctx, transition.TransactionID)
	if err != nil {
		return false, err
	}

	recordReservationConfirmOutcome(ctx, span, factory, logger, transition, outcome)

	return outcome.Confirmed > 0 || outcome.AlreadyReleased > 0, nil
}

// reportPossiblySettled closes a confirm whose earlier attempt failed on the
// transport and whose later attempt found nothing to settle. The failed attempt
// may have reached the tracer and confirmed the hold, and the confirm reply
// cannot tell an already-confirmed hold from a missing one, so this is a Debug,
// not the Warn of a settle that found nothing.
func (q *unansweredSettleQueue) reportPossiblySettled(ctx context.Context, logger libLog.Logger, transition reservationTransition) {
	logger.Log(ctx, libLog.LevelDebug,
		"Unanswered tracer reservation confirm found nothing after a transport failure; the failed attempt may have settled it",
		libLog.String("transaction_id", transition.TransactionID.String()))
}

// reportUnsettled is the last word on an unanswered settle. It is a Warn, not an
// Error: the reserve may never have reached the tracer, in which case there was
// nothing to settle. It names the transaction only.
func (q *unansweredSettleQueue) reportUnsettled(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, cause error) {
	message := "No reservation settled for the transaction after an unanswered reserve"
	if transition.Action == reservationActionRelease {
		message = "Unanswered tracer reservation release could not be delivered"
	}

	fields := []any{libLog.String("transaction_id", transition.TransactionID.String())}

	if cause != nil {
		libOpentelemetry.HandleSpanError(span, message, cause)

		fields = append(fields, libLog.Err(cause))
	}

	logger.Log(ctx, libLog.LevelWarn, message, fields...)
}

// reportOutstanding names, by transaction id only, every unanswered settle the
// process is about to abandon at shutdown.
func (q *unansweredSettleQueue) reportOutstanding(ctx context.Context, logger libLog.Logger) {
	for _, transition := range q.outstanding() {
		logger.Log(ctx, libLog.LevelWarn,
			"Unanswered tracer reservation settle abandoned at shutdown",
			libLog.String("reservation_action", transition.Action),
			libLog.String("transaction_id", transition.TransactionID.String()))
	}
}
