// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type refusingAtomicTransactionBatchEngine struct {
	transactionIndex int
	executions       []EngineExecution
}

func (engine *refusingAtomicTransactionBatchEngine) Execute(
	_ context.Context,
	execution EngineExecution,
) (*accounting.ExecutionResult, error) {
	engine.executions = append(engine.executions, execution)
	transaction := execution.Execution.Transactions[engine.transactionIndex]

	return nil, &accounting.Failure{
		Code:             accounting.FailureInsufficientFunds,
		TransactionIndex: engine.transactionIndex,
		PostingIndex:     0,
		BalanceRef:       transaction.Postings[0].BalanceRef,
	}
}

type indeterminateAtomicTransactionBatchError struct {
	cause error
}

func (err *indeterminateAtomicTransactionBatchError) Error() string {
	return "indeterminate atomic transaction batch execution: " + err.cause.Error()
}

func (err *indeterminateAtomicTransactionBatchError) Unwrap() error {
	return err.cause
}

func (err *indeterminateAtomicTransactionBatchError) EngineFailureCode() string {
	return "connection_lost"
}

func (err *indeterminateAtomicTransactionBatchError) OutcomeIndeterminate() bool {
	return true
}

func TestCreateAtomicTransactionBatchV2_ExecutesOneOrderedEngineRequest(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(
		t,
		repository,
		engine,
		reserver,
	)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, engine.executions, 1)

	execution := engine.executions[0]
	assert.Equal(t, executionID, execution.Execution.ExecutionID)
	assert.Equal(t, transactionIDs, atomicTransactionBatchEngineTransactionIDs(execution))
	assert.Equal(t, transactionIDs, atomicTransactionBatchGuardIDs(execution.Guards))
	assert.Equal(t, transactionIDs, atomicTransactionBatchCompletionRecordIDs(execution.CompletionPlans))
	assert.Equal(t, 1, repository.transitions)
	assert.Equal(t, txRedis.AtomicTransactionBatchStatePrepared, repository.transition.State)
	assert.Equal(t, transactionIDs, repository.transition.TransactionIDs)
	assert.Equal(t, 1, repository.handoffs)
	assert.Equal(t, txRedis.AtomicTransactionBatchStateApplied, repository.handoff.State)
	require.NotNil(t, repository.handoff.ExecutionID)
	assert.Equal(t, executionID, *repository.handoff.ExecutionID)
	assert.Equal(t, transactionIDs, repository.handoff.TransactionIDs)
	assert.Zero(t, repository.aborts)
	assert.Zero(t, repository.deletes)

	requests, confirmed, released := reserver.snapshot()
	assert.Equal(t, transactionIDs, atomicTransactionBatchTracerRequestIDs(requests))
	assert.Equal(t, atomicTransactionBatchExecutionReservationIDs(), confirmed)
	assert.Empty(t, released)
	assert.True(t, engine.sawAdmissionSink)
	require.NotEmpty(t, engine.admissionTokens)
	for _, token := range engine.admissionTokens {
		assert.NotEmpty(t, token)
	}
	assert.Zero(t, atomicTransactionBatchProtectionStore(t, uc).ownedAccounts())
}

func TestAtomicTransactionBatchRetentionSecondsSupportsHeaderAndDurationConventions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int64(300), atomicTransactionBatchRetentionSeconds(time.Duration(300)))
	assert.Equal(t, int64(60), atomicTransactionBatchRetentionSeconds(time.Minute))
}

func TestCreateAtomicTransactionBatchV2_ConfirmedRefusalAbortsAndCorrelatesFirstFailure(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &refusingAtomicTransactionBatchEngine{transactionIndex: 1}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(
		t,
		repository,
		engine,
		reserver,
	)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.Error(t, err)
	assert.Nil(t, result)
	require.Len(t, engine.executions, 1, "a deterministic refusal must not retry accounting")

	var business pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &business))
	assert.Equal(t, constant.ErrInsufficientFunds.Error(), business.Code)
	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[1]",
		Message:  "accounting execution refused",
	}}, carrier.FieldErrors())

	assert.Equal(t, 1, repository.aborts)
	assert.Equal(t, executionID, repository.abortExecutionID)
	assert.Equal(t, transactionIDs, repository.abortTransactionIDs)
	assert.Zero(t, repository.deletes)
	_, confirmed, released := reserver.snapshot()
	assert.Empty(t, confirmed)
	assert.Equal(t, atomicTransactionBatchExecutionReservationIDs(), released)
	assert.Zero(t, atomicTransactionBatchProtectionStore(t, uc).ownedAccounts())
}

func TestCreateAtomicTransactionBatchV2_PrecommitTechnicalRefusalReleasesClaim(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	cause := errors.New("dependency evidence is absent")
	engine := &scriptedEngine{responses: []engineResponse{{err: testEngineTechnicalError{
		code: "dependency_evidence_missing", cause: cause,
	}}}}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.Error(t, err)
	assert.Nil(t, result)
	require.Len(t, engine.requests, 1)
	assert.Equal(t, 1, repository.aborts, "a definite precommit refusal must release the idempotency claim")
	assert.Equal(t, executionID, repository.abortExecutionID)
	assert.Equal(t, transactionIDs, repository.abortTransactionIDs)
	_, confirmed, released := reserver.snapshot()
	assert.Empty(t, confirmed)
	assert.Equal(t, atomicTransactionBatchExecutionReservationIDs(), released)
}

func TestCreateAtomicTransactionBatchV2_ProtectedRefusalRetainsIdentityAndReservations(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{
		abortErr: txRedis.ErrAtomicTransactionBatchRefusalProtected,
	}
	engine := &refusingAtomicTransactionBatchEngine{transactionIndex: 0}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, _, _ := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.ErrorIs(t, err, txRedis.ErrAtomicTransactionBatchRefusalProtected)
	assert.Nil(t, result)
	require.Len(t, engine.executions, 1, "protected refusal must not retry accounting")
	assert.Equal(t, 1, repository.aborts)
	assert.Zero(t, repository.deletes)
	_, confirmed, released := reserver.snapshot()
	assert.Empty(t, confirmed)
	assert.Empty(t, released)
	assert.Zero(t, atomicTransactionBatchProtectionStore(t, uc).ownedAccounts())
}

func TestCreateAtomicTransactionBatchV2_TransitionFailureAbortsBeforeTracerAndAccounting(t *testing.T) {
	cause := errors.New("redis transition unavailable")
	repository := &atomicTransactionBatchClaimRepositoryFake{transitionErr: cause}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, _, _ := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.ErrorIs(t, err, cause)
	assert.Nil(t, result)
	assert.Equal(t, 1, repository.transitions)
	assert.Zero(t, repository.handoffs)
	assert.Zero(t, repository.aborts)
	assert.Equal(t, 1, repository.deletes)
	assert.Empty(t, engine.executions)
	requests, confirmed, released := reserver.snapshot()
	assert.Empty(t, requests)
	assert.Empty(t, confirmed)
	assert.Empty(t, released)
}

func TestCreateAtomicTransactionBatchV2_IndeterminateOutcomeRetainsProtectionWithoutRetry(t *testing.T) {
	cause := errors.New("connection closed after write")
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &scriptedEngine{responses: []engineResponse{{
		err: &indeterminateAtomicTransactionBatchError{cause: cause},
	}}}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, _, _ := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.ErrorIs(t, err, cause)
	assert.Nil(t, result)
	require.Len(t, engine.requests, 1, "an unknown outcome must never retry accounting")
	assert.Equal(t, 1, repository.handoffs)
	assert.Zero(t, repository.aborts)
	assert.Zero(t, repository.deletes)
	_, confirmed, released := reserver.snapshot()
	assert.Empty(t, confirmed)
	assert.Empty(t, released)
	assert.Equal(t, 4, atomicTransactionBatchProtectionStore(t, uc).ownedAccounts())
}

func atomicTransactionBatchExecutionFixture(
	t *testing.T,
	repository *atomicTransactionBatchClaimRepositoryFake,
	engine Engine,
	reserver *atomicTransactionBatchTracerFake,
) (*UseCase, CreateAtomicTransactionBatchV2Input, []uuid.UUID, uuid.UUID) {
	t.Helper()

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000e1")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000e2")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000e3")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000e4"),
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000e5"),
	}
	executionID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000e6")
	settings := mmodel.LedgerSettings{}
	settings.Tracer = mmodel.TracerSettings{
		Mode:        mmodel.TracerModeEnforce,
		FailPosture: mmodel.TracerFailPostureClosed,
	}
	reader := &atomicTransactionBatchSettingsReader{
		settings:        settings,
		protectionStore: newAccountClosingMarkerStore(),
		balances: []*mmodel.Balance{
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-0000000000e7", "@source-0", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-0000000000e8", "@destination-0", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-0000000000e9", "@source-1", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-0000000000ea", "@destination-1", "BRL"),
		},
	}
	uc := &UseCase{
		TransactionReader:                     reader,
		AtomicTransactionBatchIdempotencyRepo: repository,
		Engine:                                engine,
		AppliedTransactionCompleter: &createAppliedTransactionCompleter{
			outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
		},
		EngineRecoveryAcknowledger: &recordingEngineRecoveryAcknowledger{},
		TracerReserver:             reserver,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			transactionIDs[0],
			transactionIDs[1],
			executionID,
		),
		Clock: func() time.Time {
			return time.Date(2026, time.September, 16, 20, 0, 0, 0, time.UTC)
		},
	}
	input := CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
		},
		CanonicalRequest: []byte(`{"transactions":[{"index":0},{"index":1}]}`),
		IdempotencyKey:   "atomic-execution-test",
		IdempotencyTTL:   time.Minute,
	}

	return uc, input, transactionIDs, executionID
}

func atomicTransactionBatchProtectionStore(t *testing.T, uc *UseCase) *accountClosingMarkerStore {
	t.Helper()

	reader, ok := uc.TransactionReader.(*atomicTransactionBatchSettingsReader)
	require.True(t, ok)
	require.NotNil(t, reader.protectionStore)

	return reader.protectionStore
}

func atomicTransactionBatchExecutionReserver() *atomicTransactionBatchTracerFake {
	reservationIDs := atomicTransactionBatchExecutionReservationIDs()

	return &atomicTransactionBatchTracerFake{results: []*tracer.ReserveResult{
		{ReservationIDs: []uuid.UUID{reservationIDs[0]}},
		{ReservationIDs: []uuid.UUID{reservationIDs[1]}},
	}}
}

func atomicTransactionBatchExecutionReservationIDs() []uuid.UUID {
	return []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000eb"),
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000ec"),
	}
}

func atomicTransactionBatchEngineTransactionIDs(execution EngineExecution) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(execution.Execution.Transactions))
	for index := range execution.Execution.Transactions {
		transactionIDs[index] = execution.Execution.Transactions[index].ID
	}

	return transactionIDs
}

func atomicTransactionBatchGuardIDs(guards []ExecutionGuard) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(guards))
	for index := range guards {
		transactionIDs[index] = guards[index].TransactionID
	}

	return transactionIDs
}

func atomicTransactionBatchCompletionRecordIDs(records []CompletionPlanRecord) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(records))
	for index := range records {
		transactionIDs[index] = records[index].TransactionID
	}

	return transactionIDs
}

func TestAtomicBatchFenceRefusalCleansHandoffBeforeAccounting(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean abort", true: "protected abort"}[protected], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := NewMockTracerObligationStore(ctrl)
			repository := &atomicTransactionBatchClaimRepositoryFake{}
			if protected {
				repository.abortErr = txRedis.ErrAtomicTransactionBatchRefusalProtected
			}
			engine := &scriptedEngine{}
			now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
			uc := &UseCase{Engine: engine, AtomicTransactionBatchIdempotencyRepo: repository, ContextTracer: &ContextTracerCoordinator{recovery: &TracerRecoveryProcessor{store: store, now: func() time.Time { return now }, config: TracerRecoveryConfig{MaxBatch: 2, AttemptTimeout: time.Second}}}}
			run := atomicTransactionBatchTracerTestRun(2)
			run.executionID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
			run.idempotencyClaimed, run.idempotencyHandedOff = true, true
			keys := make([]tracerreservation.Key, len(run.items))
			for i := range run.items {
				keys[i] = tracerreservation.Key{TransactionID: run.items[i].transactionID}
				run.items[i].tracerReservation = reservationHandle{ContextAttempt: &ContextTracerAttempt{Key: keys[i], IntentAttempted: true, Frozen: true}}
			}
			store.EXPECT().BeginExecutions(gomock.Any(), keys, now).Return(constant.ErrReserveOperationConflict)
			if !protected {
				for _, key := range keys {
					store.EXPECT().SetOutcome(gomock.Any(), key, tracerreservation.Released, now).Return(nil)
				}
			}
			ctx, span, logger := anchorDeps()
			_, err := uc.executeAtomicTransactionBatch(ctx, span, logger, run, PreparedEngineExecution{})
			if protected {
				require.ErrorIs(t, err, repository.abortErr)
			} else {
				require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
			}
			require.Empty(t, engine.requests)
			require.Equal(t, 1, repository.aborts)
			require.Equal(t, protected, run.idempotencyHandedOff)
			require.Equal(t, protected, run.idempotencyClaimed)
		})
	}
}
