// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// settleWaits records every delay the queue asked for and, when gated, holds
// each wait until the gate closes. It stands in for the clock so no test sleeps.
type settleWaits struct {
	mu     sync.Mutex
	delays []time.Duration
	gate   <-chan struct{}
}

func (w *settleWaits) wait(ctx context.Context, delay time.Duration) error {
	w.mu.Lock()
	w.delays = append(w.delays, delay)
	w.mu.Unlock()

	if w.gate != nil {
		select {
		case <-w.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return ctx.Err()
}

func (w *settleWaits) requested() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	return append([]time.Duration(nil), w.delays...)
}

// drain blocks until every scheduled settle has finished. Only tests join on a
// settle; production never does, which is why it runs off the request path.
func (q *unansweredSettleQueue) drain() {
	q.wg.Wait()
}

// newTestUnansweredSettleQueue builds a dedicated queue whose waits are recorded
// and, with a non-nil gate, held until the gate closes. Cleanup drains it.
func newTestUnansweredSettleQueue(t *testing.T, maxInFlight int, gate <-chan struct{}) (*unansweredSettleQueue, *settleWaits) {
	t.Helper()

	waits := &settleWaits{gate: gate}
	queue := newUnansweredSettleQueue(unansweredSettlePolicy{MaxInFlight: maxInFlight})
	queue.wait = waits.wait

	t.Cleanup(queue.drain)

	return queue, waits
}

// withImmediateUnansweredSettles gives uc a dedicated queue that settles without
// waiting, so a test can drain it and assert on the settle.
func withImmediateUnansweredSettles(t *testing.T, uc *UseCase) *unansweredSettleQueue {
	t.Helper()

	queue, _ := newTestUnansweredSettleQueue(t, defaultUnansweredSettlePolicy.MaxInFlight, nil)
	uc.unansweredSettles = queue

	return queue
}

// settleAnswer is one scripted by-transaction confirm answer.
type settleAnswer struct {
	outcome tracer.ConfirmOutcome
	err     error
}

// sequenceReserver answers each by-transaction confirm and release from a
// script, in call order; past the script it answers the last entry again.
type sequenceReserver struct {
	stubReserver

	confirmAnswers []settleAnswer
	releaseErrs    []error
}

func (r *sequenceReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (tracer.ConfirmOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := min(len(r.confirmedTxns), len(r.confirmAnswers)-1)
	r.confirmedTxns = append(r.confirmedTxns, transactionID)

	return r.confirmAnswers[index].outcome, r.confirmAnswers[index].err
}

func (r *sequenceReserver) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := len(r.releasedTxns)
	r.releasedTxns = append(r.releasedTxns, transactionID)

	if index >= len(r.releaseErrs) {
		return nil
	}

	return r.releaseErrs[index]
}

// unansweredTestHandle is an unanswered handle whose amount cannot occur inside a
// uuid, so a test can prove the amount is never logged.
func unansweredTestHandle() reservationHandle {
	return reservationHandle{
		TransactionID: uuid.New(),
		Amount:        decimal.RequireFromString("1234.56"),
		Asset:         "BRL",
		Unanswered:    true,
	}
}

var errSettleTransport = fmt.Errorf("settle attempt: %w", tracer.ErrTracerUnavailable)

func TestUnansweredSettle_WaitsTheFixedDelayOffTheRequestPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		clientTimeout time.Duration
		wantDelay     time.Duration
	}{
		{name: "the default client timeout", wantDelay: tracerReserveLockWait + tracer.DefaultOperationTimeout},
		{name: "a configured client timeout", clientTimeout: 400 * time.Millisecond, wantDelay: tracerReserveLockWait + 400*time.Millisecond},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gate := make(chan struct{})
			queue, waits := newTestUnansweredSettleQueue(t, 4, gate)
			reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
			uc := &UseCase{TracerReserver: reserver, TracerClientTimeout: tc.clientTimeout, unansweredSettles: queue}

			recorder := tracetest.NewSpanRecorder()
			ctx := context.Background()
			_, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("t").Start(ctx, "request")

			handle := unansweredTestHandle()
			uc.confirmReservations(ctx, span, &libLog.NopLogger{}, handle)
			span.End()

			assert.Empty(t, reserver.confirmedTransactions(), "the request returns before the settle is attempted")

			var scheduled bool

			for _, ended := range recorder.Ended() {
				for _, event := range ended.Events() {
					scheduled = scheduled || event.Name == unansweredSettleSpanEvent
				}
			}

			assert.True(t, scheduled, "the request span records that a settle was scheduled")

			close(gate)
			queue.drain()

			assert.Equal(t, []uuid.UUID{handle.TransactionID}, reserver.confirmedTransactions())
			assert.Equal(t, []time.Duration{tc.wantDelay}, waits.requested(),
				"the settle waits out the tracer lock wait plus the client timeout before its first attempt")
		})
	}
}

func TestUnansweredConfirm_SecondAttemptOnlyWhenNothingWasSettled(t *testing.T) {
	t.Parallel()

	nothing := settleAnswer{}
	counted := settleAnswer{outcome: tracer.ConfirmOutcome{Confirmed: 1}}
	released := settleAnswer{outcome: tracer.ConfirmOutcome{AlreadyReleased: 1}}
	failed := settleAnswer{err: errSettleTransport}

	cases := []struct {
		name      string
		answers   []settleAnswer
		wantCalls int
		wantWarn  bool
		wantDebug bool
	}{
		{name: "a confirm that counted the spend is final", answers: []settleAnswer{counted}, wantCalls: 1},
		{name: "nothing found after a transport failure is only a Debug", answers: []settleAnswer{failed, nothing}, wantCalls: 2, wantDebug: true},
		{name: "a confirm that found the hold released is final", answers: []settleAnswer{released}, wantCalls: 1},
		{name: "a confirm that found nothing is offered once more", answers: []settleAnswer{nothing, counted}, wantCalls: 2},
		{name: "a transport failure is offered once more", answers: []settleAnswer{failed, counted}, wantCalls: 2},
		{name: "nothing found twice is reported without the amount", answers: []settleAnswer{nothing, nothing}, wantCalls: 2, wantWarn: true},
		{name: "a transport failure twice is reported without the amount", answers: []settleAnswer{failed, failed}, wantCalls: 2, wantWarn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			queue, waits := newTestUnansweredSettleQueue(t, 4, nil)
			reserver := &sequenceReserver{confirmAnswers: tc.answers}
			uc := &UseCase{TracerReserver: reserver, unansweredSettles: queue}
			logger := &capturingLogger{}
			ctx, span, _ := anchorDeps()

			handle := unansweredTestHandle()
			uc.confirmReservations(ctx, span, logger, handle)
			queue.drain()

			assert.Len(t, reserver.confirmedTransactions(), tc.wantCalls)
			assert.Len(t, waits.requested(), tc.wantCalls, "every attempt waits the fixed delay first")

			reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
			assert.NotContains(t, reported, "1234.56", "an unanswered settle never logs the amount")

			if tc.wantWarn {
				assert.Contains(t, reported, "No reservation settled for the transaction after an unanswered reserve")
				assert.Contains(t, reported, handle.TransactionID.String())
			} else {
				assert.NotContains(t, reported, "No reservation settled")
			}

			debug := rendered(logger.atLevelOrMoreSevere(libLog.LevelDebug))
			if tc.wantDebug {
				assert.Contains(t, debug, "found nothing after a transport failure")
				assert.Contains(t, debug, handle.TransactionID.String())
				assert.NotContains(t, debug, "1234.56")
			} else {
				assert.NotContains(t, debug, "found nothing after a transport failure")
			}
		})
	}
}

func TestUnansweredRelease_RetriesOnlyATransportFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		errs      []error
		wantCalls int
		wantWarn  bool
	}{
		{name: "a delivered release is final", wantCalls: 1},
		{name: "a transport failure is offered once more", errs: []error{errSettleTransport}, wantCalls: 2},
		{name: "a transport failure twice is reported without the amount", errs: []error{errSettleTransport, errSettleTransport}, wantCalls: 2, wantWarn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			queue, waits := newTestUnansweredSettleQueue(t, 4, nil)
			reserver := &sequenceReserver{releaseErrs: tc.errs, confirmAnswers: []settleAnswer{{}}}
			uc := &UseCase{TracerReserver: reserver, unansweredSettles: queue}
			logger := &capturingLogger{}
			ctx, span, _ := anchorDeps()

			handle := unansweredTestHandle()
			uc.releaseReservations(ctx, span, logger, handle)
			queue.drain()

			assert.Len(t, reserver.releasedTransactions(), tc.wantCalls)
			assert.Len(t, waits.requested(), tc.wantCalls)
			assert.Empty(t, reserver.confirmedTransactions())

			reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
			assert.NotContains(t, reported, "1234.56")

			if tc.wantWarn {
				assert.Contains(t, reported, "Unanswered tracer reservation release could not be delivered")
				assert.Contains(t, reported, handle.TransactionID.String())
			} else {
				assert.NotContains(t, reported, "could not be delivered")
			}
		})
	}
}

func TestUnansweredSettle_FullQueueDropsWithoutBlocking(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	queue, _ := newTestUnansweredSettleQueue(t, 1, gate)
	reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
	uc := &UseCase{TracerReserver: reserver, unansweredSettles: queue}
	logger := &capturingLogger{}
	ctx, span, _ := anchorDeps()

	held := unansweredTestHandle()
	dropped := unansweredTestHandle()

	uc.confirmReservations(ctx, span, logger, held)
	uc.confirmReservations(ctx, span, logger, dropped)

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.Contains(t, reported, "Unanswered tracer reservation settle dropped")
	assert.Contains(t, reported, dropped.TransactionID.String())
	assert.NotContains(t, reported, "1234.56")

	close(gate)
	queue.drain()

	assert.Equal(t, []uuid.UUID{held.TransactionID}, reserver.confirmedTransactions(), "only the queued settle is attempted")
}

// outageReserver models a tracer outage: every by-transaction settle fails, and
// a by-id confirm fails once before the tracer comes back.
type outageReserver struct {
	stubReserver

	byIDFailures int
}

func (r *outageReserver) Confirm(_ context.Context, id uuid.UUID) (tracer.ConfirmOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.byIDFailures > 0 {
		r.byIDFailures--

		return tracer.ConfirmOutcome{}, errSettleTransport
	}

	r.confirmedIDs = append(r.confirmedIDs, id)

	return tracer.ConfirmOutcome{Confirmed: 1}, nil
}

func (r *outageReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (tracer.ConfirmOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.confirmedTxns = append(r.confirmedTxns, transactionID)

	return tracer.ConfirmOutcome{}, errSettleTransport
}

// TestUnansweredSettlesDuringAnOutageLeaveTheSharedRetrierFree swaps the
// process-wide retrier, so it cannot run in parallel.
func TestUnansweredSettlesDuringAnOutageLeaveTheSharedRetrierFree(t *testing.T) {
	withFastSharedRetrier(t)

	gate := make(chan struct{})
	queue, _ := newTestUnansweredSettleQueue(t, 4, gate)
	reserver := &outageReserver{byIDFailures: 1}
	uc := &UseCase{TracerReserver: reserver, unansweredSettles: queue}
	ctx, span, logger := anchorDeps()

	for range 3 * fastRetryPolicy().MaxInFlight {
		uc.confirmReservations(ctx, span, logger, unansweredTestHandle())
	}

	assert.Empty(t, reserver.confirmedTransactions(), "no unanswered settle runs on the request path")
	assert.Zero(t, len(sharedReservationRetrier.slots), "no unanswered settle occupies the shared retrier")

	answeredID := uuid.New()
	uc.confirmReservations(ctx, span, logger, reservationHandle{
		TransactionID:  uuid.New(),
		ReservationIDs: []uuid.UUID{answeredID},
		Amount:         decimal.RequireFromString("1234.56"),
		Asset:          "BRL",
	})
	sharedReservationRetrier.wait()

	assert.Equal(t, []uuid.UUID{answeredID}, reserver.confirmed(), "the by-id confirm is still redelivered")

	close(gate)
	queue.drain()
}

func TestUnansweredSettle_OutstandingAreReportedAtShutdownWithoutTheAmount(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	queue, _ := newTestUnansweredSettleQueue(t, 4, gate)
	uc := &UseCase{TracerReserver: &stubReserver{}, unansweredSettles: queue}
	ctx, span, _ := anchorDeps()

	handle := unansweredTestHandle()
	uc.confirmReservations(ctx, span, &libLog.NopLogger{}, handle)

	logger := &capturingLogger{}
	queue.reportOutstanding(ctx, logger)

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.Contains(t, reported, handle.TransactionID.String())
	assert.NotContains(t, reported, "1234.56")

	close(gate)
}

func TestUnansweredSettle_NilReserverSchedulesNothing(t *testing.T) {
	t.Parallel()

	queue, waits := newTestUnansweredSettleQueue(t, 4, nil)
	uc := &UseCase{unansweredSettles: queue}
	ctx, span, logger := anchorDeps()

	uc.confirmReservations(ctx, span, logger, unansweredTestHandle())
	uc.releaseReservations(ctx, span, logger, unansweredTestHandle())
	queue.drain()

	require.Empty(t, waits.requested())
}

func TestUnansweredSettle_CompletedSettleFreesItsSlotAndLeavesNoOutstanding(t *testing.T) {
	t.Parallel()

	const maxInFlight = 3

	cases := []struct {
		name  string
		gated bool
	}{
		{name: "completed settles free every slot for the next round"},
		{name: "a held queue accepts exactly its bound and drops the next", gated: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gate chan struct{}
			if tc.gated {
				gate = make(chan struct{})
			}

			queue, _ := newTestUnansweredSettleQueue(t, maxInFlight, gate)
			reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
			uc := &UseCase{TracerReserver: reserver, unansweredSettles: queue}
			logger := &capturingLogger{}
			ctx, span, _ := anchorDeps()

			if tc.gated {
				accepted := make([]uuid.UUID, 0, maxInFlight)

				for range maxInFlight {
					handle := unansweredTestHandle()
					accepted = append(accepted, handle.TransactionID)
					uc.confirmReservations(ctx, span, logger, handle)
				}

				assert.NotContains(t, rendered(logger.snapshot()), "settle dropped", "the bound itself is accepted")

				dropped := unansweredTestHandle()
				uc.confirmReservations(ctx, span, logger, dropped)

				reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
				assert.Contains(t, reported, "Unanswered tracer reservation settle dropped")
				assert.Contains(t, reported, dropped.TransactionID.String())
				assert.Equal(t, 1, strings.Count(reported, "settle dropped"), "only the settle past the bound is dropped")

				close(gate)
				queue.drain()

				assert.ElementsMatch(t, accepted, reserver.confirmedTransactions())

				return
			}

			for round := 1; round <= 2; round++ {
				for range maxInFlight {
					uc.confirmReservations(ctx, span, logger, unansweredTestHandle())
				}

				queue.drain()

				assert.Zero(t, len(queue.slots), "round %d: every finished settle returns its slot", round)
				assert.Empty(t, queue.outstanding(), "round %d: every finished settle is deregistered", round)

				shutdown := &capturingLogger{}
				queue.reportOutstanding(ctx, shutdown)
				assert.Empty(t, shutdown.snapshot(), "round %d: nothing is reported as abandoned", round)
			}

			assert.Len(t, reserver.confirmedTransactions(), 2*maxInFlight, "both rounds are attempted in full")
			assert.NotContains(t, rendered(logger.snapshot()), "settle dropped")
		})
	}
}

// withSharedUnansweredSettleQueue swaps the process-wide queue for a gated test
// queue whose settles stay queued for the whole test. Cleanup releases the gate,
// drains, and restores the shipped queue. A test using it cannot run in parallel.
func withSharedUnansweredSettleQueue(t *testing.T) {
	t.Helper()

	gate := make(chan struct{})
	queue, _ := newTestUnansweredSettleQueue(t, 4, gate)

	previous := sharedUnansweredSettleQueue
	sharedUnansweredSettleQueue = queue

	t.Cleanup(func() {
		close(gate)
		queue.drain()

		sharedUnansweredSettleQueue = previous
	})
}

// TestReportOutstandingReservationRetries_NamesAQueuedUnansweredSettle swaps the
// process-wide queues, so it cannot run in parallel.
func TestReportOutstandingReservationRetries_NamesAQueuedUnansweredSettle(t *testing.T) {
	withFastSharedRetrier(t)
	withSharedUnansweredSettleQueue(t)

	uc := &UseCase{TracerReserver: &stubReserver{}}
	ctx, span, _ := anchorDeps()

	handle := unansweredTestHandle()
	uc.confirmReservations(ctx, span, &libLog.NopLogger{}, handle)

	logger := &capturingLogger{}
	ReportOutstandingReservationRetries(ctx, logger)

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.Contains(t, reported, "Unanswered tracer reservation settle abandoned at shutdown")
	assert.Contains(t, reported, handle.TransactionID.String())
	assert.NotContains(t, rendered(logger.snapshot()), "1234.56", "the shutdown report never logs the amount")
}
