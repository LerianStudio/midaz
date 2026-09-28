// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"sync"
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

type atomicTransactionBatchTracerFake struct {
	mu sync.Mutex

	results   []*tracer.ReserveResult
	requests  []tracer.ReserveRequest
	confirmed []uuid.UUID
	released  []uuid.UUID
}

func (fake *atomicTransactionBatchTracerFake) Reserve(
	_ context.Context,
	request tracer.ReserveRequest,
) (*tracer.ReserveResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.requests = append(fake.requests, request)
	index := len(fake.requests) - 1
	if index >= len(fake.results) {
		return &tracer.ReserveResult{}, nil
	}

	return fake.results[index], nil
}

func (fake *atomicTransactionBatchTracerFake) Confirm(_ context.Context, reservationID uuid.UUID) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.confirmed = append(fake.confirmed, reservationID)

	return nil
}

func (fake *atomicTransactionBatchTracerFake) Release(_ context.Context, reservationID uuid.UUID) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.released = append(fake.released, reservationID)

	return nil
}

func (fake *atomicTransactionBatchTracerFake) ConfirmByTransaction(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (fake *atomicTransactionBatchTracerFake) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (fake *atomicTransactionBatchTracerFake) snapshot() (
	[]tracer.ReserveRequest,
	[]uuid.UUID,
	[]uuid.UUID,
) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return append([]tracer.ReserveRequest(nil), fake.requests...),
		append([]uuid.UUID(nil), fake.confirmed...),
		append([]uuid.UUID(nil), fake.released...)
}

func TestReserveAtomicTransactionBatch_DenialReleasesPriorReservationsInOrder(t *testing.T) {
	firstReservationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000091")
	secondReservationIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000092"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000093"),
	}
	fake := &atomicTransactionBatchTracerFake{results: []*tracer.ReserveResult{
		{ReservationIDs: []uuid.UUID{firstReservationID}},
		{ReservationIDs: secondReservationIDs},
		{Denied: true},
	}}
	run := atomicTransactionBatchTracerTestRun(4)
	uc := &UseCase{TracerReserver: fake}
	ctx, span, logger := anchorDeps()

	err := uc.reserveAtomicTransactionBatch(ctx, span, logger, run)
	require.Error(t, err)

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
		firstReservationID,
		secondReservationIDs[0],
		secondReservationIDs[1],
	}, released)
	assert.Empty(t, run.items[2].tracerReservation.ReservationIDs)
	assert.Empty(t, run.items[3].tracerReservation.ReservationIDs)
}

func TestAtomicTransactionBatchReservations_SkipUnknownAndSuccess(t *testing.T) {
	firstReservationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000a1")
	secondReservationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000a2")
	fake := &atomicTransactionBatchTracerFake{results: []*tracer.ReserveResult{
		{ReservationIDs: []uuid.UUID{firstReservationID}},
		{ReservationIDs: []uuid.UUID{secondReservationID}},
	}}
	run := atomicTransactionBatchTracerTestRun(3)
	run.items[0].honoredTracerSkip = true
	uc := &UseCase{TracerReserver: fake}
	ctx, span, logger := anchorDeps()

	require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))
	requests, confirmed, released := fake.snapshot()
	assert.Equal(t, []uuid.UUID{
		run.items[1].transactionID,
		run.items[2].transactionID,
	}, atomicTransactionBatchTracerRequestIDs(requests))
	assert.Empty(t, run.items[0].tracerReservation.ReservationIDs)
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
	_, confirmed, released = fake.snapshot()
	assert.Equal(t, []uuid.UUID{firstReservationID, secondReservationID}, confirmed)
	assert.Empty(t, released)
}

func TestAtomicTransactionBatchReservationSettlement_RetriesTransportFailure(t *testing.T) {
	withFastSharedRetrier(t)

	reserver := &scriptedReserver{confirm: failNTimes(1)}
	uc := &UseCase{TracerReserver: reserver}
	run := atomicTransactionBatchTracerTestRun(1)
	run.items[0].tracerReservation = reservationHandle{
		ReservationIDs: []uuid.UUID{uuid.MustParse("01994f13-29b7-7000-8000-0000000000b1")},
		TransactionID:  run.items[0].transactionID,
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

	attempts, delivered := reserver.attempts()
	assert.True(t, delivered)
	assert.Equal(t, 2, attempts, "the first failed confirm must be redelivered by the bounded retrier")
}

func TestAtomicContextBatchPreservesHoldUntilTerminalOutcome(t *testing.T) {
	for _, action := range []string{reservationActionConfirm, reservationActionRelease} {
		t.Run(action, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockContextTracerReserver(ctrl)
			coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
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
	coordinator, err := NewContextTracerCoordinator(NewMockContextTracerReserver(ctrl), NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
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
		ledgerSettings: mmodel.LedgerSettings{
			Tracer: mmodel.TracerSettings{
				Mode:        mmodel.TracerModeEnforce,
				FailPosture: mmodel.TracerFailPostureClosed,
			},
		},
		items: make([]atomicTransactionBatchItemRun, itemCount),
	}
	for index := range run.items {
		transactionID := uuid.NewSHA1(
			uuid.MustParse("01994f13-29b7-7000-8000-0000000000d0"),
			[]byte{byte(index)},
		)
		alias := "@source-" + decimal.NewFromInt(int64(index)).String()
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
				Asset: "BRL",
				Value: decimal.NewFromInt(int64(index + 1)),
			}},
			validate: &mtransaction.Responses{Sources: []string{alias + "#" + constant.DefaultBalanceKey}},
			prepared: enginePreparedTransaction{pool: EngineSnapshotPool{ExplicitBalances: []*mmodel.Balance{{
				Alias:     alias,
				Key:       constant.DefaultBalanceKey,
				AccountID: "account-" + decimal.NewFromInt(int64(index)).String(),
			}}}},
		}
	}

	return run
}

func atomicTransactionBatchTracerRequestIDs(requests []tracer.ReserveRequest) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(requests))
	for index := range requests {
		transactionIDs[index] = requests[index].TransactionID
	}

	return transactionIDs
}
