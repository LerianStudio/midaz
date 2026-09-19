// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

//nolint:gocyclo // immutable capture, replay finalization, dispatch, and fallback are one ordered durability boundary
func (uc *UseCase) completeAtomicTransactionBatch(
	ctx context.Context,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
	outcome EngineExecutionOutcome,
) ([]*transaction.Transaction, error) {
	envelopes, err := atomicTransactionBatchWriteBehindEnvelopes(outcome)
	if err != nil {
		return nil, err
	}

	if run == nil || len(run.items) != len(envelopes) {
		return nil, invalidTransactionCompletionRecord("atomic batch items do not match completion records")
	}

	if isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return nil, invalidTransactionCompletionRecord("atomic batch completer is not configured")
	}

	transactions := make([]*transaction.Transaction, len(envelopes))
	for index := range envelopes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		views, err := BuildTransactionEvidenceViews(envelopes[index].Record)
		if err != nil {
			return nil, fmt.Errorf("compose atomic transaction batch item %d: %w", index, err)
		}

		tran := views.InitialResponse
		if tran == nil || tran.ID != run.items[index].transactionID.String() {
			return nil, invalidTransactionCompletionRecord("atomic batch evidence returned a mismatched transaction")
		}

		transactions[index] = tran

		if err := uc.captureAtomicTransactionBatchInitialResponse(ctx, run, run.items[index].transactionID, tran); err != nil {
			return nil, err
		}
	}

	if err := uc.finalizeAtomicTransactionBatch(ctx, run, transactions); err != nil {
		return nil, err
	}

	dispatched := uc.TransactionWriteBehindAsync && uc.TransactionWriteBehindDispatcher != nil
	if dispatched {
		for index := range envelopes {
			if err := uc.TransactionWriteBehindDispatcher.DispatchTransactionWriteBehind(ctx, envelopes[index]); err != nil {
				dispatched = false

				uc.recordEngineWriteBehindProjection(ctx, "fallback", "failed")

				logger.Log(ctx, libLog.LevelWarn, "Atomic batch write-behind publish failed or was uncertain; using synchronous projection fallback",
					libLog.Int("item_index", index), libLog.Err(err))

				break
			}
		}
	}

	if dispatched {
		uc.recordEngineWriteBehindProjection(ctx, "bulk", "published")
		return transactions, nil
	}

	fallbackPath := "bulk"
	if uc.TransactionWriteBehindAsync {
		fallbackPath = "fallback"
	}

	completions, err := uc.completeAtomicTransactionBatchFallback(ctx, envelopes)
	if err != nil {
		uc.recordEngineWriteBehindProjection(ctx, fallbackPath, "deferred")
		logger.Log(ctx, libLog.LevelWarn, "Atomic batch projection deferred to recovery", libLog.Err(err))

		return transactions, nil
	}

	uc.recordEngineWriteBehindProjection(ctx, fallbackPath, "completed")

	for index, completion := range completions {
		expectedStatus := run.items[index].status
		if expectedStatus == constant.CREATED {
			expectedStatus = constant.APPROVED
		}

		if completion.Outcome.TransactionStatus != expectedStatus {
			return nil, fmt.Errorf("%w: atomic batch completer confirmed %q for item %d, expected %q",
				ErrTransactionCompletionConflict, completion.Outcome.TransactionStatus, index, expectedStatus)
		}

		uc.acknowledgeEngineRecovery(ctx, logger, &envelopes[index].Record, completion)
	}

	return transactions, nil
}

func (uc *UseCase) completeAtomicTransactionBatchFallback(
	ctx context.Context,
	envelopes []*TransactionWriteBehindEnvelope,
) ([]TransactionCompletionResult, error) {
	if bulk, ok := uc.AppliedTransactionCompleter.(AppliedTransactionBulkCompleter); ok {
		return CompleteTransactionWriteBehindBulk(ctx, envelopes, uc.TransactionEvidenceResolver, bulk)
	}

	completions := make([]TransactionCompletionResult, len(envelopes))
	for index, envelope := range envelopes {
		completion, err := completeTransactionWriteBehindFallback(ctx, envelope, uc.TransactionEvidenceResolver, uc.AppliedTransactionCompleter)
		if err != nil {
			return nil, fmt.Errorf("complete atomic transaction batch item %d: %w", index, err)
		}

		completions[index] = completion
	}

	return completions, nil
}

func atomicTransactionBatchWriteBehindEnvelopes(outcome EngineExecutionOutcome) ([]*TransactionWriteBehindEnvelope, error) {
	records, err := atomicTransactionBatchCompletionRecords(outcome)
	if err != nil {
		return nil, err
	}

	envelopes := make([]*TransactionWriteBehindEnvelope, len(records))
	for index, record := range records {
		envelopes[index] = &TransactionWriteBehindEnvelope{
			FormatVersion: TransactionWriteBehindFormatVersion, ApplicationState: TransactionApplicationConfirmed,
			ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
			Record: *record,
			Dependencies: append([]TransactionEvidenceReference{},
				outcome.Prepared.Execution.CompletionPlans[index].Dependencies...),
		}
	}

	return envelopes, nil
}

func atomicTransactionBatchCompletionRecords(
	outcome EngineExecutionOutcome,
) ([]*TransactionCompletionRecord, error) {
	if !outcome.Executed || outcome.Result == nil {
		return nil, invalidEngineResult(errors.New("successful atomic batch has no engine result"))
	}

	prepared := outcome.Prepared
	if err := validatePreparedEngineExecution(prepared); err != nil {
		return nil, err
	}

	if len(outcome.Partitions) != len(prepared.CompletionPlans) {
		return nil, invalidEngineResult(errors.New("atomic batch result partitions do not match completion plans"))
	}

	records := make([]*TransactionCompletionRecord, len(prepared.CompletionPlans))
	for index := range prepared.CompletionPlans {
		plan := prepared.CompletionPlans[index]

		embedded := prepared.Execution.CompletionPlans[index]
		if embedded.TransactionID != plan.TransactionID {
			return nil, invalidEngineResult(errors.New("atomic batch completion order is inconsistent"))
		}

		records[index] = &TransactionCompletionRecord{
			FormatVersion:     TransactionCompletionFormatVersion,
			TenantID:          plan.TenantID,
			OrganizationID:    plan.OrganizationID,
			LedgerID:          plan.LedgerID,
			ExecutionID:       plan.ExecutionID,
			IntentFingerprint: plan.IntentFingerprint,
			TransactionID:     plan.TransactionID,
			Payload:           string(embedded.Payload),
			Result:            outcome.Partitions[index],
		}
	}

	return records, nil
}
