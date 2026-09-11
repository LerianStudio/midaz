// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

func TestExecutePreparedEngineExecutesExactlyOnce(t *testing.T) {
	t.Parallel()

	plan, result := recoveryContractFixture(t)
	prepared := preparedExecutionFixture(t, plan, result)
	executor := &scriptedEngine{responses: []engineResponse{{result: &result}}}

	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.NoError(t, err)
	assert.Equal(t, prepared, outcome.Prepared)
	assert.Same(t, &result, outcome.Result)
	assert.True(t, outcome.Executed)
	require.Len(t, executor.requests, 1)
}

func TestExecutePreparedEngineDoesNotRetryRefusals(t *testing.T) {
	t.Parallel()

	plan, result := recoveryContractFixture(t)
	prepared := preparedExecutionFixture(t, plan, result)
	refusal := &accounting.Failure{
		Code: accounting.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "@source#default",
	}
	executor := &scriptedEngine{responses: []engineResponse{{err: refusal}, {result: &result}}}

	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	assert.Same(t, refusal, err)
	assert.Nil(t, outcome.Result)
	assert.True(t, outcome.Executed)
	require.Len(t, executor.requests, 1)
	require.Len(t, executor.responses, 1, "the execution boundary must not consume a second outcome")
}

func TestExecutePreparedEngineTreatsInvalidSuccessAsIndeterminate(t *testing.T) {
	t.Parallel()

	plan, result := recoveryContractFixture(t)
	prepared := preparedExecutionFixture(t, plan, result)
	executor := &scriptedEngine{responses: []engineResponse{{}}}

	_, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.ErrorIs(t, err, ErrInvalidEngineResult)
	var technical engineTechnicalError
	require.ErrorAs(t, err, &technical)
	assert.True(t, technical.OutcomeIndeterminate())
	require.Len(t, executor.requests, 1)
}

func TestExecutePreparedEngineRejectsMismatchedPlanBeforeExecution(t *testing.T) {
	t.Parallel()

	plan, result := recoveryContractFixture(t)
	prepared := preparedExecutionFixture(t, plan, result)
	prepared.CompletionPlan.HeaderID = "different"
	executor := &scriptedEngine{}

	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	assert.False(t, outcome.Executed)
	assert.Empty(t, executor.requests)
}

func preparedExecutionFixture(t *testing.T, plan TransactionCompletionPlan, result accounting.ExecutionResult) PreparedEngineExecution {
	t.Helper()

	raw, err := EncodeTransactionCompletionPlan(plan)
	require.NoError(t, err)

	return PreparedEngineExecution{
		CompletionPlan: plan,
		Execution: EngineExecution{
			Execution: accounting.Execution{
				OrganizationID: plan.OrganizationID,
				LedgerID:       plan.LedgerID,
				ExecutionID:    plan.ExecutionID,
				Balances:       append([]accounting.BalanceSnapshot(nil), result.Final...),
				Transactions: []accounting.Transaction{{
					ID: plan.TransactionID,
					Postings: []accounting.Posting{{
						Ref: "source:0", BalanceRef: "@source#default", Type: accounting.PostingDebit,
						Amount: plan.OperationSpecs[0].RequestedAmount,
					}},
				}},
			},
			IntentFingerprint: plan.IntentFingerprint,
			Guards:            []ExecutionGuard{{TransactionID: plan.TransactionID, NextToken: "approved"}},
			CompletionPlans:   []CompletionPlanRecord{{TransactionID: plan.TransactionID, Payload: raw}},
		},
	}
}
