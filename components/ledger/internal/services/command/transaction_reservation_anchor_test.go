// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fixedReserveTimestamp is a deterministic timestamp the anchor tests pass for
// transactionTimestamp so no test calls time.Now().
var fixedReserveTimestamp = time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

const fixedReserveAccountID = "acc-source-1"

// fixedReserveAccount is the account scope the anchor tests hand the reserve
// anchor: the source account id plus its free-form ledger account type.
var fixedReserveAccount = tracer.ReserveAccount{AccountID: fixedReserveAccountID, Type: "deposit"}

// byTxnIdentity builds the identity the by-transaction confirm/release is
// addressed and reported with: a handle carrying no reservation ids, because
// the create-time handle does not survive into /commit or /cancel.
func byTxnIdentity(transactionID uuid.UUID) reservationHandle {
	return reservationHandle{
		TransactionID: transactionID,
		Amount:        decimal.NewFromInt(1000),
		Asset:         "BRL",
	}
}

// stubReserver is a scripted TracerReserver: it records calls and returns the
// configured reserve result/error and per-action transition errors so each
// branch of the anchor and the post-commit transport can be asserted without a
// live tracer.
type stubReserver struct {
	// mu guards the recorded calls. A transition the tracer refuses is now
	// retried on a background goroutine, so the stub is written from there
	// while the test body reads it.
	mu sync.Mutex

	reserveCalls int
	requests     []tracer.ReserveRequest
	confirmedIDs []uuid.UUID
	releasedIDs  []uuid.UUID

	confirmedTxns []uuid.UUID
	releasedTxns  []uuid.UUID

	result     *tracer.ReserveResult
	reserveErr error

	confirmErr error
	releaseErr error

	confirmByTxnErr error
	releaseByTxnErr error

	// confirmOutcome and confirmByTxnOutcome are what a successful confirm
	// reports; the zero value is a confirm that found nothing released.
	confirmOutcome      tracer.ConfirmOutcome
	confirmByTxnOutcome tracer.ConfirmOutcome
}

func (s *stubReserver) Reserve(_ context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reserveCalls++
	s.requests = append(s.requests, req)

	if s.reserveErr != nil {
		return nil, s.reserveErr
	}

	return s.result, nil
}

func (s *stubReserver) Confirm(_ context.Context, id uuid.UUID) (tracer.ConfirmOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmedIDs = append(s.confirmedIDs, id)

	if s.confirmErr != nil {
		return tracer.ConfirmOutcome{}, s.confirmErr
	}

	return s.confirmOutcome, nil
}

func (s *stubReserver) Release(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releasedIDs = append(s.releasedIDs, id)

	return s.releaseErr
}

func (s *stubReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (tracer.ConfirmOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmedTxns = append(s.confirmedTxns, transactionID)

	if s.confirmByTxnErr != nil {
		return tracer.ConfirmOutcome{}, s.confirmByTxnErr
	}

	return s.confirmByTxnOutcome, nil
}

func (s *stubReserver) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releasedTxns = append(s.releasedTxns, transactionID)

	return s.releaseByTxnErr
}

// The recorded-call accessors below return copies under the lock. Tests read
// through them rather than touching the slices directly.

func (s *stubReserver) reserves() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reserveCalls
}

func (s *stubReserver) reserveRequests() []tracer.ReserveRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]tracer.ReserveRequest(nil), s.requests...)
}

func (s *stubReserver) confirmed() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.confirmedIDs...)
}

func (s *stubReserver) released() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.releasedIDs...)
}

func (s *stubReserver) confirmedTransactions() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.confirmedTxns...)
}

func (s *stubReserver) releasedTransactions() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.releasedTxns...)
}

// anchorDeps returns the ctx, noop span, and a nil logger used by every anchor
// unit test. The span is a real otel noop span so SetAttributes /
// HandleSpanError are valid no-ops; the logger is the lib-observability
// NopLogger so structured-log calls do not write.
func anchorDeps() (context.Context, trace.Span, libLog.Logger) {
	ctx := context.Background()
	_, span := noop.NewTracerProvider().Tracer("t").Start(ctx, "test")

	return ctx, span, &libLog.NopLogger{}
}

func TestReserveTransaction_OffOrNilReserver_Proceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	t.Run("nil reserver", func(t *testing.T) {
		uc := &UseCase{TracerReserver: nil}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Empty(t, out.Handle.ReservationIDs)
	})

	t.Run("mode off", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 0, reserver.reserves(), "mode=off must not call the tracer")
	})

	t.Run("empty mode treated as off", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 0, reserver.reserves())
	})
}

// TestReserveTransaction_HonoredSkip_Proceeds proves the per-call tracer skip:
// an honored skip short-circuits the reserve anchor — zero gRPC Reserve, outcome
// proceed, empty handle — even under enforce/advisory, where the reserve would
// otherwise fire. The skip wins over the mode because the operator opted in.
func TestReserveTransaction_HonoredSkip_Proceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	cases := []struct {
		name     string
		settings mmodel.TracerSettings
	}{
		{"enforce", mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed}},
		{"advisory", mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory}},
	}

	for _, tc := range cases {
		t.Run(tc.name+" honored skip makes zero Reserve", func(t *testing.T) {
			reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true}}
			uc := &UseCase{TracerReserver: reserver}

			out := uc.reserveTransaction(tracerCtx, sp, logger, tc.settings, uuid.New(),
				decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, true)

			assert.Equal(t, reservationProceed, out.Kind, "honored skip must proceed without gating")
			assert.Equal(t, 0, reserver.reserves(), "honored skip must NOT call the tracer Reserve")
			assert.Empty(t, out.Handle.ReservationIDs, "an honored skip holds no reservation")
		})
	}

	t.Run("absent skip still reserves under enforce", func(t *testing.T) {
		reserver := &stubReserver{result: &tracer.ReserveResult{Denied: false, ReservationIDs: []uuid.UUID{uuid.New()}}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 1, reserver.reserves(), "without a skip the reserve fires exactly once, as today")
	})
}

func TestReserveTransaction_EnforceAllow_Proceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	reserver := &stubReserver{result: &tracer.ReserveResult{Denied: false, ReservationIDs: ids}}
	uc := &UseCase{TracerReserver: reserver}

	out := uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	assert.Equal(t, reservationProceed, out.Kind)
	assert.Equal(t, 1, reserver.reserves())
	assert.Equal(t, ids, out.Handle.ReservationIDs, "the handle carries the reservation ids for post-commit confirm")
}

func TestReserveTransaction_EnforceDeny_Rejects(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true}}
	uc := &UseCase{TracerReserver: reserver}

	out := uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	require.Equal(t, reservationReject, out.Kind)
	require.Error(t, out.Err)

	var unprocessable pkg.UnprocessableOperationError
	require.ErrorAs(t, out.Err, &unprocessable)
	assert.Equal(t, constant.ErrTransactionReservationDenied.Error(), unprocessable.Code)
}

func TestReserveTransaction_Advisory_NeverBlocks(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	t.Run("advisory + deny proceeds", func(t *testing.T) {
		reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind, "advisory must never block, even on deny")
		assert.Equal(t, 1, reserver.reserves(), "advisory still calls the tracer")
	})

	t.Run("advisory + unavailable proceeds", func(t *testing.T) {
		reserver := &stubReserver{reserveErr: fmt.Errorf("boom: %w", tracer.ErrTracerUnavailable)}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.Equal(t, reservationProceed, out.Kind, "advisory ignores availability failures")
	})
}

func TestReserveTransaction_FailOpen_SkipsAndProceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	uc := &UseCase{TracerReserver: reserver}

	out := uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	assert.Equal(t, reservationProceed, out.Kind, "fail-open must proceed when the tracer is unavailable")
	assert.Empty(t, out.Handle.ReservationIDs)
}

func TestReserveTransaction_FailClosed_Rejects(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	uc := &UseCase{TracerReserver: reserver}

	out := uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	require.Equal(t, reservationReject, out.Kind, "fail-closed must reject when the tracer is unavailable")
	require.Error(t, out.Err)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, out.Err, &unavailable)
	assert.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)
}

func TestReserveTransaction_LongLivedHint_OnPending(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	// Capture the request the anchor builds to assert the long-lived hint.
	capturing := &capturingReserver{result: &tracer.ReserveResult{}}
	uc := &UseCase{TracerReserver: capturing}

	uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLLongLived, reservationForCreate, false)

	assert.True(t, capturing.lastReq.LongLived,
		"PENDING reservations must carry the long-lived TTL hint")
	assert.Empty(t, capturing.lastReq.TransactionType,
		"the long-lived hint must NOT be smuggled through transactionType (it broke the tracer reserve enum)")

	// Default TTL must NOT carry the hint.
	uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	assert.False(t, capturing.lastReq.LongLived, "direct transactions must not carry the long-lived hint")
}

func TestReserveTransaction_BuildsFaithfulTracerRequest(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	capturing := &capturingReserver{result: &tracer.ReserveResult{}}
	uc := &UseCase{TracerReserver: capturing}

	txID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	metadata := map[string]any{
		"channel":  "app",
		"priority": 3.0,
		"bad-key":  "dropped",
	}

	uc.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		txID, decimal.NewFromInt(1000), "BRL", fixedReserveAccount, metadata, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	req := capturing.lastReq
	assert.Equal(t, txID, req.TransactionID)
	assert.Equal(t, "1000", req.Amount)
	assert.Equal(t, "BRL", req.Asset)
	assert.Equal(t, fixedReserveAccountID, req.Account.AccountID, "account scope must be the structured account, not a bare string")
	assert.Equal(t, "deposit", req.Account.Type, "the source account type must reach the tracer verbatim")
	assert.Equal(t, map[string]string{"channel": "app", "priority": "3"}, req.Metadata,
		"metadata carries only tracer-accepted keys, with scalar values rendered as strings")
	assert.NotEmpty(t, req.RequestID, "the tracer reserve contract requires a non-nil requestId")
	assert.Equal(t, fixedReserveTimestamp.Format(time.RFC3339Nano), req.TransactionTimestamp)

	// RequestID is deterministic: same transactionID derives the same requestId
	// so retries dedup.
	assert.Equal(t, reservationRequestID(txID).String(), req.RequestID)
}

func TestReservationRequestID_Deterministic(t *testing.T) {
	txID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	first := reservationRequestID(txID)
	second := reservationRequestID(txID)

	assert.Equal(t, first, second, "the same transactionID must derive the same requestId")
	assert.NotEqual(t, uuid.Nil, first, "requestId must be non-nil for the tracer reserve contract")
	assert.NotEqual(t, reservationRequestID(uuid.MustParse("55555555-5555-5555-5555-555555555555")), first,
		"distinct transactionIDs must derive distinct requestIds")
}

// deadlineReserver records the deadline the anchor hands the reserve call and,
// when waitForDeadline is set, blocks until that context is done. It maps a done
// context to tracer.ErrTracerUnavailable, as the tracer clients do.
type deadlineReserver struct {
	capturingReserver

	waitForDeadline bool
	deadline        time.Time
	hasDeadline     bool
}

func (d *deadlineReserver) Reserve(ctx context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	d.lastReq = req
	d.deadline, d.hasDeadline = ctx.Deadline()

	if d.waitForDeadline {
		<-ctx.Done()

		return nil, fmt.Errorf("%w: %w", tracer.ErrTracerUnavailable, ctx.Err())
	}

	return d.result, nil
}

// farParentDeadline is a fixed deadline far beyond any timeoutMs, so a deadline
// the anchor adds is observable as one earlier than the parent's.
var farParentDeadline = time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)

func TestReserveTransaction_HonorsTimeoutMs(t *testing.T) {
	t.Parallel()

	parentWithFarDeadline := func(t *testing.T) context.Context {
		t.Helper()

		ctx, cancel := context.WithDeadline(context.Background(), farParentDeadline)
		t.Cleanup(cancel)

		return ctx
	}

	t.Run("timeoutMs bounds the reserve call below the parent deadline", func(t *testing.T) {
		t.Parallel()

		_, sp, logger := anchorDeps()
		reserver := &deadlineReserver{capturingReserver: capturingReserver{result: &tracer.ReserveResult{}}}
		uc := &UseCase{TracerReserver: reserver}

		uc.reserveTransaction(parentWithFarDeadline(t), sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen, TimeoutMs: 250},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		require.True(t, reserver.hasDeadline, "a positive timeoutMs must put a deadline on the reserve call")
		assert.True(t, reserver.deadline.Before(farParentDeadline),
			"the timeoutMs deadline must tighten the parent deadline, not inherit it")
	})

	t.Run("zero timeoutMs leaves the parent deadline untouched", func(t *testing.T) {
		t.Parallel()

		_, sp, logger := anchorDeps()
		reserver := &deadlineReserver{capturingReserver: capturingReserver{result: &tracer.ReserveResult{}}}
		uc := &UseCase{TracerReserver: reserver}

		uc.reserveTransaction(parentWithFarDeadline(t), sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		require.True(t, reserver.hasDeadline)
		assert.True(t, reserver.deadline.Equal(farParentDeadline),
			"timeoutMs=0 leaves the client timeout as the only bound")
	})

	t.Run("zero timeoutMs adds no deadline", func(t *testing.T) {
		t.Parallel()

		tracerCtx, sp, logger := anchorDeps()
		reserver := &deadlineReserver{capturingReserver: capturingReserver{result: &tracer.ReserveResult{}}}
		uc := &UseCase{TracerReserver: reserver}

		uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		assert.False(t, reserver.hasDeadline, "timeoutMs=0 leaves the client timeout as the only bound")
	})

	t.Run("an expired timeoutMs is an outage and follows the posture", func(t *testing.T) {
		t.Parallel()

		ctx, span, ended := recordingSpan(t)
		reserver := &deadlineReserver{capturingReserver: capturingReserver{result: &tracer.ReserveResult{}}, waitForDeadline: true}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed, TimeoutMs: 50},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

		require.True(t, reserver.hasDeadline)
		require.Equal(t, reservationReject, out.Kind)

		var unavailable pkg.ServiceUnavailableError
		require.ErrorAs(t, out.Err, &unavailable)
		assert.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)

		attrs := spanAttributes(ended())
		assert.Equal(t, int64(50), attrs["app.tracer.timeout_ms"].AsInt64())
	})
}

// spanAttributes flattens the attributes of the ended spans, last write wins.
func spanAttributes(spans []sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	out := map[attribute.Key]attribute.Value{}

	for _, s := range spans {
		for _, kv := range s.Attributes() {
			out[kv.Key] = kv.Value
		}
	}

	return out
}

func TestReservationTTLForStatus(t *testing.T) {
	assert.Equal(t, reservationTTLLongLived, reservationTTLForStatus(constant.PENDING))
	assert.Equal(t, reservationTTLDefault, reservationTTLForStatus(constant.APPROVED))
	assert.Equal(t, reservationTTLDefault, reservationTTLForStatus(constant.CREATED))
}

func TestConfirmReservations(t *testing.T) {
	withFastSharedRetrier(t)

	ctx, sp, logger := anchorDeps()

	t.Run("confirms every id", func(t *testing.T) {
		ids := []uuid.UUID{uuid.New(), uuid.New()}
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: ids})

		assert.Equal(t, ids, reserver.confirmed())
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		uc := &UseCase{TracerReserver: nil}
		uc.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: []uuid.UUID{uuid.New()}})
		// no panic, nothing to assert beyond not crashing
	})

	t.Run("transport failure does not propagate, and is retried", func(t *testing.T) {
		ids := []uuid.UUID{uuid.New(), uuid.New()}
		reserver := &stubReserver{confirmErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		uc := &UseCase{TracerReserver: reserver}

		// confirmReservations returns nothing; the contract is that it must not
		// panic and must attempt every id despite the error.
		uc.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: ids})

		sharedReservationRetrier.wait()

		attempted := reserver.confirmed()
		assert.Subset(t, attempted, ids, "every id is attempted inline even when transport fails")
		assert.Greater(t, len(attempted), len(ids),
			"a refused confirm is retried off the request path, not dropped after one attempt")
	})
}

func TestReleaseReservations(t *testing.T) {
	withFastSharedRetrier(t)

	ctx, sp, logger := anchorDeps()

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	reserver := &stubReserver{releaseErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
	uc := &UseCase{TracerReserver: reserver}

	uc.releaseReservations(ctx, sp, logger, reservationHandle{ReservationIDs: ids})

	sharedReservationRetrier.wait()

	attempted := reserver.released()
	assert.Subset(t, attempted, ids, "release is attempted for every id despite transport failure")
	assert.Greater(t, len(attempted), len(ids),
		"a refused release is retried too: capacity a customer is not spending must come back")
}

func TestConfirmReservationsByTransaction(t *testing.T) {
	withFastSharedRetrier(t)

	ctx, sp, logger := anchorDeps()

	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	t.Run("commit confirms by transaction id", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(txID), false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTransactions())
		assert.Empty(t, reserver.releasedTransactions())
	})

	t.Run("advisory still confirms (lifecycle observed, never blocks)", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory}, byTxnIdentity(txID), false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTransactions())
	})

	t.Run("mode off does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, byTxnIdentity(uuid.New()), false)

		assert.Empty(t, reserver.confirmedTransactions(), "mode=off must not confirm")
	})

	t.Run("empty mode does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, sp, logger, mmodel.TracerSettings{}, byTxnIdentity(uuid.New()), false)

		assert.Empty(t, reserver.confirmedTransactions())
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		uc := &UseCase{TracerReserver: nil}
		uc.confirmReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(uuid.New()), false)
		// no panic, nothing to assert beyond not crashing
	})

	t.Run("transport failure does not propagate", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{confirmByTxnErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		uc := &UseCase{TracerReserver: reserver}

		// The contract is that the request still succeeds: the helper returns
		// nothing, swallows the error, and the caller proceeds.
		uc.confirmReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(txID), false)

		sharedReservationRetrier.wait()

		attempted := reserver.confirmedTransactions()
		assert.Contains(t, attempted, txID, "the transition is attempted despite transport failure")
		assert.Greater(t, len(attempted), 1, "and it is retried rather than dropped after one attempt")
	})

	t.Run("honored skip does not confirm even under enforce", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(uuid.New()), true)

		assert.Empty(t, reserver.confirmedTransactions(), "an honored tracer skip must make zero ConfirmByTransaction")
	})
}

func TestReleaseReservationsByTransaction(t *testing.T) {
	withFastSharedRetrier(t)

	ctx, sp, logger := anchorDeps()

	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	t.Run("cancel releases by transaction id", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.releaseReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(txID), false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.releasedTransactions())
		assert.Empty(t, reserver.confirmedTransactions())
	})

	t.Run("mode off does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.releaseReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, byTxnIdentity(uuid.New()), false)

		assert.Empty(t, reserver.releasedTransactions())
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		uc := &UseCase{TracerReserver: nil}
		uc.releaseReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(uuid.New()), false)
	})

	t.Run("transport failure does not propagate", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{releaseByTxnErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		uc := &UseCase{TracerReserver: reserver}

		uc.releaseReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(txID), false)

		sharedReservationRetrier.wait()

		attempted := reserver.releasedTransactions()
		assert.Contains(t, attempted, txID, "the transition is attempted despite transport failure")
		assert.Greater(t, len(attempted), 1, "and it is retried rather than dropped after one attempt")
	})

	t.Run("honored skip does not release even under enforce", func(t *testing.T) {
		reserver := &stubReserver{}
		uc := &UseCase{TracerReserver: reserver}

		uc.releaseReservationsByTransaction(ctx, sp, logger, enforce, byTxnIdentity(uuid.New()), true)

		assert.Empty(t, reserver.releasedTransactions(), "an honored tracer skip must make zero ReleaseByTransaction")
	})
}

// capturingReserver records the last reserve request so the long-lived TTL hint
// can be asserted.
type capturingReserver struct {
	lastReq tracer.ReserveRequest
	result  *tracer.ReserveResult
}

func (c *capturingReserver) Reserve(_ context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	c.lastReq = req
	return c.result, nil
}

func (c *capturingReserver) Confirm(_ context.Context, _ uuid.UUID) (tracer.ConfirmOutcome, error) {
	return tracer.ConfirmOutcome{}, nil
}
func (c *capturingReserver) Release(_ context.Context, _ uuid.UUID) error { return nil }

func (c *capturingReserver) ConfirmByTransaction(_ context.Context, _ uuid.UUID) (tracer.ConfirmOutcome, error) {
	return tracer.ConfirmOutcome{}, nil
}

func (c *capturingReserver) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error { return nil }

// forbiddenReserver fails the test on ANY call. It is the direct proof a pipeline never
// reaches the tracer: asserting a zero call count only shows the stub was not invoked,
// while this shows no transport could have been reached at all.
type forbiddenReserver struct {
	t *testing.T
}

func (f *forbiddenReserver) fail(method string) {
	f.t.Helper()
	f.t.Fatalf("a /v1 pipeline reached the tracer via %s — the /v1 contract names no reservation seam", method)
}

func (f *forbiddenReserver) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	f.fail("Reserve")

	return nil, nil
}

func (f *forbiddenReserver) Confirm(_ context.Context, _ uuid.UUID) (tracer.ConfirmOutcome, error) {
	f.fail("Confirm")

	return tracer.ConfirmOutcome{}, nil
}

func (f *forbiddenReserver) Release(_ context.Context, _ uuid.UUID) error {
	f.fail("Release")

	return nil
}

func (f *forbiddenReserver) ConfirmByTransaction(_ context.Context, _ uuid.UUID) (tracer.ConfirmOutcome, error) {
	f.fail("ConfirmByTransaction")

	return tracer.ConfirmOutcome{}, nil
}

func (f *forbiddenReserver) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error {
	f.fail("ReleaseByTransaction")

	return nil
}
