// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
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
	prepared.CompletionPlans[0].HeaderID = "different"
	executor := &scriptedEngine{}

	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	assert.False(t, outcome.Executed)
	assert.Empty(t, executor.requests)
}

func TestValidatePreparedEngineExecutionSupportsOrderedOneToFifty(t *testing.T) {
	t.Parallel()

	for _, count := range []int{1, 2, maxPreparedEngineTransactions} {
		prepared := preparedExecutionFixtureCount(t, count)
		require.NoError(t, validatePreparedEngineExecution(prepared), "count %d", count)
	}

	require.ErrorIs(t, validatePreparedEngineExecution(PreparedEngineExecution{}), ErrInvalidTransactionCompletionRecord)
	require.ErrorIs(
		t,
		validatePreparedEngineExecution(preparedExecutionFixtureCount(t, maxPreparedEngineTransactions+1)),
		ErrInvalidTransactionCompletionRecord,
	)
}

func TestValidatePreparedEngineExecutionRejectsMissingExtraAndReorderedCorrelations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*PreparedEngineExecution)
	}{
		{name: "missing transaction", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Execution.Transactions = p.Execution.Execution.Transactions[:1]
		}},
		{name: "missing guard", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Guards = p.Execution.Guards[:1]
		}},
		{name: "missing embedded plan", mutate: func(p *PreparedEngineExecution) {
			p.Execution.CompletionPlans = p.Execution.CompletionPlans[:1]
		}},
		{name: "missing typed plan", mutate: func(p *PreparedEngineExecution) {
			p.CompletionPlans = p.CompletionPlans[:1]
		}},
		{name: "extra transaction", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Execution.Transactions = append(p.Execution.Execution.Transactions, p.Execution.Execution.Transactions[0])
		}},
		{name: "extra guard", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Guards = append(p.Execution.Guards, p.Execution.Guards[0])
		}},
		{name: "extra embedded plan", mutate: func(p *PreparedEngineExecution) {
			p.Execution.CompletionPlans = append(p.Execution.CompletionPlans, p.Execution.CompletionPlans[0])
		}},
		{name: "extra typed plan", mutate: func(p *PreparedEngineExecution) {
			p.CompletionPlans = append(p.CompletionPlans, p.CompletionPlans[0])
		}},
		{name: "reordered transactions", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Execution.Transactions[0], p.Execution.Execution.Transactions[1] =
				p.Execution.Execution.Transactions[1], p.Execution.Execution.Transactions[0]
		}},
		{name: "reordered guards", mutate: func(p *PreparedEngineExecution) {
			p.Execution.Guards[0], p.Execution.Guards[1] = p.Execution.Guards[1], p.Execution.Guards[0]
		}},
		{name: "reordered embedded plans", mutate: func(p *PreparedEngineExecution) {
			p.Execution.CompletionPlans[0], p.Execution.CompletionPlans[1] =
				p.Execution.CompletionPlans[1], p.Execution.CompletionPlans[0]
		}},
		{name: "reordered typed plans", mutate: func(p *PreparedEngineExecution) {
			p.CompletionPlans[0], p.CompletionPlans[1] = p.CompletionPlans[1], p.CompletionPlans[0]
		}},
		{name: "noncanonical embedded plan", mutate: func(p *PreparedEngineExecution) {
			p.Execution.CompletionPlans[0].Payload = append([]byte(" \n"), p.Execution.CompletionPlans[0].Payload...)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prepared := preparedExecutionFixtureCount(t, 2)
			tt.mutate(&prepared)
			require.ErrorIs(t, validatePreparedEngineExecution(prepared), ErrInvalidTransactionCompletionRecord)
		})
	}
}

func preparedExecutionFixture(t *testing.T, plan TransactionCompletionPlan, result accounting.ExecutionResult) PreparedEngineExecution {
	t.Helper()

	raw, err := EncodeTransactionCompletionPlan(plan)
	require.NoError(t, err)

	return PreparedEngineExecution{
		CompletionPlans: []TransactionCompletionPlan{plan},
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

func preparedExecutionFixtureCount(t *testing.T, count int) PreparedEngineExecution {
	t.Helper()

	basePlan, result := recoveryContractFixture(t)
	prepared := PreparedEngineExecution{
		Execution: EngineExecution{
			Execution: accounting.Execution{
				OrganizationID: basePlan.OrganizationID,
				LedgerID:       basePlan.LedgerID,
				ExecutionID:    basePlan.ExecutionID,
				Balances:       append([]accounting.BalanceSnapshot(nil), result.Final...),
			},
		},
		CompletionPlans: make([]TransactionCompletionPlan, 0, count),
	}
	intent := EngineIntent{
		TenantID:       basePlan.TenantID,
		OrganizationID: basePlan.OrganizationID,
		LedgerID:       basePlan.LedgerID,
		ExecutionID:    basePlan.ExecutionID,
		Transactions:   make([]EngineTransactionIntent, 0, count),
	}

	for index := range count {
		transactionID := uuid.NewSHA1(basePlan.TransactionID, []byte{byte(index), byte(index >> 8)})
		plan := basePlan
		plan.TransactionID = transactionID
		plan.OperationSpecs = append([]OperationRecordSpec(nil), basePlan.OperationSpecs...)
		for specIndex := range plan.OperationSpecs {
			plan.OperationSpecs[specIndex].TransactionID = transactionID
		}

		transaction := accounting.Transaction{
			ID: transactionID,
			Postings: []accounting.Posting{{
				Ref:        "source:0",
				BalanceRef: "@source#default",
				Type:       accounting.PostingDebit,
				Amount:     plan.OperationSpecs[0].RequestedAmount,
			}},
		}
		prepared.Execution.Execution.Transactions = append(prepared.Execution.Execution.Transactions, transaction)
		prepared.Execution.Guards = append(prepared.Execution.Guards, ExecutionGuard{
			TransactionID: transactionID,
			NextToken:     "approved-" + transactionID.String(),
		})
		prepared.CompletionPlans = append(prepared.CompletionPlans, plan)
		intent.Transactions = append(intent.Transactions, transactionCompletionIntent(transaction, plan))
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	require.NoError(t, err)
	prepared.Execution.IntentFingerprint = fingerprint
	for index := range prepared.CompletionPlans {
		prepared.CompletionPlans[index].IntentFingerprint = fingerprint
		raw, err := EncodeTransactionCompletionPlan(prepared.CompletionPlans[index])
		require.NoError(t, err)
		prepared.Execution.CompletionPlans = append(
			prepared.Execution.CompletionPlans,
			CompletionPlanRecord{TransactionID: prepared.CompletionPlans[index].TransactionID, Payload: raw},
		)
	}

	return prepared
}
