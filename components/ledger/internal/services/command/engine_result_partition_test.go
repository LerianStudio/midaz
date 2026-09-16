// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestPartitionEngineResultPreservesRepeatedBalanceBoundaries(t *testing.T) {
	t.Parallel()

	prepared, result := repeatedBalancePartitionFixture(t, 3)
	result.Final[0].Blocked = true
	result.Final[0].AllowSending = false

	partitions, err := PartitionEngineResult(prepared, result)
	require.NoError(t, err)
	require.Len(t, partitions, 3)

	for index, partition := range partitions {
		require.Equal(t, result.Movements[index:index+1], partition.Movements)
		require.Len(t, partition.Final, 1)
		assert.Equal(t, int64(index+1), partition.Final[0].Version)
		assert.True(t, partition.Final[0].Available.Equal(decimal.NewFromInt(int64(70-30*index))))
		assert.True(t, partition.Final[0].Blocked)
		assert.False(t, partition.Final[0].AllowSending)
	}

	executor := &scriptedEngine{responses: []engineResponse{{result: &result}}}
	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.NoError(t, err)
	assert.Equal(t, partitions, outcome.Partitions)
	require.Len(t, executor.requests, 1)
}

func TestPartitionEngineResultPreservesOverdraftCompanionsAndFirstTouchOrder(t *testing.T) {
	t.Parallel()

	prepared, result := overdraftPartitionFixture(t)

	partitions, err := PartitionEngineResult(prepared, result)
	require.NoError(t, err)
	require.Len(t, partitions, 2)

	for index, partition := range partitions {
		require.Len(t, partition.Movements, 2)
		assert.Equal(t, accounting.RolePrimary, partition.Movements[0].Role)
		assert.Equal(t, accounting.RoleOverdraftCompanion, partition.Movements[1].Role)
		require.Len(t, partition.Final, 2)
		assert.Equal(t, "@source#default", partition.Final[0].BalanceRef)
		assert.Equal(t, "@source#overdraft", partition.Final[1].BalanceRef)
		assert.Equal(t, int64(index+1), partition.Final[0].Version)
		assert.Equal(t, int64(index+1), partition.Final[1].Version)
	}

	assert.True(t, partitions[0].Final[0].OverdraftUsed.Equal(decimal.NewFromInt(20)))
	assert.True(t, partitions[0].Final[1].Available.Equal(decimal.NewFromInt(20)))
	assert.True(t, partitions[1].Final[0].OverdraftUsed.Equal(decimal.NewFromInt(50)))
	assert.True(t, partitions[1].Final[1].Available.Equal(decimal.NewFromInt(50)))
}

func TestPartitionEngineResultRejectsUnknownAndInterleavedMovements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*PreparedEngineExecution, *accounting.ExecutionResult)
	}{
		{
			name: "unknown transaction",
			mutate: func(_ *PreparedEngineExecution, result *accounting.ExecutionResult) {
				result.Movements[0].TransactionID = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
			},
		},
		{
			name: "unknown balance",
			mutate: func(_ *PreparedEngineExecution, result *accounting.ExecutionResult) {
				result.Movements[0].BalanceRef = "@unknown#default"
			},
		},
		{
			name: "interleaved transaction range",
			mutate: func(prepared *PreparedEngineExecution, result *accounting.ExecutionResult) {
				result.Movements[2].TransactionID = prepared.Execution.Execution.Transactions[0].ID
				result.Movements[2].Ref = "interleaved-movement"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prepared, result := repeatedBalancePartitionFixture(t, 3)
			tt.mutate(&prepared, &result)

			_, err := PartitionEngineResult(prepared, result)
			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
		})
	}
}

func TestPartitionEngineResultRejectsFinalOrderAndIdentityMismatch(t *testing.T) {
	t.Parallel()

	t.Run("first-touch order", func(t *testing.T) {
		t.Parallel()

		prepared, result := overdraftPartitionFixture(t)
		result.Final[0], result.Final[1] = result.Final[1], result.Final[0]

		_, err := PartitionEngineResult(prepared, result)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})

	t.Run("immutable identity", func(t *testing.T) {
		t.Parallel()

		prepared, result := repeatedBalancePartitionFixture(t, 2)
		result.Final[0].ID = uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")

		_, err := PartitionEngineResult(prepared, result)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})
}

func TestPartitionEngineResultOneItemIsEquivalent(t *testing.T) {
	t.Parallel()

	plan, result := recoveryContractFixture(t)
	prepared := preparedExecutionFixture(t, plan, result)

	partitions, err := PartitionEngineResult(prepared, result)
	require.NoError(t, err)
	require.Equal(t, []accounting.ExecutionResult{result}, partitions)
}

func TestExecutePreparedEngineRejectsInvalidBatchPartition(t *testing.T) {
	t.Parallel()

	prepared, result := repeatedBalancePartitionFixture(t, 2)
	result.Movements = result.Movements[:1]
	result.Final[0].Available = result.Movements[0].After.Available
	result.Final[0].Version = result.Movements[0].After.Version
	executor := &scriptedEngine{responses: []engineResponse{{result: &result}}}

	outcome, err := ExecutePreparedEngine(context.Background(), executor, prepared)
	require.ErrorIs(t, err, ErrInvalidEngineResult)
	assert.Nil(t, outcome.Partitions)
	assert.Same(t, &result, outcome.Result)
	assert.True(t, outcome.Executed)
	require.Len(t, executor.requests, 1)
}

func repeatedBalancePartitionFixture(t *testing.T, count int) (PreparedEngineExecution, accounting.ExecutionResult) {
	t.Helper()

	prepared := preparedExecutionFixtureCount(t, count)
	movements := make([]accounting.Movement, 0, count)
	before := accounting.BalanceState{Available: decimal.NewFromInt(100)}

	for index, transaction := range prepared.Execution.Execution.Transactions {
		after := accounting.BalanceState{
			Available: decimal.NewFromInt(int64(70 - 30*index)),
			Version:   int64(index + 1),
		}
		movements = append(movements, accounting.Movement{
			Ref:           transaction.ID.String() + ":source:0:primary:0",
			TransactionID: transaction.ID,
			PostingRef:    "source:0",
			Role:          accounting.RolePrimary,
			BalanceRef:    "@source#default",
			Type:          accounting.PostingDebit,
			Amount:        decimal.NewFromInt(30),
			Before:        before,
			After:         after,
		})
		before = after
	}

	final := prepared.Execution.Execution.Balances[0]
	final.Available = before.Available
	final.OnHold = before.OnHold
	final.OverdraftUsed = before.OverdraftUsed
	final.Version = before.Version

	return prepared, accounting.ExecutionResult{Movements: movements, Final: []accounting.BalanceSnapshot{final}}
}

func overdraftPartitionFixture(t *testing.T) (PreparedEngineExecution, accounting.ExecutionResult) {
	t.Helper()

	prepared := preparedExecutionFixtureCount(t, 2)
	companion := prepared.Execution.Execution.Balances[0]
	companion.BalanceRef = "@source#overdraft"
	companion.ID = uuid.MustParse("99999999-9999-4999-8999-999999999999")
	companion.Key = "overdraft"
	companion.Direction = constant.DirectionDebit
	companion.BalanceScope = "internal"
	companion.Available = decimal.Zero
	companion.OnHold = decimal.Zero
	companion.OverdraftUsed = decimal.Zero
	companion.OverdraftLimit = decimal.Zero
	companion.Version = 0
	companion.AllowOverdraft = false
	companion.OverdraftLimitEnabled = false
	prepared.Execution.Execution.Balances = append(prepared.Execution.Execution.Balances, companion)

	for index := range prepared.CompletionPlans {
		projection := prepared.CompletionPlans[index].OperationSpecs[0]
		projection.Role = accounting.RoleOverdraftCompanion
		projection.RowType = constant.OVERDRAFT
		projection.BalanceRef = companion.BalanceRef
		projection.Metadata = nil
		projection.ChartOfAccounts = ""
		projection.Balance.ID = companion.ID.String()
		projection.Balance.Key = companion.Key
		projection.Balance.Direction = companion.Direction
		projection.Balance.Available = companion.Available
		projection.Balance.OnHold = companion.OnHold
		projection.Balance.OverdraftUsed = companion.OverdraftUsed
		projection.Balance.Version = companion.Version
		prepared.CompletionPlans[index].OperationSpecs = append(prepared.CompletionPlans[index].OperationSpecs, projection)
	}
	refingerprintPreparedPartitionFixture(t, &prepared)

	primaryStates := [][2]accounting.BalanceState{
		{
			{Available: decimal.NewFromInt(10)},
			{Available: decimal.Zero, OverdraftUsed: decimal.NewFromInt(20), Version: 1},
		},
		{
			{Available: decimal.Zero, OverdraftUsed: decimal.NewFromInt(20), Version: 1},
			{Available: decimal.Zero, OverdraftUsed: decimal.NewFromInt(50), Version: 2},
		},
	}
	companionStates := [][2]accounting.BalanceState{
		{
			{Available: decimal.Zero},
			{Available: decimal.NewFromInt(20), Version: 1},
		},
		{
			{Available: decimal.NewFromInt(20), Version: 1},
			{Available: decimal.NewFromInt(50), Version: 2},
		},
	}
	deltas := []decimal.Decimal{decimal.NewFromInt(20), decimal.NewFromInt(30)}
	primaryAmounts := []decimal.Decimal{decimal.NewFromInt(10), decimal.Zero}
	movements := make([]accounting.Movement, 0, 4)

	for index, transaction := range prepared.Execution.Execution.Transactions {
		movements = append(
			movements,
			accounting.Movement{
				Ref: transaction.ID.String() + ":source:0:primary:0", TransactionID: transaction.ID,
				PostingRef: "source:0", Role: accounting.RolePrimary, BalanceRef: "@source#default",
				Type: accounting.PostingDebit, Amount: primaryAmounts[index], OverdraftDelta: deltas[index],
				Before: primaryStates[index][0], After: primaryStates[index][1],
			},
			accounting.Movement{
				Ref: transaction.ID.String() + ":source:0:overdraft_companion:0", TransactionID: transaction.ID,
				PostingRef: "source:0", Role: accounting.RoleOverdraftCompanion, BalanceRef: companion.BalanceRef,
				Type: accounting.PostingDebit, Amount: deltas[index],
				Before: companionStates[index][0], After: companionStates[index][1],
			},
		)
	}

	primaryFinal := prepared.Execution.Execution.Balances[0]
	primaryFinal.Available = primaryStates[1][1].Available
	primaryFinal.OnHold = primaryStates[1][1].OnHold
	primaryFinal.OverdraftUsed = primaryStates[1][1].OverdraftUsed
	primaryFinal.Version = primaryStates[1][1].Version
	companion.Available = companionStates[1][1].Available
	companion.OnHold = companionStates[1][1].OnHold
	companion.OverdraftUsed = companionStates[1][1].OverdraftUsed
	companion.Version = companionStates[1][1].Version

	return prepared, accounting.ExecutionResult{
		Movements: movements,
		Final:     []accounting.BalanceSnapshot{primaryFinal, companion},
	}
}

func refingerprintPreparedPartitionFixture(t *testing.T, prepared *PreparedEngineExecution) {
	t.Helper()

	intent := EngineIntent{
		TenantID:       prepared.CompletionPlans[0].TenantID,
		OrganizationID: prepared.Execution.Execution.OrganizationID,
		LedgerID:       prepared.Execution.Execution.LedgerID,
		ExecutionID:    prepared.Execution.Execution.ExecutionID,
		Transactions:   make([]EngineTransactionIntent, len(prepared.CompletionPlans)),
	}
	for index, plan := range prepared.CompletionPlans {
		intent.Transactions[index] = transactionCompletionIntent(prepared.Execution.Execution.Transactions[index], plan)
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	require.NoError(t, err)
	prepared.Execution.IntentFingerprint = fingerprint
	prepared.Execution.CompletionPlans = prepared.Execution.CompletionPlans[:0]
	for index := range prepared.CompletionPlans {
		prepared.CompletionPlans[index].IntentFingerprint = fingerprint
		raw, encodeErr := EncodeTransactionCompletionPlan(prepared.CompletionPlans[index])
		require.NoError(t, encodeErr)
		prepared.Execution.CompletionPlans = append(prepared.Execution.CompletionPlans, CompletionPlanRecord{
			TransactionID: prepared.CompletionPlans[index].TransactionID,
			Payload:       raw,
		})
	}
}
