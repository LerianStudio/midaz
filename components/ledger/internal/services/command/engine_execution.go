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

// ErrInvalidEngineResult identifies a nil or malformed result returned
// as a successful accounting execution.
var ErrInvalidEngineResult = errors.New("invalid engine result")

// PreparedEngineExecution carries one validated execution and its
// canonical single-transaction completion plan.
type PreparedEngineExecution struct {
	Execution      EngineExecution
	CompletionPlan TransactionCompletionPlan
}

// EngineExecutionOutcome preserves the prepared input and any result
// returned by its single atomic execution.
type EngineExecutionOutcome struct {
	Prepared PreparedEngineExecution
	Result   *accounting.ExecutionResult
	Executed bool
}

// ExecutePreparedEngine executes a prepared request exactly once. Live
// balance concurrency is resolved inside the atomic engine implementation. It
// deliberately performs no retry: an error may follow an applied mutation, and
// only recovery of that recorded execution may continue automatically.
func ExecutePreparedEngine(
	ctx context.Context,
	executor Engine,
	prepared PreparedEngineExecution,
) (EngineExecutionOutcome, error) {
	outcome := EngineExecutionOutcome{Prepared: prepared}
	if executor == nil {
		return outcome, fmt.Errorf("engine execution requires an executor")
	}

	if err := ctx.Err(); err != nil {
		return outcome, err
	}

	if err := validatePreparedEngineExecution(prepared); err != nil {
		return outcome, err
	}

	outcome.Executed = true
	result, err := executor.Execute(ctx, prepared.Execution)
	outcome.Result = result
	if err != nil {
		if result != nil {
			return outcome, invalidEngineResult(fmt.Errorf("executor returned both a result and an error: %w", err))
		}

		return outcome, err
	}

	if result == nil {
		return outcome, invalidEngineResult(errors.New("executor returned a nil result"))
	}

	if _, validationErr := validateOperationMovementResult(prepared.CompletionPlan, *result); validationErr != nil {
		return outcome, invalidEngineResult(validationErr)
	}

	return outcome, nil
}

func validatePreparedEngineExecution(prepared PreparedEngineExecution) error {
	if err := ValidateTransactionCompletion(prepared.Execution); err != nil {
		return err
	}

	if len(prepared.Execution.Execution.Transactions) != 1 || len(prepared.Execution.CompletionPlans) != 1 {
		return invalidTransactionCompletionRecord("engine execution requires one transaction and completion plan")
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

type invalidEngineResultError struct {
	err error
}

func (e *invalidEngineResultError) Error() string {
	return "engine returned an invalid result: " + e.err.Error()
}

func (e *invalidEngineResultError) Unwrap() error {
	return e.err
}

func (e *invalidEngineResultError) EngineFailureCode() string {
	return "invalid_result"
}

func (e *invalidEngineResultError) OutcomeIndeterminate() bool {
	return true
}

func invalidEngineResult(cause error) error {
	return &invalidEngineResultError{err: fmt.Errorf("%w: %w", ErrInvalidEngineResult, cause)}
}
