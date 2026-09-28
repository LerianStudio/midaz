// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// atomicTransactionBatchTracerFake is the contextual Tracer the batch tests
// script, one decision per Reserve call in request order.
type atomicTransactionBatchTracerFake = stubContextTracer

func TestReserveAtomicTransactionBatch_DenialReleasesPriorReservationsInOrder(t *testing.T) {
	withFastSharedRetrier(t)

	fake := &atomicTransactionBatchTracerFake{decisions: []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionAllow, tracercontract.DecisionDeny}}
	run := atomicTransactionBatchTracerTestRun(4)
	uc := &UseCase{ContextTracer: fake.coordinatorFor(t)}
	ctx, span, logger := anchorDeps()

	err := uc.reserveAtomicTransactionBatch(ctx, span, logger, run)
	require.Error(t, err)
	sharedReservationRetrier.wait()

	var business pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &business))
	assert.Equal(t, constant.ErrTransactionReservationDenied.Error(), business.Code)
	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[2]",
		Message:  "tracer reservation rejected",
	}}, carrier.FieldErrors())

	requests, confirmed, released := fake.snapshot()
	assert.Equal(t, []uuid.UUID{
		run.items[0].transactionID,
		run.items[1].transactionID,
		run.items[2].transactionID,
	}, atomicTransactionBatchTracerRequestIDs(requests))
	assert.Empty(t, confirmed)
	assert.Equal(t, []uuid.UUID{
		run.items[2].transactionID,
		run.items[0].transactionID,
		run.items[1].transactionID,
	}, released, "the denied admission is released first, then every earlier admission in request order")
	assert.Nil(t, run.items[2].tracerReservation.ContextAttempt)
	assert.Nil(t, run.items[3].tracerReservation.ContextAttempt)
}

func TestAtomicTransactionBatchReservations_SkipUnknownAndSuccess(t *testing.T) {
	withFastSharedRetrier(t)

	fake := &atomicTransactionBatchTracerFake{}
	run := atomicTransactionBatchTracerTestRun(3)
	run.items[0].honoredTracerSkip = true
	uc := &UseCase{ContextTracer: fake.coordinatorFor(t)}
	ctx, span, logger := anchorDeps()

	require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))
	requests, confirmed, released := fake.snapshot()
	assert.Equal(t, []uuid.UUID{
		run.items[1].transactionID,
		run.items[2].transactionID,
	}, atomicTransactionBatchTracerRequestIDs(requests))
	require.NotNil(t, run.items[0].tracerReservation.ContextAttempt)
	assert.True(t, run.items[0].tracerReservation.ContextAttempt.Skipped)
	assert.Empty(t, confirmed)
	assert.Empty(t, released)

	uc.settleAtomicTransactionBatchReservations(
		ctx,
		span,
		logger,
		run,
		atomicTransactionBatchReservationUnknown,
	)
	_, confirmed, released = fake.snapshot()
	assert.Empty(t, confirmed, "an indeterminate accounting result must retain every reservation")
	assert.Empty(t, released, "an indeterminate accounting result must not return capacity")

	uc.settleAtomicTransactionBatchReservations(
		ctx,
		span,
		logger,
		run,
		atomicTransactionBatchReservationKnownSuccess,
	)
	sharedReservationRetrier.wait()
	_, confirmed, released = fake.snapshot()
	assert.Equal(t, []uuid.UUID{run.items[1].transactionID, run.items[2].transactionID}, confirmed, "a skipped member is not completed")
	assert.Empty(t, released)
}

func TestAtomicTransactionBatchReservationSettlement_RetriesTransportFailure(t *testing.T) {
	withFastSharedRetrier(t)

	run := atomicTransactionBatchTracerTestRun(1)
	transactionID := run.items[0].transactionID
	coordinator, client := newCompletionTestCoordinator(t)
	gomock.InOrder(
		client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(nil, tracer.ErrTracerUnavailable),
		client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "CONFIRMED"), nil),
	)
	uc := &UseCase{ContextTracer: coordinator}
	run.items[0].tracerReservation = reservationHandle{
		ContextAttempt: &ContextTracerAttempt{Dispatched: true, Settings: run.ledgerSettings.Tracer},
		TransactionID:  transactionID,
		Amount:         run.items[0].input.Send.Value,
		Asset:          run.items[0].input.Send.Asset,
	}
	ctx, span, logger := anchorDeps()

	uc.settleAtomicTransactionBatchReservations(
		ctx,
		span,
		logger,
		run,
		atomicTransactionBatchReservationKnownSuccess,
	)
	sharedReservationRetrier.wait()
}

func TestAtomicContextBatchPreservesHoldUntilTerminalOutcome(t *testing.T) {
	for _, action := range []string{reservationActionConfirm, reservationActionRelease} {
		t.Run(action, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockContextTracerClient(ctrl)
			coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, fixedTracerClock)
			require.NoError(t, err)
			uc := &UseCase{ContextTracer: coordinator}
			run := atomicTransactionBatchTracerTestRun(2)
			run.items[0].status = constant.APPROVED
			run.items[1].status = constant.PENDING
			for i := range run.items {
				run.items[i].tracerReservation = reservationHandle{ContextAttempt: &ContextTracerAttempt{Dispatched: true, Settings: run.ledgerSettings.Tracer, Result: &tracercontract.ReserveResult{Decision: tracercontract.DecisionAllow}}, TransactionID: run.items[i].transactionID}
			}
			ctx, span, logger := anchorDeps()
			// Creation settles only the posted item. Nothing is sent for the
			// hold, so a later commit OR cancel remains possible.
			client.EXPECT().ConfirmByTransaction(gomock.Any(), run.items[0].transactionID).Return(completionResult(run.items[0].transactionID, "CONFIRMED"), nil)
			uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationKnownSuccess)
			pending := run.items[1].transactionID
			if action == reservationActionConfirm {
				client.EXPECT().ConfirmByTransaction(gomock.Any(), pending).Return(completionResult(pending, "CONFIRMED"), nil)
				uc.confirmReservationsByTransaction(ctx, span, logger, run.ledgerSettings.Tracer, run.items[1].tracerReservation, false)
			} else {
				client.EXPECT().ReleaseByTransaction(gomock.Any(), pending).Return(completionResult(pending, "RELEASED"), nil)
				uc.releaseReservationsByTransaction(ctx, span, logger, run.ledgerSettings.Tracer, run.items[1].tracerReservation, false)
			}
		})
	}
}

func TestAtomicContextBatchUnknownOutcomeLeavesReservations(t *testing.T) {
	ctrl := gomock.NewController(t)
	coordinator, err := NewContextTracerCoordinator(NewMockContextTracerClient(ctrl), NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, fixedTracerClock)
	require.NoError(t, err)
	uc := &UseCase{ContextTracer: coordinator}
	run := atomicTransactionBatchTracerTestRun(1)
	run.items[0].tracerReservation = reservationHandle{ContextAttempt: &ContextTracerAttempt{Dispatched: true, Settings: run.ledgerSettings.Tracer}, TransactionID: run.items[0].transactionID}
	require.True(t, atomicTransactionBatchHoldsReservations(run))
	ctx, span, _ := anchorDeps()
	logger := &capturingLogger{}
	uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationUnknown)
	warnings := logger.atLevelOrMoreSevere(libLog.LevelWarn)
	require.Len(t, warnings, 1)
	require.Equal(t, "Atomic transaction batch outcome unknown, reservations left to tracer TTL", warnings[0].Msg)
	require.Contains(t, warnings[0].Fields, run.executionID.String())
	run.items[0].tracerReservation.ContextAttempt.Dispatched = false
	require.False(t, atomicTransactionBatchHoldsReservations(run), "an admission that never reached Tracer holds nothing")
}

func TestAtomicTransactionBatchFailureSettlementReleasesWhenEngineNeverRan(t *testing.T) {
	require.Equal(t, atomicTransactionBatchReservationConfirmedAbort, atomicTransactionBatchFailureSettlement(EngineExecutionOutcome{}))
	require.Equal(t, atomicTransactionBatchReservationUnknown, atomicTransactionBatchFailureSettlement(EngineExecutionOutcome{Executed: true}))
}

func atomicTransactionBatchTracerTestRun(itemCount int) *atomicTransactionBatchRun {
	run := &atomicTransactionBatchRun{
		organizationID: uuid.MustParse("01994f13-29b7-7000-8000-0000000000c1"),
		ledgerID:       uuid.MustParse("01994f13-29b7-7000-8000-0000000000c2"),
		ledgerSettings: mmodel.LedgerSettings{
			Tracer: mmodel.TracerSettings{
				Mode:           mmodel.TracerModeEnforce,
				FailPosture:    mmodel.TracerFailPostureClosed,
				ValidationMode: string(tracercontract.ValidationLimits),
				TimeoutMs:      stubContextTracerTimeoutMs,
			},
		},
		items: make([]atomicTransactionBatchItemRun, itemCount),
	}
	for index := range run.items {
		transactionID := uuid.NewSHA1(
			uuid.MustParse("01994f13-29b7-7000-8000-0000000000d0"),
			[]byte{byte(index)},
		)
		suffix := decimal.NewFromInt(int64(index)).String()
		source, destination := "@source-"+suffix, "@destination-"+suffix
		sourceLeg, destinationLeg := "0#"+source+"#"+constant.DefaultBalanceKey, "0#"+destination+"#"+constant.DefaultBalanceKey
		value := decimal.NewFromInt(int64(index + 1))
		run.items[index] = atomicTransactionBatchItemRun{
			index:         index,
			transactionID: transactionID,
			transactionDate: time.Date(
				2026,
				time.September,
				16,
				18,
				index,
				0,
				0,
				time.UTC,
			),
			status: constant.CREATED,
			input: mtransaction.Transaction{Send: mtransaction.Send{
				Asset:      "BRL",
				Value:      value,
				Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: sourceLeg}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: destinationLeg}}},
			}},
			validate: &mtransaction.Responses{
				Sources: []string{source + "#" + constant.DefaultBalanceKey},
				From:    map[string]mtransaction.Amount{sourceLeg: {Value: value}},
				To:      map[string]mtransaction.Amount{destinationLeg: {Value: value}},
			},
			prepared: enginePreparedTransaction{pool: EngineSnapshotPool{ExplicitBalances: []*mmodel.Balance{
				{Alias: source, Key: constant.DefaultBalanceKey, AccountID: uuid.NewSHA1(transactionID, []byte("source")).String(), AssetCode: "BRL", AccountType: "deposit"},
				{Alias: destination, Key: constant.DefaultBalanceKey, AccountID: uuid.NewSHA1(transactionID, []byte("destination")).String(), AssetCode: "BRL", AccountType: "deposit"},
			}}},
		}
	}

	return run
}

func atomicTransactionBatchTracerRequestIDs(requests []tracercontract.ReserveRequest) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(requests))
	for index := range requests {
		transactionIDs[index] = requests[index].TransactionID
	}

	return transactionIDs
}
