// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type balanceEngineFunc func(context.Context, EngineExecution) (*engine.Result, error)

func (execute balanceEngineFunc) Execute(ctx context.Context, input EngineExecution) (*engine.Result, error) {
	return execute(ctx, input)
}

func TestExecuteBalanceEngineWithRetryReloadsAfterStaleVersion(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	stale := &engine.Failure{
		Code:             engine.FailureStaleVersion,
		TransactionIndex: 0,
		PostingIndex:     0,
		BalanceRef:       "@source#default",
	}
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{
		{err: stale},
		{result: &validResult},
	}}
	builds := 0

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		attempt := retryContractAttempt(t, payload, validResult)
		attempt.Execution.Request.Balances[0].Version = int64(builds)

		return attempt, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, builds)
	require.Len(t, executor.requests, 2)
	assert.Equal(t, int64(1), executor.requests[0].Request.Balances[0].Version)
	assert.Equal(t, int64(2), executor.requests[1].Request.Balances[0].Version)
	assert.Equal(t, int64(2), got.Attempt.Execution.Request.Balances[0].Version)
	assert.Same(t, &validResult, got.Result)
}

func TestExecuteBalanceEngineWithRetryStopsAfterThreeStaleVersions(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	stale := retryContractStaleVersion()
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{
		{err: stale},
		{err: stale},
		{err: stale},
	}}
	builds := 0

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		attempt := retryContractAttempt(t, payload, validResult)
		attempt.Execution.Request.Balances[0].Version = int64(builds)

		return attempt, nil
	})

	assert.Same(t, stale, err)
	assert.ErrorIs(t, err, stale)
	assert.Equal(t, 3, builds)
	assert.Len(t, executor.requests, 3)
	assert.Equal(t, int64(3), got.Attempt.Execution.Request.Balances[0].Version)
	assert.Nil(t, got.Result)
}

func TestExecuteBalanceEngineWithRetryHonorsCancellation(t *testing.T) {
	t.Parallel()

	t.Run("after builder and before execute", func(t *testing.T) {
		payload, validResult := recoveryContractFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		executor := &scriptedBalanceEngine{}

		got, err := ExecuteBalanceEngineWithRetry(ctx, executor, func(context.Context) (BalanceEngineAttempt, error) {
			cancel()
			return retryContractAttempt(t, payload, validResult), nil
		})

		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, executor.requests)
		assert.Equal(t, payload.ExecutionID, got.Attempt.Execution.Request.ExecutionID)
	})

	t.Run("between stale attempts", func(t *testing.T) {
		payload, validResult := recoveryContractFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		executor := balanceEngineFunc(func(context.Context, EngineExecution) (*engine.Result, error) {
			calls++
			cancel()

			return nil, retryContractStaleVersion()
		})
		builds := 0

		_, err := ExecuteBalanceEngineWithRetry(ctx, executor, func(context.Context) (BalanceEngineAttempt, error) {
			builds++
			return retryContractAttempt(t, payload, validResult), nil
		})

		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, builds)
		assert.Equal(t, 1, calls)
	})
}

func TestExecuteBalanceEngineWithRetryAllowsBalanceDependentReloads(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	stale := retryContractStaleVersion()
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{
		{err: stale},
		{result: &validResult},
	}}
	builds := 0

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		current := cloneRetryContractPayload(t, payload)
		attempt := retryContractAttempt(t, current, validResult)
		if builds == 1 {
			return attempt, nil
		}

		limit := "200"
		current.OperationSpecs[0].Balance.Settings = &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		}
		companion := current.OperationSpecs[0]
		companion.Role = engine.RoleOverdraftCompanion
		companion.BalanceRef = "@source#overdraft"
		companion.Balance.ID = "99999999-9999-4999-8999-999999999999"
		companion.Balance.Key = "overdraft"
		companion.Balance.Direction = "debit"
		companion.Metadata = nil
		companion.ChartOfAccounts = ""
		current.OperationSpecs = append(current.OperationSpecs, companion)

		attempt = retryContractAttempt(t, current, validResult)
		attempt.Execution.Request.Balances[0].Version = 2
		attempt.Execution.Request.Balances[0].AllowOverdraft = true
		attempt.Execution.Request.Balances[0].OverdraftLimitEnabled = true
		attempt.Execution.Request.Balances = append(attempt.Execution.Request.Balances, engine.BalanceSnapshot{
			BalanceRef:  companion.BalanceRef,
			ID:          uuid.MustParse(companion.Balance.ID),
			AccountID:   uuid.MustParse(companion.Balance.AccountID),
			AccountType: companion.Balance.AccountType,
			AssetCode:   companion.Balance.AssetCode,
			Alias:       companion.Balance.Alias,
			Key:         companion.Balance.Key,
			Direction:   companion.Balance.Direction,
			Version:     7,
		})

		return attempt, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, builds)
	assert.Len(t, executor.requests, 2)
	assert.Len(t, got.Attempt.Payload.OperationSpecs, 2)
	assert.Len(t, got.Attempt.Execution.Request.Balances, 2)
	assert.True(t, got.Attempt.Execution.Request.Balances[0].AllowOverdraft)
}

func TestExecuteBalanceEngineWithRetryRejectsImmutableIdentityChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, *TransactionCompletionPlan, *BalanceEngineAttempt)
	}{
		{
			name: "execution id",
			mutate: func(t *testing.T, payload *TransactionCompletionPlan, attempt *BalanceEngineAttempt) {
				payload.ExecutionID = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
				refingerprintRetryContractPayload(t, payload)
				*attempt = retryContractAttempt(t, *payload, engine.Result{Movements: []engine.Movement{}, Final: []engine.BalanceSnapshot{}})
			},
		},
		{
			name: "fingerprint",
			mutate: func(t *testing.T, payload *TransactionCompletionPlan, attempt *BalanceEngineAttempt) {
				payload.TransactionDate = payload.TransactionDate.Add(time.Second)
				refingerprintRetryContractPayload(t, payload)
				*attempt = retryContractAttempt(t, *payload, engine.Result{Movements: []engine.Movement{}, Final: []engine.BalanceSnapshot{}})
			},
		},
		{
			name: "guard",
			mutate: func(_ *testing.T, _ *TransactionCompletionPlan, attempt *BalanceEngineAttempt) {
				attempt.Execution.Guards[0].NextToken = "committed"
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			payload, validResult := recoveryContractFixture(t)
			executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: retryContractStaleVersion()}}}
			builds := 0

			got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
				builds++
				current := cloneRetryContractPayload(t, payload)
				attempt := retryContractAttempt(t, current, validResult)
				if builds == 2 {
					test.mutate(t, &current, &attempt)
				}

				return attempt, nil
			})

			assert.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
			assert.Equal(t, 2, builds)
			assert.Len(t, executor.requests, 1)
			assert.Equal(t, got.Attempt.Execution.Request.ExecutionID, got.Attempt.Payload.ExecutionID)
		})
	}
}

func TestExecuteBalanceEngineWithRetryCopiesCapturedGuards(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: retryContractStaleVersion()}}}
	builds := 0
	var firstGuards []ExecutionGuard

	_, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		attempt := retryContractAttempt(t, payload, validResult)
		if builds == 1 {
			firstGuards = attempt.Execution.Guards
			return attempt, nil
		}

		firstGuards[0].NextToken = "committed"
		attempt.Execution.Guards = firstGuards

		return attempt, nil
	})

	assert.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	assert.Equal(t, 2, builds)
	assert.Len(t, executor.requests, 1)
}

func TestExecuteBalanceEngineWithRetryRequiresCanonicalAttemptPayload(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	executor := &scriptedBalanceEngine{}

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		attempt := retryContractAttempt(t, payload, validResult)
		attempt.Payload.HeaderID = "different-correlation-header"

		return attempt, nil
	})

	assert.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	assert.Empty(t, executor.requests)
	assert.Equal(t, "different-correlation-header", got.Attempt.Payload.HeaderID)
}

func TestExecuteBalanceEngineWithRetryDoesNotRetryTechnicalStaleWrapper(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	stale := retryContractStaleVersion()
	technical := testBalanceEngineTechnicalError{code: "execute_indeterminate", indeterminate: true, cause: stale}
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: technical}}}
	builds := 0

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		return retryContractAttempt(t, payload, validResult), nil
	})

	assert.Equal(t, technical, err)
	assert.ErrorIs(t, err, stale)
	assert.Equal(t, 1, builds)
	assert.Len(t, executor.requests, 1)
	assert.Equal(t, payload.ExecutionID, got.Attempt.Execution.Request.ExecutionID)
}

func TestExecuteBalanceEngineWithRetryDoesNotRetryStaleWithResult(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	stale := retryContractStaleVersion()
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{result: &validResult, err: stale}}}
	builds := 0

	got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		return retryContractAttempt(t, payload, validResult), nil
	})

	assert.ErrorIs(t, err, stale)
	assert.ErrorIs(t, err, ErrInvalidBalanceEngineResult)
	var technical balanceEngineTechnicalError
	require.ErrorAs(t, err, &technical)
	assert.Equal(t, "invalid_result", technical.EngineFailureCode())
	assert.True(t, technical.OutcomeIndeterminate())
	assert.Same(t, &validResult, got.Result)
	assert.Equal(t, 1, builds)
	assert.Len(t, executor.requests, 1)
}

func TestExecuteBalanceEngineWithRetryDoesNotRetryNilStaleFailure(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	var stale *engine.Failure
	var returned error = stale
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: returned}}}
	builds := 0

	_, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		builds++
		return retryContractAttempt(t, payload, validResult), nil
	})

	if err == nil {
		t.Fatal("typed-nil failure must remain an error")
	}
	assert.Equal(t, 1, builds)
	assert.Len(t, executor.requests, 1)
}

func TestExecuteBalanceEngineWithRetryRejectsPostingIntentChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reuseFirst bool
		mutate     func(*engine.Posting)
	}{
		{
			name:       "amount through aliased first-attempt slice",
			reuseFirst: true,
			mutate: func(posting *engine.Posting) {
				posting.Amount = posting.Amount.Add(posting.Amount)
			},
		},
		{
			name: "type",
			mutate: func(posting *engine.Posting) {
				posting.Type = engine.PostingCredit
			},
		},
		{
			name: "overdraft amount",
			mutate: func(posting *engine.Posting) {
				posting.OverdraftAmount = posting.Amount
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			payload, validResult := recoveryContractFixture(t)
			executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: retryContractStaleVersion()}}}
			builds := 0
			var firstPostings []engine.Posting

			_, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
				builds++
				attempt := retryContractAttempt(t, payload, validResult)
				if builds == 1 {
					firstPostings = attempt.Execution.Request.Transactions[0].Postings
					return attempt, nil
				}

				posting := &attempt.Execution.Request.Transactions[0].Postings[0]
				if test.reuseFirst {
					posting = &firstPostings[0]
					attempt.Execution.Request.Transactions[0].Postings = firstPostings
				}
				test.mutate(posting)

				return attempt, nil
			})

			assert.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
			assert.Equal(t, 2, builds)
			assert.Len(t, executor.requests, 1)
		})
	}
}

func TestExecuteBalanceEngineWithRetryTreatsInvalidSuccessfulResultsAsIndeterminate(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	tests := []struct {
		name   string
		result *engine.Result
	}{
		{name: "nil result"},
		{name: "malformed result", result: &engine.Result{}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{result: test.result}}}
			got, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
				return retryContractAttempt(t, payload, validResult), nil
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidBalanceEngineResult)
			var technical balanceEngineTechnicalError
			require.ErrorAs(t, err, &technical)
			assert.Equal(t, "invalid_result", technical.EngineFailureCode())
			assert.True(t, technical.OutcomeIndeterminate())
			assert.Same(t, test.result, got.Result)
			assert.Len(t, executor.requests, 1)
		})
	}
}

func TestExecuteBalanceEngineWithRetryRejectsMalformedStaleFailure(t *testing.T) {
	t.Parallel()

	payload, validResult := recoveryContractFixture(t)
	malformed := retryContractStaleVersion()
	malformed.BalanceRef = "@unrelated#default"
	executor := &scriptedBalanceEngine{responses: []balanceEngineResponse{{err: malformed}}}

	_, err := ExecuteBalanceEngineWithRetry(context.Background(), executor, func(context.Context) (BalanceEngineAttempt, error) {
		return retryContractAttempt(t, payload, validResult), nil
	})

	assert.Same(t, malformed, err)
	assert.Len(t, executor.requests, 1)
}

func retryContractAttempt(t *testing.T, payload TransactionCompletionPlan, result engine.Result) BalanceEngineAttempt {
	t.Helper()

	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)

	return BalanceEngineAttempt{
		Payload: payload,
		Execution: EngineExecution{
			Request: engine.Request{
				OrganizationID: payload.OrganizationID,
				LedgerID:       payload.LedgerID,
				ExecutionID:    payload.ExecutionID,
				Balances:       append([]engine.BalanceSnapshot(nil), result.Final...),
				Transactions: []engine.Transaction{{
					ID: payload.TransactionID,
					Postings: []engine.Posting{{
						Ref:        "source:0",
						BalanceRef: "@source#default",
						Type:       engine.PostingDebit,
						Amount:     payload.OperationSpecs[0].RequestedAmount,
					}},
				}},
			},
			IntentFingerprint: payload.IntentFingerprint,
			Guards: []ExecutionGuard{{
				TransactionID: payload.TransactionID,
				NextToken:     "approved",
			}},
			CompletionPlans: []CompletionPlanRecord{{
				TransactionID: payload.TransactionID,
				Payload:       raw,
			}},
		},
	}
}

func retryContractStaleVersion() *engine.Failure {
	return &engine.Failure{
		Code:             engine.FailureStaleVersion,
		TransactionIndex: 0,
		PostingIndex:     0,
		BalanceRef:       "@source#default",
	}
}

func cloneRetryContractPayload(t *testing.T, payload TransactionCompletionPlan) TransactionCompletionPlan {
	t.Helper()

	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	clone, err := DecodeTransactionCompletionPlan(raw)
	require.NoError(t, err)

	return *clone
}

func refingerprintRetryContractPayload(t *testing.T, payload *TransactionCompletionPlan) {
	t.Helper()

	fingerprint, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(*payload))
	require.NoError(t, err)
	payload.IntentFingerprint = fingerprint
}
