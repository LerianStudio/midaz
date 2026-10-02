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
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestHandoffReservedAtomicTransactionBatchExecution(t *testing.T) {
	t.Parallel()

	errHandoff := errors.New("handoff transport failed")

	cases := []struct {
		name        string
		handoffErr  error
		nilReserved bool
		wantRelease bool
	}{
		{name: "a failed hand-off releases every reservation", handoffErr: errHandoff, wantRelease: true},
		{name: "a successful hand-off keeps the reservations for the engine outcome"},
		{name: "a failed hand-off with nothing reserved releases nothing", handoffErr: errHandoff, nilReserved: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			answeredID := uuid.MustParse("01994f13-29b7-7000-8000-000000000301")
			reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){
				batchAllowed(answeredID),
				batchUnanswered,
			}}
			reserver.confirmByTxnOutcome = tracer.ConfirmOutcome{Confirmed: 1}

			run := atomicTransactionBatchTracerTestRun(2)
			run.ledgerSettings.Tracer.FailPosture = mmodel.TracerFailPostureOpen

			repository := &atomicTransactionBatchClaimRepositoryFake{handoffErr: tc.handoffErr}
			uc := &UseCase{TracerReserver: reserver, AtomicTransactionBatchIdempotencyRepo: repository}
			settles := withImmediateUnansweredSettles(t, uc)
			ctx, span, logger := anchorDeps()

			require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))

			reserved := run
			if tc.nilReserved {
				reserved = nil
			}

			err := uc.handoffReservedAtomicTransactionBatchExecution(ctx, span, logger, run, reserved)

			settles.drain()
			assert.Equal(t, 1, repository.handoffs)

			if tc.handoffErr != nil {
				require.ErrorIs(t, err, tc.handoffErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantRelease {
				assert.Equal(t, []uuid.UUID{answeredID}, reserver.released(), "the engine never ran, so the answered hold goes back by id")
				assert.Equal(t, []uuid.UUID{run.items[1].transactionID}, reserver.releasedTransactions(),
					"the unanswered hold goes back by transaction")
			} else {
				assert.Empty(t, reserver.released())
				assert.Empty(t, reserver.releasedTransactions())
			}

			assert.Empty(t, reserver.confirmed())
			assert.Empty(t, reserver.confirmedTransactions())
		})
	}
}

func TestAtomicTransactionBatchReservations_UnansweredSettleFollowsItemLedgerSettings(t *testing.T) {
	t.Parallel()

	enforceOpen := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}
	off := mmodel.TracerSettings{Mode: mmodel.TracerModeOff}

	cases := []struct {
		name       string
		runTracer  mmodel.TracerSettings
		itemTracer mmodel.TracerSettings
		wantItem   bool
		wantRun    bool
	}{
		{name: "an item ledger with the tracer off is never settled under an enforcing run", runTracer: enforceOpen, itemTracer: off, wantRun: true},
		{name: "an enforcing item ledger is settled under a run with the tracer off", runTracer: off, itemTracer: enforceOpen, wantItem: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reserver := &batchItemReserver{answers: []func() (*tracer.ReserveResult, error){batchUnanswered, batchUnanswered}}
			reserver.confirmByTxnOutcome = tracer.ConfirmOutcome{Confirmed: 1}

			run := atomicTransactionBatchTracerTestRun(2)
			run.organizationID = uuid.MustParse("01994f13-29b7-7000-8000-000000000311")
			run.ledgerID = uuid.MustParse("01994f13-29b7-7000-8000-000000000312")
			run.ledgerSettings.Tracer = tc.runTracer

			crossLedgerItem := &run.items[0]
			crossLedgerItem.organizationID = uuid.MustParse("01994f13-29b7-7000-8000-000000000313")
			crossLedgerItem.ledgerID = uuid.MustParse("01994f13-29b7-7000-8000-000000000314")
			crossLedgerItem.ledgerSettings.Tracer = tc.itemTracer

			require.Equal(t, tc.itemTracer, run.itemLedgerSettings(crossLedgerItem).Tracer)
			require.Equal(t, tc.runTracer, run.itemLedgerSettings(&run.items[1]).Tracer)

			uc := &UseCase{TracerReserver: reserver}
			settles := withImmediateUnansweredSettles(t, uc)
			ctx, span, logger := anchorDeps()

			require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))
			uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationKnownSuccess)
			settles.drain()

			var want []uuid.UUID
			if tc.wantItem {
				want = append(want, run.items[0].transactionID)
			}

			if tc.wantRun {
				want = append(want, run.items[1].transactionID)
			}

			assert.Equal(t, want, reserver.confirmedTransactions())
			assert.Empty(t, reserver.releasedTransactions())
		})
	}
}

func TestCreateAtomicTransactionBatchV2_HandoffFailureReleasesReservations(t *testing.T) {
	errHandoff := errors.New("handoff transport failed")
	repository := &atomicTransactionBatchClaimRepositoryFake{handoffErr: errHandoff}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.ErrorIs(t, err, errHandoff)
	assert.Nil(t, result)
	assert.Equal(t, 1, repository.handoffs)
	assert.Empty(t, engine.executions, "a failed hand-off never reaches the engine")

	requests, confirmed, released := reserver.snapshot()
	assert.Equal(t, transactionIDs, atomicTransactionBatchTracerRequestIDs(requests))
	assert.Equal(t, atomicTransactionBatchExecutionReservationIDs(), released, "every held reservation goes back, in request order")
	assert.Empty(t, confirmed)
}

func TestTransitionCrossLedgerGroupV2_HandoffFailureReleasesDestinationReservations(t *testing.T) {
	errHandoff := errors.New("handoff transport failed")
	destinationReservationID := uuid.MustParse("0199a500-0000-7000-8000-000000000031")

	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed}

	uc, repo, engine, target, in, group := newCrossLedgerLifecycleFixtureWith(t, constant.APPROVED, crossLedgerLifecycleSetup{
		settingsA: &settings,
		settingsB: &settings,
	})
	uc.AtomicTransactionBatchIdempotencyRepo = &atomicTransactionBatchClaimRepositoryFake{handoffErr: errHandoff}

	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{destinationReservationID}}}
	uc.TracerReserver = reserver
	settles := withImmediateUnansweredSettles(t, uc)

	repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)

	result, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, constant.APPROVED)
	require.ErrorIs(t, err, errHandoff)
	assert.Nil(t, result)
	settles.drain()

	assert.Empty(t, engine.executions, "a failed hand-off never reaches the engine")
	require.Len(t, reserver.reserveRequests(), 1, "only the destination reserves at commit")
	assert.Equal(t, []uuid.UUID{destinationReservationID}, reserver.released(), "the destination hold goes back")
	assert.Empty(t, reserver.releasedTransactions(), "the pending origins keep their holds")
	assert.Empty(t, reserver.confirmedTransactions())
	assert.Empty(t, reserver.confirmed())
}
