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

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestAtomicTransactionBatchRevert_IsPrivateToCrossLedgerGroups(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("01995162-6f96-7000-8000-000000000001")
	groupID := uuid.MustParse("01995162-6f96-7000-8000-000000000002")
	first := atomicTransactionBatchItemInput(
		organizationID,
		uuid.MustParse("01995162-6f96-7000-8000-000000000003"),
		"@source-0",
		"@destination-0",
	)
	second := atomicTransactionBatchItemInput(
		organizationID,
		uuid.MustParse("01995162-6f96-7000-8000-000000000004"),
		"@source-1",
		"@destination-1",
	)
	first.Action, first.Order, first.OriginalIndex = constant.ActionRevert, 1, 1
	second.Action, second.Order, second.OriginalIndex = constant.ActionRevert, 2, 0
	transactions := []CreateAtomicTransactionBatchV2ItemInput{first, second}

	uc := &UseCase{UUIDv7Generator: func() (uuid.UUID, error) { return uuid.New(), nil }}
	_, err := uc.initializeAtomicTransactionBatchIdentity(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions:     transactions,
		GroupID:          &groupID,
		CrossLedgerGroup: true,
	})
	require.NoError(t, err)

	_, err = uc.initializeAtomicTransactionBatchIdentity(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: transactions,
		GroupID:      &groupID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrTransactionScopeMismatch.Error())
	require.Error(t, validateAtomicTransactionBatchItemCorrelationForGroup(transactions, false))
}

func TestCreateAtomicTransactionBatchV2_RevertCarriesParentAndOriginEvidenceWithoutFees(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(t, repository, engine, reserver)

	groupID := uuid.MustParse("01995162-6f96-7000-8000-000000000010")
	parentIDs := []uuid.UUID{
		uuid.MustParse("01995162-6f96-7000-8000-000000000011"),
		uuid.MustParse("01995162-6f96-7000-8000-000000000012"),
	}
	originExecutionIDs := []uuid.UUID{
		uuid.MustParse("01995162-6f96-7000-8000-000000000013"),
		uuid.MustParse("01995162-6f96-7000-8000-000000000014"),
	}
	feeApplier := &fakeFeeApplier{}
	uc.FeeApplier = feeApplier
	uc.UUIDv7Generator = orderedAtomicTransactionBatchUUIDs(t, transactionIDs[0], transactionIDs[1], executionID)
	input.GroupID = &groupID
	input.CrossLedgerGroup = true

	for index := range input.Transactions {
		item := &input.Transactions[index]
		item.Action = constant.ActionRevert
		item.Order = index + 1
		item.OriginalIndex = len(input.Transactions) - 1 - index
		item.ParentTransactionID = &parentIDs[index]
		item.Dependencies = []TransactionEvidenceReference{{
			Kind:           TransactionDependencyOrigin,
			OrganizationID: item.OrganizationID,
			LedgerID:       item.LedgerID,
			TransactionID:  parentIDs[index],
			ExecutionID:    originExecutionIDs[index],
		}}
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, result.Transactions, 2)
	require.Len(t, engine.executions, 1)
	assert.Zero(t, feeApplier.calls, "revert reconstructs historical fee legs and must not calculate fees again")

	execution := engine.executions[0]
	for index := range execution.CompletionPlans {
		require.Equal(t, input.Transactions[index].Dependencies, execution.CompletionPlans[index].Dependencies)

		plan, err := DecodeTransactionCompletionPlan(execution.CompletionPlans[index].Payload)
		require.NoError(t, err)
		require.NotNil(t, plan.ParentTransactionID)
		assert.Equal(t, parentIDs[index], *plan.ParentTransactionID)
		assert.Equal(t, constant.ActionRevert, plan.Action)
		require.NotNil(t, plan.GroupID)
		assert.Equal(t, groupID, *plan.GroupID)

		require.NotNil(t, result.Transactions[index].ParentTransactionID)
		assert.Equal(t, parentIDs[index].String(), *result.Transactions[index].ParentTransactionID)
	}
}
