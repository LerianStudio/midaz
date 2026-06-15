// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fixedReserveTimestamp is a deterministic timestamp the anchor tests pass for
// transactionTimestamp so no test calls time.Now().
var fixedReserveTimestamp = time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

const fixedReserveAccountID = "acc-source-1"

// stubReserver is a scripted TracerReserver: it records calls and returns the
// configured reserve result/error and per-action transition errors so each
// branch of the anchor and the post-commit transport can be asserted without a
// live tracer.
type stubReserver struct {
	reserveCalls int
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
}

func (s *stubReserver) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	s.reserveCalls++

	if s.reserveErr != nil {
		return nil, s.reserveErr
	}

	return s.result, nil
}

func (s *stubReserver) Confirm(_ context.Context, id uuid.UUID) error {
	s.confirmedIDs = append(s.confirmedIDs, id)
	return s.confirmErr
}

func (s *stubReserver) Release(_ context.Context, id uuid.UUID) error {
	s.releasedIDs = append(s.releasedIDs, id)
	return s.releaseErr
}

func (s *stubReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.confirmedTxns = append(s.confirmedTxns, transactionID)
	return s.confirmByTxnErr
}

func (s *stubReserver) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.releasedTxns = append(s.releasedTxns, transactionID)
	return s.releaseByTxnErr
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
		handler := &TransactionHandler{TracerReserver: nil}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Empty(t, out.Handle.ReservationIDs)
	})

	t.Run("mode off", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 0, reserver.reserveCalls, "mode=off must not call the tracer")
	})

	t.Run("empty mode treated as off", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{}, uuid.New(),
			decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 0, reserver.reserveCalls)
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
			handler := &TransactionHandler{TracerReserver: reserver}

			out := handler.reserveTransaction(tracerCtx, sp, logger, tc.settings, uuid.New(),
				decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, true)

			assert.Equal(t, reservationProceed, out.Kind, "honored skip must proceed without gating")
			assert.Equal(t, 0, reserver.reserveCalls, "honored skip must NOT call the tracer Reserve")
			assert.Empty(t, out.Handle.ReservationIDs, "an honored skip holds no reservation")
		})
	}

	t.Run("absent skip still reserves under enforce", func(t *testing.T) {
		reserver := &stubReserver{result: &tracer.ReserveResult{Denied: false, ReservationIDs: []uuid.UUID{uuid.New()}}}
		handler := &TransactionHandler{TracerReserver: reserver}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind)
		assert.Equal(t, 1, reserver.reserveCalls, "without a skip the reserve fires exactly once, as today")
	})
}

func TestReserveTransaction_EnforceAllow_Proceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	reserver := &stubReserver{result: &tracer.ReserveResult{Denied: false, ReservationIDs: ids}}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

	assert.Equal(t, reservationProceed, out.Kind)
	assert.Equal(t, 1, reserver.reserveCalls)
	assert.Equal(t, ids, out.Handle.ReservationIDs, "the handle carries the reservation ids for post-commit confirm")
}

func TestReserveTransaction_EnforceDeny_Rejects(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true}}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

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
		handler := &TransactionHandler{TracerReserver: reserver}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind, "advisory must never block, even on deny")
		assert.Equal(t, 1, reserver.reserveCalls, "advisory still calls the tracer")
	})

	t.Run("advisory + unavailable proceeds", func(t *testing.T) {
		reserver := &stubReserver{reserveErr: fmt.Errorf("boom: %w", tracer.ErrTracerUnavailable)}
		handler := &TransactionHandler{TracerReserver: reserver}

		out := handler.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

		assert.Equal(t, reservationProceed, out.Kind, "advisory ignores availability failures")
	})
}

func TestReserveTransaction_FailOpen_SkipsAndProceeds(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

	assert.Equal(t, reservationProceed, out.Kind, "fail-open must proceed when the tracer is unavailable")
	assert.Empty(t, out.Handle.ReservationIDs)
}

func TestReserveTransaction_FailClosed_Rejects(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

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
	handler := &TransactionHandler{TracerReserver: capturing}

	handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLLongLived, false)

	assert.True(t, capturing.lastReq.LongLived,
		"PENDING reservations must carry the long-lived TTL hint")
	assert.Empty(t, capturing.lastReq.TransactionType,
		"the long-lived hint must NOT be smuggled through transactionType (it broke the tracer reserve enum)")

	// Default TTL must NOT carry the hint.
	handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

	assert.False(t, capturing.lastReq.LongLived, "direct transactions must not carry the long-lived hint")
}

func TestReserveTransaction_BuildsFaithfulTracerRequest(t *testing.T) {
	tracerCtx, sp, logger := anchorDeps()

	capturing := &capturingReserver{result: &tracer.ReserveResult{}}
	handler := &TransactionHandler{TracerReserver: capturing}

	txID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	handler.reserveTransaction(tracerCtx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		txID, decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, false)

	req := capturing.lastReq
	assert.Equal(t, txID, req.TransactionID)
	assert.Equal(t, "1000", req.Amount)
	assert.Equal(t, "BRL", req.Currency)
	assert.Equal(t, fixedReserveAccountID, req.Account.AccountID, "account scope must be the structured account, not a bare string")
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

func TestFirstSourceAccountID(t *testing.T) {
	balances := []*mmodel.Balance{
		{Alias: "@alice", Key: "default", AccountID: "acc-alice"},
		{Alias: "@bob", Key: "default", AccountID: "acc-bob"},
		{Alias: "@alice", Key: constant.OverdraftBalanceKey, AccountID: "acc-alice-overdraft"},
	}

	t.Run("resolves the first internal source account", func(t *testing.T) {
		got := firstSourceAccountID([]string{"@alice#default", "@bob#default"}, balances)
		assert.Equal(t, "acc-alice", got)
	})

	t.Run("skips the overdraft companion alias", func(t *testing.T) {
		got := firstSourceAccountID([]string{"@alice#overdraft", "@bob#default"}, balances)
		assert.Equal(t, "acc-bob", got, "companion sources must not be chosen as the account scope")
	})

	t.Run("returns empty when no internal source resolves", func(t *testing.T) {
		got := firstSourceAccountID([]string{"@external/BRL#default"}, balances)
		assert.Empty(t, got, "an external-only source has no internal account scope")
	})

	t.Run("empty inputs return empty", func(t *testing.T) {
		assert.Empty(t, firstSourceAccountID(nil, balances))
		assert.Empty(t, firstSourceAccountID([]string{"@alice#default"}, nil))
	})
}

func TestReservationTTLForStatus(t *testing.T) {
	assert.Equal(t, reservationTTLLongLived, reservationTTLForStatus(constant.PENDING))
	assert.Equal(t, reservationTTLDefault, reservationTTLForStatus(constant.APPROVED))
	assert.Equal(t, reservationTTLDefault, reservationTTLForStatus(constant.CREATED))
}

func TestConfirmReservations(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	t.Run("confirms every id", func(t *testing.T) {
		ids := []uuid.UUID{uuid.New(), uuid.New()}
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: ids})

		assert.Equal(t, ids, reserver.confirmedIDs)
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		handler := &TransactionHandler{TracerReserver: nil}
		handler.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: []uuid.UUID{uuid.New()}})
		// no panic, nothing to assert beyond not crashing
	})

	t.Run("transport failure does not propagate", func(t *testing.T) {
		reserver := &stubReserver{confirmErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		handler := &TransactionHandler{TracerReserver: reserver}

		// confirmReservations returns nothing; the contract is that it must not
		// panic and must attempt every id despite the error.
		handler.confirmReservations(ctx, sp, logger, reservationHandle{ReservationIDs: []uuid.UUID{uuid.New(), uuid.New()}})

		assert.Len(t, reserver.confirmedIDs, 2, "every id is attempted even when transport fails")
	})
}

func TestReleaseReservations(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	reserver := &stubReserver{releaseErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
	handler := &TransactionHandler{TracerReserver: reserver}

	handler.releaseReservations(ctx, sp, logger, reservationHandle{ReservationIDs: ids})

	assert.Equal(t, ids, reserver.releasedIDs, "release is attempted for every id despite transport failure")
}

func TestConfirmReservationsByTransaction(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	t.Run("commit confirms by transaction id", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservationsByTransaction(ctx, sp, logger, enforce, txID, false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTxns)
		assert.Empty(t, reserver.releasedTxns)
	})

	t.Run("advisory still confirms (lifecycle observed, never blocks)", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory}, txID, false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTxns)
	})

	t.Run("mode off does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, uuid.New(), false)

		assert.Empty(t, reserver.confirmedTxns, "mode=off must not confirm")
	})

	t.Run("empty mode does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservationsByTransaction(ctx, sp, logger, mmodel.TracerSettings{}, uuid.New(), false)

		assert.Empty(t, reserver.confirmedTxns)
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		handler := &TransactionHandler{TracerReserver: nil}
		handler.confirmReservationsByTransaction(ctx, sp, logger, enforce, uuid.New(), false)
		// no panic, nothing to assert beyond not crashing
	})

	t.Run("transport failure does not propagate", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{confirmByTxnErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		handler := &TransactionHandler{TracerReserver: reserver}

		// The contract is that the request still succeeds: the helper returns
		// nothing, swallows the error, and the caller proceeds.
		handler.confirmReservationsByTransaction(ctx, sp, logger, enforce, txID, false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTxns, "the transition is attempted despite transport failure")
	})

	t.Run("honored skip does not confirm even under enforce", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.confirmReservationsByTransaction(ctx, sp, logger, enforce, uuid.New(), true)

		assert.Empty(t, reserver.confirmedTxns, "an honored tracer skip must make zero ConfirmByTransaction")
	})
}

func TestReleaseReservationsByTransaction(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	t.Run("cancel releases by transaction id", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.releaseReservationsByTransaction(ctx, sp, logger, enforce, txID, false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.releasedTxns)
		assert.Empty(t, reserver.confirmedTxns)
	})

	t.Run("mode off does not call the tracer", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.releaseReservationsByTransaction(ctx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, uuid.New(), false)

		assert.Empty(t, reserver.releasedTxns)
	})

	t.Run("nil reserver is a no-op", func(t *testing.T) {
		handler := &TransactionHandler{TracerReserver: nil}
		handler.releaseReservationsByTransaction(ctx, sp, logger, enforce, uuid.New(), false)
	})

	t.Run("transport failure does not propagate", func(t *testing.T) {
		txID := uuid.New()
		reserver := &stubReserver{releaseByTxnErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.releaseReservationsByTransaction(ctx, sp, logger, enforce, txID, false)

		assert.Equal(t, []uuid.UUID{txID}, reserver.releasedTxns, "the transition is attempted despite transport failure")
	})

	t.Run("honored skip does not release even under enforce", func(t *testing.T) {
		reserver := &stubReserver{}
		handler := &TransactionHandler{TracerReserver: reserver}

		handler.releaseReservationsByTransaction(ctx, sp, logger, enforce, uuid.New(), true)

		assert.Empty(t, reserver.releasedTxns, "an honored tracer skip must make zero ReleaseByTransaction")
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

func (c *capturingReserver) Confirm(_ context.Context, _ uuid.UUID) error { return nil }
func (c *capturingReserver) Release(_ context.Context, _ uuid.UUID) error { return nil }

func (c *capturingReserver) ConfirmByTransaction(_ context.Context, _ uuid.UUID) error { return nil }
func (c *capturingReserver) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error { return nil }
