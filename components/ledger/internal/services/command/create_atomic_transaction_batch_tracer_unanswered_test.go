// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// batchItemReserver scripts one reserve answer per batch item, in request order,
// and records by-id and by-transaction settles.
type batchItemReserver struct {
	stubReserver

	answers []func() (*tracer.ReserveResult, error)
}

func (r *batchItemReserver) Reserve(_ context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	r.mu.Lock()
	index := r.reserveCalls
	r.reserveCalls++
	r.requests = append(r.requests, req)
	r.mu.Unlock()

	if index >= len(r.answers) {
		return &tracer.ReserveResult{}, nil
	}

	return r.answers[index]()
}

func batchAllowed(ids ...uuid.UUID) func() (*tracer.ReserveResult, error) {
	return func() (*tracer.ReserveResult, error) { return &tracer.ReserveResult{ReservationIDs: ids}, nil }
}

func batchUnanswered() (*tracer.ReserveResult, error) { return nil, errReserveDeadline }

func TestReserveAtomicTransactionBatch_FailClosedReleasesUnansweredItem(t *testing.T) {
	t.Parallel()

	firstReservationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000e1")
	reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){
		batchAllowed(firstReservationID),
		batchUnanswered,
	}}
	run := atomicTransactionBatchTracerTestRun(3)
	uc := &UseCase{TracerReserver: reserver}
	settles := withImmediateUnansweredSettles(t, uc)
	ctx, span, logger := anchorDeps()

	err := uc.reserveAtomicTransactionBatch(ctx, span, logger, run)
	require.Error(t, err)
	settles.drain()

	var unavailable pkg.ServiceUnavailableError
	require.True(t, errors.As(err, &unavailable))
	assert.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)

	assert.Equal(t, []uuid.UUID{firstReservationID}, reserver.released(), "the earlier item's handle is released by id")
	assert.Equal(t, []uuid.UUID{run.items[1].transactionID}, reserver.releasedTransactions(),
		"the item whose reserve went unanswered is released by transaction")
	assert.Empty(t, reserver.confirmed())
	assert.Empty(t, reserver.confirmedTransactions())
}

func TestAtomicTransactionBatchReservations_SettleUnansweredItem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		settlement  atomicTransactionBatchReservationSettlement
		pending     bool
		wantConfirm bool
		wantRelease bool
	}{
		{name: "applied confirms the unanswered item by transaction", settlement: atomicTransactionBatchReservationKnownSuccess, wantConfirm: true},
		{name: "a confirmed abort releases the unanswered item by transaction", settlement: atomicTransactionBatchReservationConfirmedAbort, wantRelease: true},
		{name: "an unknown outcome settles nothing", settlement: atomicTransactionBatchReservationUnknown},
		{name: "a pending item leaves the settle to commit or cancel", settlement: atomicTransactionBatchReservationKnownSuccess, pending: true},
		{name: "a pending item still releases on a confirmed abort", settlement: atomicTransactionBatchReservationConfirmedAbort, pending: true, wantRelease: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			firstReservationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000f1")
			reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){
				batchAllowed(firstReservationID),
				batchUnanswered,
			}}
			reserver.confirmByTxnOutcome = tracer.ConfirmOutcome{Confirmed: 1}
			run := atomicTransactionBatchTracerTestRun(2)
			run.ledgerSettings.Tracer.FailPosture = mmodel.TracerFailPostureOpen

			if tc.pending {
				run.items[1].status = constant.PENDING
			}

			uc := &UseCase{TracerReserver: reserver}
			settles := withImmediateUnansweredSettles(t, uc)
			ctx, span, logger := anchorDeps()

			require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))
			assert.True(t, run.items[1].tracerReservation.Unanswered)
			assert.Empty(t, reserver.confirmedTransactions())
			assert.Empty(t, reserver.releasedTransactions())

			uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, tc.settlement)
			settles.drain()

			if tc.wantConfirm {
				assert.Equal(t, []uuid.UUID{run.items[1].transactionID}, reserver.confirmedTransactions())
				assert.Equal(t, []uuid.UUID{firstReservationID}, reserver.confirmed(), "the answered item keeps its by-id confirm")
			} else {
				assert.Empty(t, reserver.confirmedTransactions())
			}

			if tc.wantRelease {
				assert.Equal(t, []uuid.UUID{run.items[1].transactionID}, reserver.releasedTransactions())
				assert.Equal(t, []uuid.UUID{firstReservationID}, reserver.released(), "the answered item keeps its by-id release")
			} else {
				assert.Empty(t, reserver.releasedTransactions())
			}
		})
	}
}

func TestAtomicTransactionBatchReservations_UnansweredHonorsSkipAndMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(run *atomicTransactionBatchRun)
	}{
		{name: "an honored skip", mutate: func(run *atomicTransactionBatchRun) { run.items[0].honoredTracerSkip = true }},
		{name: "mode off", mutate: func(run *atomicTransactionBatchRun) { run.ledgerSettings.Tracer.Mode = mmodel.TracerModeOff }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){batchUnanswered}}
			run := atomicTransactionBatchTracerTestRun(1)
			run.ledgerSettings.Tracer.FailPosture = mmodel.TracerFailPostureOpen
			tc.mutate(run)

			uc := &UseCase{TracerReserver: reserver}
			settles := withImmediateUnansweredSettles(t, uc)
			ctx, span, logger := anchorDeps()

			require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))
			assert.Zero(t, reserver.reserves(), "no reserve is sent, so nothing can go unanswered")
			assert.False(t, run.items[0].tracerReservation.Unanswered)

			uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationKnownSuccess)
			uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationConfirmedAbort)
			settles.drain()

			assert.Empty(t, reserver.confirmedTransactions())
			assert.Empty(t, reserver.releasedTransactions())
		})
	}
}

// forbiddenEngine fails the test if the accounting engine is ever asked to run.
type forbiddenEngine struct {
	t *testing.T
}

func (e forbiddenEngine) Execute(context.Context, EngineExecution) (*accounting.ExecutionResult, error) {
	e.t.Error("the accounting engine must not run")

	return nil, errors.New("the accounting engine must not run")
}

func TestExecuteAtomicTransactionBatch_EngineNeverRanReleasesReservations(t *testing.T) {
	t.Parallel()

	firstReservationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000101")
	reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){
		batchAllowed(firstReservationID),
		batchUnanswered,
	}}
	run := atomicTransactionBatchTracerTestRun(2)
	run.ledgerSettings.Tracer.FailPosture = mmodel.TracerFailPostureOpen

	uc := &UseCase{TracerReserver: reserver, Engine: forbiddenEngine{t: t}}
	settles := withImmediateUnansweredSettles(t, uc)
	ctx, span, logger := anchorDeps()

	require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	outcome, err := uc.executeAtomicTransactionBatch(cancelled, span, logger, run, PreparedEngineExecution{}, nil)
	require.ErrorIs(t, err, context.Canceled)
	settles.drain()
	assert.False(t, outcome.Executed)

	assert.Equal(t, []uuid.UUID{firstReservationID}, reserver.released(), "the answered item is released by id")
	assert.Equal(t, []uuid.UUID{run.items[1].transactionID}, reserver.releasedTransactions(),
		"the item whose reserve went unanswered is released by transaction")
	assert.Empty(t, reserver.confirmed())
	assert.Empty(t, reserver.confirmedTransactions())
}
