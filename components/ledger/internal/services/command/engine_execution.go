// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// ErrInvalidEngineResult identifies a nil or malformed result returned
// as a successful accounting execution.
var ErrInvalidEngineResult = errors.New("invalid engine result")

const maxPreparedEngineTransactions = 50

// PreparedEngineExecution carries one validated execution and its ordered,
// canonical completion plans. Every plan index matches the transaction, guard,
// and embedded recovery plan at the same index.
type PreparedEngineExecution struct {
	Execution       EngineExecution
	CompletionPlans []TransactionCompletionPlan
}

// EngineExecutionOutcome preserves the prepared input, global result and
// validated per-transaction partitions from its single atomic execution.
type EngineExecutionOutcome struct {
	Prepared   PreparedEngineExecution
	Result     *accounting.ExecutionResult
	Partitions []accounting.ExecutionResult
	Executed   bool
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

	partitions, validationErr := partitionValidatedEngineResult(prepared, *result)
	if validationErr != nil {
		return outcome, invalidEngineResult(validationErr)
	}

	outcome.Partitions = partitions

	return outcome, nil
}

func validatePreparedEngineExecution(prepared PreparedEngineExecution) error {
	transactions := prepared.Execution.Execution.Transactions

	transactionCount := len(transactions)
	if transactionCount < 1 || transactionCount > maxPreparedEngineTransactions {
		return invalidTransactionCompletionRecord("engine execution requires between 1 and 50 transactions")
	}

	if len(prepared.Execution.Guards) != transactionCount ||
		len(prepared.Execution.CompletionPlans) != transactionCount ||
		len(prepared.CompletionPlans) != transactionCount {
		return invalidTransactionCompletionRecord("transactions, guards, embedded plans and typed plans must correlate one to one")
	}

	for index, transaction := range transactions {
		transactionID := transaction.ID
		guard := prepared.Execution.Guards[index]
		embeddedPlan := prepared.Execution.CompletionPlans[index]

		typedPlan := prepared.CompletionPlans[index]
		if transactionID == uuid.Nil || guard.TransactionID != transactionID ||
			embeddedPlan.TransactionID != transactionID || typedPlan.TransactionID != transactionID {
			return invalidTransactionCompletionRecord("transaction, guard, embedded plan and typed plan order does not match")
		}

		canonicalPlan, err := EncodeTransactionCompletionPlan(typedPlan)
		if err != nil {
			return err
		}

		if !bytes.Equal(canonicalPlan, embeddedPlan.Payload) {
			return invalidTransactionCompletionRecord("typed completion plan does not match canonical embedded plan")
		}
	}

	if err := ValidateTransactionCompletion(prepared.Execution); err != nil {
		return err
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
