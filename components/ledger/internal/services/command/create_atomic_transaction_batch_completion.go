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

func (uc *UseCase) completeAtomicTransactionBatch(
	ctx context.Context,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
	outcome EngineExecutionOutcome,
) ([]*transaction.Transaction, error) {
	records, err := atomicTransactionBatchCompletionRecords(outcome)
	if err != nil {
		return nil, err
	}
	if run == nil || len(run.items) != len(records) {
		return nil, invalidTransactionCompletionRecord("atomic batch items do not match completion records")
	}
	if isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return nil, invalidTransactionCompletionRecord("atomic batch completer is not configured")
	}

	transactions := make([]*transaction.Transaction, len(records))
	completions := make([]TransactionCompletionResult, len(records))
	for index := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		completion, err := uc.AppliedTransactionCompleter.Complete(ctx, records[index])
		if err != nil {
			return nil, fmt.Errorf("complete atomic transaction batch item %d: %w", index, err)
		}

		expectedStatus := run.items[index].status
		if expectedStatus == constant.CREATED {
			expectedStatus = constant.APPROVED
		}
		if completion.Outcome.TransactionStatus != expectedStatus {
			return nil, fmt.Errorf(
				"%w: atomic batch completer confirmed %q for item %d, expected %q",
				ErrTransactionCompletionConflict,
				completion.Outcome.TransactionStatus,
				index,
				expectedStatus,
			)
		}

		tran := completion.Record.Transaction
		if tran == nil || tran.ID != run.items[index].transactionID.String() {
			return nil, invalidTransactionCompletionRecord("atomic batch completer returned a mismatched transaction")
		}

		if run.items[index].status == constant.CREATED {
			created := constant.CREATED
			tran.Status = transaction.Status{Code: created, Description: &created}
		}
		transactions[index] = tran
		completions[index] = completion

		if index < len(records)-1 {
			uc.acknowledgeEngineRecovery(ctx, logger, records[index], completion)
		}
	}

	if err := uc.finalizeAtomicTransactionBatch(ctx, run, transactions); err != nil {
		return nil, err
	}
	last := len(records) - 1
	uc.acknowledgeEngineRecovery(ctx, logger, records[last], completions[last])

	return transactions, nil
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
