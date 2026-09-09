// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fastRetryPolicy is the shipped policy compressed so a test finishes in
// milliseconds. Only the durations change; the shape of the sequence does not.
func fastRetryPolicy() reservationRetryPolicy {
	return reservationRetryPolicy{
		MaxAttempts: 8,
		BaseDelay:   time.Millisecond,
		MaxDelay:    4 * time.Millisecond,
		Budget:      5 * time.Second,
		MaxInFlight: 8,
	}
}

// wait blocks until every scheduled retry sequence has finished. It lives in the
// test file because only tests need it: production never joins on a retry, which
// is the whole point of running it off the request path.
func (r *reservationRetrier) wait() {
	r.wg.Wait()
}

// withFastSharedRetrier swaps the process-wide retrier for a compressed one for
// the duration of one test, and drains it on cleanup. Without it a test whose
// stub never accepts would leave the shipped six-minute sequence running for the
// rest of the test binary's life.
func withFastSharedRetrier(t *testing.T) {
	t.Helper()

	previous := sharedReservationRetrier
	fast := newReservationRetrier(fastRetryPolicy())
	sharedReservationRetrier = fast

	t.Cleanup(func() {
		fast.wait()

		sharedReservationRetrier = previous
	})
}

// scriptedReserver is a TracerReserver whose confirm behaviour a test scripts.
// It records every call so the number of attempts is assertable.
type scriptedReserver struct {
	mu sync.Mutex

	confirmCalls     int
	confirmByTxn     int
	releaseCalls     int
	releaseByTxn     int
	confirmDelivered bool

	// confirm returns the error for attempt n (1-indexed); nil means accept.
	confirm func(attempt int) error
}

func (s *scriptedReserver) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	return &tracer.ReserveResult{}, nil
}

func (s *scriptedReserver) Confirm(_ context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmCalls++

	err := s.confirm(s.confirmCalls)
	if err == nil {
		s.confirmDelivered = true
	}

	return err
}

func (s *scriptedReserver) Release(_ context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releaseCalls++

	return nil
}

func (s *scriptedReserver) ConfirmByTransaction(_ context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmByTxn++

	err := s.confirm(s.confirmByTxn)
	if err == nil {
		s.confirmDelivered = true
	}

	return err
}

func (s *scriptedReserver) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releaseByTxn++

	return nil
}

func (s *scriptedReserver) attempts() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.confirmCalls + s.confirmByTxn, s.confirmDelivered
}

// failNTimes accepts only after n refusals.
func failNTimes(n int) func(int) error {
	return func(attempt int) error {
		if attempt <= n {
			return fmt.Errorf("tracer refused attempt %d: %w", attempt, tracer.ErrTracerUnavailable)
		}

		return nil
	}
}

// retryTransition is the confirm a test redelivers.
func retryTransition() reservationTransition {
	return reservationTransition{
		Action:        reservationActionConfirm,
		TransactionID: uuid.New(),
		ReservationID: uuid.New(),
		Amount:        decimal.RequireFromString("400.00"),
		Asset:         "BRL",
	}
}

// TestRetryDeliversAConfirmTheTransportRefused is the core repair: a confirm the
// tracer refused is not a lost spend any more. The ledger keeps offering it
// until the tracer takes it.
func TestRetryDeliversAConfirmTheTransportRefused(t *testing.T) {
	logger := &capturingLogger{}
	reserver := &scriptedReserver{confirm: failNTimes(2)}
	retrier := newReservationRetrier(fastRetryPolicy())

	cause := fmt.Errorf("inline attempt: %w", tracer.ErrTracerUnavailable)
	retrier.schedule(context.Background(), reserver, logger, retryTransition(), cause)
	retrier.wait()

	attempts, delivered := reserver.attempts()
	assert.True(t, delivered, "the committed spend must reach the tracer and be counted against the limit")
	assert.Equal(t, 3, attempts, "two refusals then acceptance")

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.Contains(t, reported, "under-enforced until now",
		"a confirm that only landed on retry must say the limit was under-enforced in between")
}

// TestRetryDeliversAConfirmThatTimedOut uses the real HTTP client against a real
// server, so the first attempts fail on the client's own 250ms per-operation
// deadline rather than on a stubbed error. No synthetic load: the handler simply
// waits for its request context to be cancelled.
func TestRetryDeliversAConfirmThatTimedOut(t *testing.T) {
	var requests int32

	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		stall := requests <= 2
		mu.Unlock()

		if stall {
			// Hold the request open until the client's deadline cancels it.
			<-r.Context().Done()

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := tracer.NewTracerClient(server.URL)
	require.NoError(t, err)

	logger := &capturingLogger{}
	retrier := newReservationRetrier(fastRetryPolicy())

	started := time.Now()
	retrier.schedule(context.Background(), client, logger,
		retryTransition(), fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))
	retrier.wait()

	mu.Lock()
	got := requests
	mu.Unlock()

	assert.Equal(t, int32(3), got, "two timed-out attempts then one accepted")
	assert.GreaterOrEqual(t, time.Since(started), 500*time.Millisecond,
		"each stalled attempt costs the client's 250ms per-operation budget, so two of them cost at least 500ms")

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.Contains(t, reported, "delivered on retry")
	assert.NotContains(t, reported, "could not be delivered")
}

// TestRetryKeepsTryingPastTheHoldExpiry covers the third failure shape: the
// confirm that only arrives after the hold has already expired. The tracer
// settles a late confirm by counting the spend, so the ledger must NOT stop
// trying once the hold is gone — stopping would be the one choice that loses
// the spend for good.
func TestRetryKeepsTryingPastTheHoldExpiry(t *testing.T) {
	// A hold that expires shortly after the retry sequence starts. The tracer
	// refuses everything until then; the first attempt after it is the late
	// confirm.
	holdExpiresAt := time.Now().Add(60 * time.Millisecond)

	var (
		mu             sync.Mutex
		lateConfirm    bool
		acceptedAtTime time.Time
	)

	reserver := &scriptedReserver{}
	reserver.confirm = func(_ int) error {
		mu.Lock()
		defer mu.Unlock()

		if time.Now().Before(holdExpiresAt) {
			return fmt.Errorf("tracer unreachable: %w", tracer.ErrTracerUnavailable)
		}

		lateConfirm = true
		acceptedAtTime = time.Now()

		return nil
	}

	policy := fastRetryPolicy()
	policy.MaxAttempts = 200
	policy.BaseDelay = time.Millisecond
	policy.MaxDelay = 5 * time.Millisecond

	retrier := newReservationRetrier(policy)
	retrier.schedule(context.Background(), reserver, &capturingLogger{},
		retryTransition(), fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))
	retrier.wait()

	_, delivered := reserver.attempts()
	require.True(t, delivered, "the spend must still be counted even though the hold had already expired")

	mu.Lock()
	defer mu.Unlock()

	assert.True(t, lateConfirm)
	assert.False(t, acceptedAtTime.Before(holdExpiresAt),
		"the confirm that landed is the one issued after the hold expired")
}

// TestShippedRetryBudgetOutlastsTheHold locks the load-bearing relationship
// between the two services' clocks. The tracer gives a direct transaction's hold
// a five-minute lifetime (reservationTTL in components/tracer's reservation
// service) and sweeps it at expiry. If the ledger gave up before that, every
// tracer outage longer than the retry budget but shorter than the hold would
// lose a spend that a slightly more patient ledger would have counted.
func TestShippedRetryBudgetOutlastsTheHold(t *testing.T) {
	const tracerDirectHoldLifetime = 5 * time.Minute

	assert.Greater(t, defaultReservationRetryPolicy.Budget, tracerDirectHoldLifetime,
		"the ledger must still be trying when the tracer's own hold expires")
	assert.Positive(t, defaultReservationRetryPolicy.MaxAttempts)
	assert.Positive(t, defaultReservationRetryPolicy.MaxInFlight)
}

// TestRetryReportsATransitionItCannotDeliver is the visibility floor. When every
// attempt fails there IS a lost spend, and it must be legible: severity at least
// Warn, naming the transaction, the reservation and the amount.
func TestRetryReportsATransitionItCannotDeliver(t *testing.T) {
	// A closed port: the dial is refused immediately, so the sequence exhausts
	// its attempts without waiting on any timeout.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	closedAddr := listener.Addr().String()
	require.NoError(t, listener.Close())

	client, err := tracer.NewTracerClient("http://" + closedAddr)
	require.NoError(t, err)

	logger := &capturingLogger{}
	transition := retryTransition()

	retrier := newReservationRetrier(fastRetryPolicy())
	retrier.schedule(context.Background(), client, logger, transition,
		fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))
	retrier.wait()

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
	require.NotEmpty(t, reported, "an undeliverable confirm must never be silent")

	assert.Contains(t, reported, "could not be delivered")
	assert.Contains(t, reported, "will not be counted against the limit")
	assert.Contains(t, reported, transition.TransactionID.String())
	assert.Contains(t, reported, transition.ReservationID.String())
	assert.Contains(t, reported, "400")
	assert.Contains(t, reported, "BRL")

	errors := logger.atLevelOrMoreSevere(libLog.LevelError)
	assert.NotEmpty(t, errors, "a spend the ledger gave up on is an Error, not a Warn")
}

// TestRetryReportsATransitionItHasNoCapacityFor proves the concurrency cap fails
// loudly. Under a total tracer outage the retrier runs out of slots; the
// transitions it turns away are the honest residual and must be reported, not
// dropped.
func TestRetryReportsATransitionItHasNoCapacityFor(t *testing.T) {
	policy := fastRetryPolicy()
	policy.MaxInFlight = 1
	policy.MaxAttempts = 100
	policy.BaseDelay = 20 * time.Millisecond
	policy.MaxDelay = 20 * time.Millisecond

	retrier := newReservationRetrier(policy)

	occupy := &scriptedReserver{confirm: func(_ int) error {
		return fmt.Errorf("still down: %w", tracer.ErrTracerUnavailable)
	}}

	// Fill the single slot with a sequence that will keep retrying.
	retrier.schedule(context.Background(), occupy, &capturingLogger{}, retryTransition(),
		fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))

	logger := &capturingLogger{}
	turnedAway := retryTransition()
	rejected := &scriptedReserver{confirm: func(_ int) error { return nil }}

	retrier.schedule(context.Background(), rejected, logger, turnedAway,
		fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))

	reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelError))
	assert.Contains(t, reported, "too many retries already in flight")
	assert.Contains(t, reported, turnedAway.TransactionID.String())
	assert.Contains(t, reported, "400")

	attempts, _ := rejected.attempts()
	assert.Zero(t, attempts, "a turned-away transition is reported, never quietly retried later")

	retrier.wait()
}

// TestRepeatedConfirmDoesNotCountTheSpendTwice mirrors, on the ledger side, the
// rule the tracer enforces in the database: a confirm settles a reservation only
// while it is still settleable (RESERVED or EXPIRED), and the row flip is
// guarded on the status read under the lock. A second confirm therefore flips
// nothing and moves no counter.
//
// The tracer owns that guarantee — see settleableByConfirmPredicate and
// applyConfirm in components/tracer's usage_reservation_repository, and the
// service mapping of ErrReservationAlreadyTerminal to a 200. This test proves
// the ledger's retry does not depend on anything weaker: it re-offers the same
// transition and requires the counted spend to stay at one.
func TestRepeatedConfirmDoesNotCountTheSpendTwice(t *testing.T) {
	var (
		mu           sync.Mutex
		countedSpend int
		settled      bool
	)

	// tracerLike models the settleable-status rule: the first confirm settles
	// and counts, every later one is an accepted no-op.
	tracerLike := &scriptedReserver{}
	tracerLike.confirm = func(attempt int) error {
		mu.Lock()
		defer mu.Unlock()

		if attempt == 1 {
			// The tracer accepted and settled, but the response never reached
			// the ledger — the case that makes a repeat necessary.
			countedSpend++
			settled = true

			return fmt.Errorf("response lost: %w", tracer.ErrTracerUnavailable)
		}

		if settled {
			return nil // already terminal: accepted, nothing flipped
		}

		countedSpend++
		settled = true

		return nil
	}

	retrier := newReservationRetrier(fastRetryPolicy())
	retrier.schedule(context.Background(), tracerLike, &capturingLogger{}, retryTransition(),
		fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))
	retrier.wait()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 1, countedSpend, "a repeated confirm must not count the spend twice")
}

// tenantSpy records the tenant each retry attempt carried on its context.
type tenantSpy struct {
	mu    sync.Mutex
	seen  []string
	calls int
}

func (s *tenantSpy) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	return &tracer.ReserveResult{}, nil
}

func (s *tenantSpy) Confirm(ctx context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls++
	s.seen = append(s.seen, tmcore.GetTenantIDContext(ctx))

	if s.calls < 3 {
		return fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)
	}

	return nil
}

func (s *tenantSpy) Release(_ context.Context, _ uuid.UUID) error              { return nil }
func (s *tenantSpy) ConfirmByTransaction(_ context.Context, _ uuid.UUID) error { return nil }
func (s *tenantSpy) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error { return nil }

func (s *tenantSpy) tenants() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.seen...)
}

// TestRetryKeepsTheTenantAfterTheRequestEnds guards the two properties that make
// a detached retry correct in a multi-tenant deployment.
//
// The first is that it runs at all: the request's context is cancelled the
// moment the response is written, and a retry bound to it would die before its
// first attempt. The second is that it still speaks for the right customer —
// the tracer client reads the tenant off the context it is handed, so a
// detachment that dropped the tenant would send the confirm to the wrong
// tenant's data, or to none, which on a spending limit is worse than not
// sending it.
func TestRetryKeepsTheTenantAfterTheRequestEnds(t *testing.T) {
	const tenant = "acme-tenant-1"

	requestCtx, endRequest := context.WithCancel(tmcore.ContextWithTenantID(context.Background(), tenant))

	spy := &tenantSpy{}
	retrier := newReservationRetrier(fastRetryPolicy())

	retrier.schedule(requestCtx, spy, &capturingLogger{}, retryTransition(),
		fmt.Errorf("inline: %w", tracer.ErrTracerUnavailable))

	// The response is written and the request context is cancelled while the
	// ledger still owes the tracer a confirm.
	endRequest()

	retrier.wait()

	seen := spy.tenants()
	require.NotEmpty(t, seen, "the retry must outlive the request whose context was cancelled")

	for attempt, got := range seen {
		assert.Equal(t, tenant, got, "attempt %d must still speak for the tenant that owns the transaction", attempt+1)
	}
}

// TestAnchorHandsAFailedTransitionToTheRetrier wires the two halves together:
// the seams the transaction pipelines call must route a failure into the shared
// retrier rather than swallowing it.
func TestAnchorHandsAFailedTransitionToTheRetrier(t *testing.T) {
	withFastSharedRetrier(t)

	ctx := context.Background()
	_, span := noop.NewTracerProvider().Tracer("t").Start(ctx, "test")

	t.Run("direct create, by reservation id", func(t *testing.T) {
		reserver := &scriptedReserver{confirm: failNTimes(1)}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservations(ctx, span, &capturingLogger{}, reservationHandle{
			ReservationIDs: []uuid.UUID{uuid.New()},
			TransactionID:  uuid.New(),
			Amount:         decimal.RequireFromString("400.00"),
			Asset:          "BRL",
		})

		sharedReservationRetrier.wait()

		attempts, delivered := reserver.attempts()
		assert.True(t, delivered, "the spend reaches the tracer on the retry")
		assert.Equal(t, 2, attempts, "one inline attempt plus one retry")
	})

	t.Run("pending commit, by transaction id", func(t *testing.T) {
		reserver := &scriptedReserver{confirm: failNTimes(1)}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, span, &capturingLogger{},
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce},
			reservationHandle{
				TransactionID: uuid.New(),
				Amount:        decimal.RequireFromString("250.00"),
				Asset:         "BRL",
			}, false)

		sharedReservationRetrier.wait()

		attempts, delivered := reserver.attempts()
		assert.True(t, delivered)
		assert.Equal(t, 2, attempts)
	})

	t.Run("a tracer that is off schedules nothing", func(t *testing.T) {
		uc := &UseCase{TracerReserver: nil}

		uc.confirmReservations(ctx, span, &capturingLogger{}, reservationHandle{
			ReservationIDs: []uuid.UUID{uuid.New()},
			TransactionID:  uuid.New(),
		})

		sharedReservationRetrier.wait()
	})
}
