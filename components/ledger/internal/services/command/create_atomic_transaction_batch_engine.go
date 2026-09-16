// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

func buildAtomicTransactionBatchPreparedExecution(
	run *atomicTransactionBatchRun,
) (PreparedEngineExecution, error) {
	if run == nil || len(run.items) == 0 {
		return PreparedEngineExecution{}, errors.New("atomic transaction batch has no prepared items")
	}

	if run.executionID == uuid.Nil || run.engineIntentFingerprint == "" {
		return PreparedEngineExecution{}, errors.New("atomic transaction batch execution identity is incomplete")
	}

	balancePrefixes, err := atomicTransactionBatchBalancePrefixes(run)
	if err != nil {
		return PreparedEngineExecution{}, err
	}

	prepared := PreparedEngineExecution{
		Execution: EngineExecution{
			Execution: accounting.Execution{
				OrganizationID: run.organizationID,
				LedgerID:       run.ledgerID,
				ExecutionID:    run.executionID,
				Balances:       append([]accounting.BalanceSnapshot(nil), balancePrefixes[len(balancePrefixes)-1]...),
				Transactions:   make([]accounting.Transaction, 0, len(run.items)),
			},
			IntentFingerprint: run.engineIntentFingerprint,
			RetentionSeconds:  atomicTransactionBatchRetentionSeconds(run.idempotencyTTL),
			Guards:            make([]ExecutionGuard, 0, len(run.items)),
			CompletionPlans:   make([]CompletionPlanRecord, 0, len(run.items)),
		},
		CompletionPlans: make([]TransactionCompletionPlan, 0, len(run.items)),
	}
	for index := range run.items {
		item := &run.items[index]
		prepared.Execution.Execution.Transactions = append(
			prepared.Execution.Execution.Transactions,
			item.prepared.transaction,
		)
		prepared.Execution.Guards = append(prepared.Execution.Guards, item.guard)
		prepared.Execution.CompletionPlans = append(
			prepared.Execution.CompletionPlans,
			CompletionPlanRecord{
				TransactionID: item.transactionID,
				Payload:       append(json.RawMessage(nil), item.completionPlanPayload...),
			},
		)
		prepared.CompletionPlans = append(prepared.CompletionPlans, item.completionPlan)
	}

	if err := validatePreparedEngineExecution(prepared); err != nil {
		return PreparedEngineExecution{}, err
	}

	return prepared, nil
}

func (uc *UseCase) prepareAtomicTransactionBatchIdempotency(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	if uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return errors.New("atomic transaction batch idempotency repository is not configured")
	}

	next := atomicTransactionBatchIdempotencyRecord(run, txRedis.AtomicTransactionBatchStatePrepared)

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.TransitionAtomicTransactionBatch(
		ctx,
		run.organizationID,
		run.ledgerID,
		run.idempotencyEffectiveKey,
		run.idempotencyOwnerToken,
		txRedis.AtomicTransactionBatchStateClaimed,
		next,
		0,
	)
	if err != nil {
		return err
	}

	if result == nil ||
		(result.Outcome != txRedis.AtomicTransactionBatchTransitionUpdated &&
			result.Outcome != txRedis.AtomicTransactionBatchAlreadyTransitioned) ||
		result.Record.State != txRedis.AtomicTransactionBatchStatePrepared {
		return errors.New("atomic transaction batch prepared transition returned an invalid result")
	}

	return nil
}

func (uc *UseCase) handoffAtomicTransactionBatchExecution(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	next := atomicTransactionBatchIdempotencyRecord(run, txRedis.AtomicTransactionBatchStateApplied)
	executionID := run.executionID
	next.ExecutionID = &executionID

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.HandoffAtomicTransactionBatchExecution(
		ctx,
		run.organizationID,
		run.ledgerID,
		run.idempotencyEffectiveKey,
		run.idempotencyOwnerToken,
		next,
	)
	if err != nil {
		return err
	}

	if result == nil ||
		(result.Outcome != txRedis.AtomicTransactionBatchTransitionUpdated &&
			result.Outcome != txRedis.AtomicTransactionBatchAlreadyTransitioned) ||
		result.Record.State != txRedis.AtomicTransactionBatchStateApplied ||
		result.Record.ExecutionID == nil || *result.Record.ExecutionID != run.executionID {
		return errors.New("atomic transaction batch execution handoff returned an invalid result")
	}

	run.idempotencyHandedOff = true

	return nil
}

func (uc *UseCase) executeAtomicTransactionBatch(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
	prepared PreparedEngineExecution,
) (EngineExecutionOutcome, error) {
	outcome, executeErr := ExecutePreparedEngine(ctx, uc.Engine, prepared)
	if executeErr == nil {
		uc.settleAtomicTransactionBatchReservations(
			ctx,
			span,
			logger,
			run,
			atomicTransactionBatchReservationKnownSuccess,
		)

		return outcome, nil
	}

	var failure *accounting.Failure
	if !outcome.Executed ||
		!errors.As(executeErr, &failure) || failure == nil ||
		!confirmedPrecommitEngineFailure(prepared.Execution.Execution, executeErr) {
		return outcome, MapEngineError(prepared.Execution.Execution, executeErr)
	}

	mapped := MapEngineError(prepared.Execution.Execution, executeErr)

	if err := uc.abortAtomicTransactionBatchConfirmedRefusal(ctx, run); err != nil {
		return outcome, err
	}

	uc.settleAtomicTransactionBatchReservations(
		ctx,
		span,
		logger,
		run,
		atomicTransactionBatchReservationConfirmedAbort,
	)

	return outcome, withAtomicTransactionBatchItemError(
		mapped,
		failure.TransactionIndex,
		"accounting execution refused",
	)
}

func (uc *UseCase) abortAtomicTransactionBatchConfirmedRefusal(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	transactionIDs := atomicTransactionBatchTransactionIDs(run)

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.AbortAtomicTransactionBatchConfirmedRefusal(
		ctx,
		run.organizationID,
		run.ledgerID,
		run.idempotencyEffectiveKey,
		run.idempotencyOwnerToken,
		run.executionID,
		transactionIDs,
	)
	if err != nil {
		return fmt.Errorf("protect atomic transaction batch after confirmed refusal: %w", err)
	}

	if result == nil ||
		(result.Outcome != txRedis.AtomicTransactionBatchRefusalDeleted &&
			result.Outcome != txRedis.AtomicTransactionBatchRefusalAlreadyDeleted) {
		return errors.New("protect atomic transaction batch after confirmed refusal: invalid abort result")
	}

	run.idempotencyClaimed = false
	run.idempotencyHandedOff = false

	return nil
}

func atomicTransactionBatchIdempotencyRecord(
	run *atomicTransactionBatchRun,
	state txRedis.AtomicTransactionBatchIdempotencyState,
) txRedis.AtomicTransactionBatchIdempotencyRecord {
	return txRedis.AtomicTransactionBatchIdempotencyRecord{
		FormatVersion:      txRedis.AtomicTransactionBatchIdempotencyFormatVersion,
		State:              state,
		RequestFingerprint: run.idempotencyFingerprint,
		OwnerToken:         run.idempotencyOwnerToken,
		BatchID:            run.batchID,
		TransactionIDs:     atomicTransactionBatchTransactionIDs(run),
	}
}

func atomicTransactionBatchTransactionIDs(run *atomicTransactionBatchRun) []uuid.UUID {
	transactionIDs := make([]uuid.UUID, len(run.items))
	for index := range run.items {
		transactionIDs[index] = run.items[index].transactionID
	}

	return transactionIDs
}
