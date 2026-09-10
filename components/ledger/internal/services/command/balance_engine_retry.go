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
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
)

const balanceEngineMaximumAttempts = 3

// ErrInvalidBalanceEngineResult identifies a nil or malformed result returned
// as a successful accounting execution.
var ErrInvalidBalanceEngineResult = errors.New("invalid balance engine result")

// BalanceEngineAttempt carries one freshly prepared execution and its canonical
// single-transaction completion plan.
type BalanceEngineAttempt struct {
	Execution EngineExecution
	Payload   TransactionCompletionPlan
}

// BalanceEngineRetryResult preserves the latest prepared attempt and any result
// returned by its execution, including conservative post-execution failures.
type BalanceEngineRetryResult struct {
	Attempt BalanceEngineAttempt
	Result  *engine.Result
}

// BalanceEngineAttemptBuilder reloads and prepares balance-dependent state for
// one accounting execution attempt.
type BalanceEngineAttemptBuilder func(context.Context) (BalanceEngineAttempt, error)

// ExecuteBalanceEngineWithRetry retries only valid stale-version refusals. It
// never retries technical failures or a nil/malformed successful result.
func ExecuteBalanceEngineWithRetry(
	ctx context.Context,
	executor BalanceEngine,
	build BalanceEngineAttemptBuilder,
) (BalanceEngineRetryResult, error) {
	var (
		latest   BalanceEngineRetryResult
		identity *balanceEngineRetryIdentity
	)

	if executor == nil || build == nil {
		return latest, fmt.Errorf("balance engine retry requires an executor and attempt builder")
	}

	for attemptIndex := 0; attemptIndex < balanceEngineMaximumAttempts; attemptIndex++ {
		if err := ctx.Err(); err != nil {
			return latest, err
		}

		attempt, err := build(ctx)
		latest.Attempt = attempt

		latest.Result = nil
		if err != nil {
			return latest, err
		}

		if err := validateBalanceEngineAttempt(attempt); err != nil {
			return latest, err
		}

		currentIdentity := newBalanceEngineRetryIdentity(attempt.Execution)
		if identity == nil {
			identity = &currentIdentity
		} else if !identity.matches(currentIdentity) {
			return latest, invalidTransactionCompletionRecord("balance engine retry identity changed")
		}

		if err := ctx.Err(); err != nil {
			return latest, err
		}

		result, err := executor.Execute(ctx, attempt.Execution)

		latest.Result = result
		if err == nil {
			if result == nil {
				return latest, invalidBalanceEngineResult(errors.New("executor returned a nil result"))
			}

			if _, validationErr := validateOperationMovementResult(attempt.Payload, *result); validationErr != nil {
				return latest, invalidBalanceEngineResult(validationErr)
			}

			return latest, nil
		}

		if result != nil {
			return latest, invalidBalanceEngineResult(fmt.Errorf("executor returned both a result and an error: %w", err))
		}

		if !retryableBalanceEngineStaleVersion(attempt.Execution.Request, err) || attemptIndex == balanceEngineMaximumAttempts-1 {
			return latest, err
		}
	}

	return latest, fmt.Errorf("balance engine retry exhausted without an outcome")
}

func validateBalanceEngineAttempt(attempt BalanceEngineAttempt) error {
	if err := ValidateTransactionCompletion(attempt.Execution); err != nil {
		return err
	}

	if len(attempt.Execution.Request.Transactions) != 1 || len(attempt.Execution.CompletionPlans) != 1 {
		return invalidTransactionCompletionRecord("balance engine retry requires one transaction and completion plan")
	}

	transactionID := attempt.Execution.Request.Transactions[0].ID
	if attempt.Payload.TransactionID != transactionID || attempt.Execution.CompletionPlans[0].TransactionID != transactionID {
		return invalidTransactionCompletionRecord("retry payload does not match its transaction")
	}

	canonicalPayload, err := EncodeTransactionCompletionPlan(attempt.Payload)
	if err != nil {
		return err
	}

	completionPlan, err := DecodeTransactionCompletionPlan(attempt.Execution.CompletionPlans[0].Payload)
	if err != nil {
		return err
	}

	canonicalCompletionPlan, err := EncodeTransactionCompletionPlan(*completionPlan)
	if err != nil {
		return err
	}

	if !bytes.Equal(canonicalPayload, canonicalCompletionPlan) {
		return invalidTransactionCompletionRecord("retry payload does not match canonical completion plan")
	}

	return nil
}

func retryableBalanceEngineStaleVersion(request engine.Request, err error) bool {
	var technicalErr balanceEngineTechnicalError
	if errors.As(err, &technicalErr) {
		return false
	}

	var failure *engine.Failure
	if !errors.As(err, &failure) || failure == nil || failure.Code != engine.FailureStaleVersion {
		return false
	}

	_, valid := engineFailurePosting(request, failure)

	return valid
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

type balanceEngineRetryIdentity struct {
	organizationID    uuid.UUID
	ledgerID          uuid.UUID
	executionID       uuid.UUID
	intentFingerprint string
	transactionIDs    []uuid.UUID
	postings          [][]balanceEngineRetryPostingIdentity
	guards            []ExecutionGuard
}

type balanceEngineRetryPostingIdentity struct {
	ref             string
	balanceRef      string
	postingType     engine.PostingType
	amount          decimal.Decimal
	overdraftAmount decimal.Decimal
}

func newBalanceEngineRetryIdentity(execution EngineExecution) balanceEngineRetryIdentity {
	transactionIDs := make([]uuid.UUID, len(execution.Request.Transactions))

	postings := make([][]balanceEngineRetryPostingIdentity, len(execution.Request.Transactions))
	for index, transaction := range execution.Request.Transactions {
		transactionIDs[index] = transaction.ID

		postings[index] = make([]balanceEngineRetryPostingIdentity, len(transaction.Postings))
		for postingIndex, posting := range transaction.Postings {
			postings[index][postingIndex] = balanceEngineRetryPostingIdentity{
				ref:             posting.Ref,
				balanceRef:      posting.BalanceRef,
				postingType:     posting.Type,
				amount:          posting.Amount,
				overdraftAmount: posting.OverdraftAmount,
			}
		}
	}

	return balanceEngineRetryIdentity{
		organizationID:    execution.Request.OrganizationID,
		ledgerID:          execution.Request.LedgerID,
		executionID:       execution.Request.ExecutionID,
		intentFingerprint: execution.IntentFingerprint,
		transactionIDs:    transactionIDs,
		postings:          postings,
		guards:            append([]ExecutionGuard(nil), execution.Guards...),
	}
}

func (identity balanceEngineRetryIdentity) matches(other balanceEngineRetryIdentity) bool {
	if identity.organizationID != other.organizationID ||
		identity.ledgerID != other.ledgerID ||
		identity.executionID != other.executionID ||
		identity.intentFingerprint != other.intentFingerprint ||
		len(identity.transactionIDs) != len(other.transactionIDs) ||
		len(identity.postings) != len(other.postings) ||
		len(identity.guards) != len(other.guards) {
		return false
	}

	for index := range identity.transactionIDs {
		if identity.transactionIDs[index] != other.transactionIDs[index] {
			return false
		}

		if len(identity.postings[index]) != len(other.postings[index]) {
			return false
		}

		for postingIndex := range identity.postings[index] {
			if !identity.postings[index][postingIndex].matches(other.postings[index][postingIndex]) {
				return false
			}
		}
	}

	for index := range identity.guards {
		if identity.guards[index] != other.guards[index] {
			return false
		}
	}

	return true
}

func (posting balanceEngineRetryPostingIdentity) matches(other balanceEngineRetryPostingIdentity) bool {
	return posting.ref == other.ref &&
		posting.balanceRef == other.balanceRef &&
		posting.postingType == other.postingType &&
		posting.amount.Equal(other.amount) &&
		posting.overdraftAmount.Equal(other.overdraftAmount)
}
