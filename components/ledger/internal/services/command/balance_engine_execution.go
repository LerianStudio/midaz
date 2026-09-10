// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// ErrInvalidBalanceEngineResult identifies a nil or malformed result returned
// as a successful accounting execution.
var ErrInvalidBalanceEngineResult = errors.New("invalid balance engine result")

// PreparedBalanceEngineExecution carries one validated execution and its
// canonical single-transaction completion plan.
type PreparedBalanceEngineExecution struct {
	Execution      EngineExecution
	CompletionPlan TransactionCompletionPlan
}

// BalanceEngineExecutionOutcome preserves the prepared input and any result
// returned by its single atomic execution.
type BalanceEngineExecutionOutcome struct {
	Prepared PreparedBalanceEngineExecution
	Result   *accounting.ExecutionResult
	Executed bool
}

// ExecutePreparedBalanceEngine executes a prepared request exactly once. Live
// balance concurrency is resolved inside the atomic engine implementation. It
// deliberately performs no retry: an error may follow an applied mutation, and
// only recovery of that recorded execution may continue automatically.
func ExecutePreparedBalanceEngine(
	ctx context.Context,
	executor BalanceEngine,
	prepared PreparedBalanceEngineExecution,
) (BalanceEngineExecutionOutcome, error) {
	outcome := BalanceEngineExecutionOutcome{Prepared: prepared}
	if executor == nil {
		return outcome, fmt.Errorf("balance engine execution requires an executor")
	}

	if err := ctx.Err(); err != nil {
		return outcome, err
	}

	if err := validatePreparedBalanceEngineExecution(prepared); err != nil {
		return outcome, err
	}

	outcome.Executed = true
	result, err := executor.Execute(ctx, prepared.Execution)
	outcome.Result = result
	if err != nil {
		if result != nil {
			return outcome, invalidBalanceEngineResult(fmt.Errorf("executor returned both a result and an error: %w", err))
		}

		return outcome, err
	}

	if result == nil {
		return outcome, invalidBalanceEngineResult(errors.New("executor returned a nil result"))
	}

	if _, validationErr := validateOperationMovementResult(prepared.CompletionPlan, *result); validationErr != nil {
		return outcome, invalidBalanceEngineResult(validationErr)
	}

	return outcome, nil
}

func validatePreparedBalanceEngineExecution(prepared PreparedBalanceEngineExecution) error {
	if err := ValidateTransactionCompletion(prepared.Execution); err != nil {
		return err
	}

	if len(prepared.Execution.Execution.Transactions) != 1 || len(prepared.Execution.CompletionPlans) != 1 {
		return invalidTransactionCompletionRecord("balance engine execution requires one transaction and completion plan")
	}

	transactionID := prepared.Execution.Execution.Transactions[0].ID
	if prepared.CompletionPlan.TransactionID != transactionID || prepared.Execution.CompletionPlans[0].TransactionID != transactionID {
		return invalidTransactionCompletionRecord("completion plan does not match its transaction")
	}

	canonicalPlan, err := EncodeTransactionCompletionPlan(prepared.CompletionPlan)
	if err != nil {
		return err
	}

	storedPlan, err := DecodeTransactionCompletionPlan(prepared.Execution.CompletionPlans[0].Payload)
	if err != nil {
		return err
	}

	canonicalStoredPlan, err := EncodeTransactionCompletionPlan(*storedPlan)
	if err != nil {
		return err
	}

	if !bytes.Equal(canonicalPlan, canonicalStoredPlan) {
		return invalidTransactionCompletionRecord("completion plan does not match canonical execution plan")
	}

	return nil
}

type invalidBalanceEngineResultError struct {
	err error
}

func (e *invalidBalanceEngineResultError) Error() string {
	return "balance engine returned an invalid result: " + e.err.Error()
}

func (e *invalidBalanceEngineResultError) Unwrap() error {
	return e.err
}

func (e *invalidBalanceEngineResultError) EngineFailureCode() string {
	return "invalid_result"
}

func (e *invalidBalanceEngineResultError) OutcomeIndeterminate() bool {
	return true
}

func invalidBalanceEngineResult(cause error) error {
	return &invalidBalanceEngineResultError{err: fmt.Errorf("%w: %w", ErrInvalidBalanceEngineResult, cause)}
}
