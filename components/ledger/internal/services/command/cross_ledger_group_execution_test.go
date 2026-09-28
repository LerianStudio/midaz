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
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestBuildCrossLedgerGroupExecution_MergesTransitionAndCreateUnderOneIdentity(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	uc, input, _, _ := atomicTransactionBatchExecutionFixture(t, repository, engine, atomicTransactionBatchExecutionReserver())

	run, err := uc.initializeAtomicTransactionBatchIdentity(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, uc.initializeAtomicTransactionBatchItemsAndSettings(context.Background(), input, run))
	require.NoError(t, uc.prepareAtomicTransactionBatchItems(context.Background(), nil, nil, run))
	createPrepared, err := buildAtomicTransactionBatchPreparedExecution(run)
	require.NoError(t, err)

	originItem := run.items[0]
	persisted := atomicTransactionBatchFoundationResult(&originItem)
	persisted.Status = transaction.Status{Code: constant.PENDING}
	originPrepared, err := buildPendingEngineExecution(
		persisted,
		originItem.input,
		originItem.validate,
		originItem.prepared,
		pendingEngineStableContext{
			executionID:        uuid.New(),
			organizationID:     originItem.organizationID,
			ledgerID:           originItem.ledgerID,
			tenantID:           "",
			headerID:           "header-a",
			enqueuedAt:         originItem.operationUpdatedAt,
			actionDate:         originItem.transactionDate,
			transactionUpdated: originItem.transactionUpdatedAt,
			operationUpdated:   originItem.operationUpdatedAt,
			guard: ExecutionGuard{
				TransactionID: originItem.transactionID,
				ExpectedToken: constant.PENDING,
				NextToken:     constant.APPROVED,
			},
		},
		constant.ActionCommit,
		nil,
	)
	require.NoError(t, err)

	destinationPrepared := preparedEngineExecutionItem(createPrepared, 1)
	groupID := uuid.MustParse("0199a400-0000-7000-8000-000000000001")
	executionID := uuid.MustParse("0199a400-0000-7000-8000-000000000002")

	prepared, err := buildCrossLedgerGroupExecution(
		run.organizationID,
		run.ledgerID,
		groupID,
		executionID,
		[]PreparedEngineExecution{originPrepared, destinationPrepared},
	)
	require.NoError(t, err)
	require.Len(t, prepared.Execution.Execution.Transactions, 2)
	assert.Equal(t, executionID, prepared.Execution.Execution.ExecutionID)
	assert.Equal(t, constant.PENDING, prepared.Execution.Guards[0].ExpectedToken)
	assert.Equal(t, constant.APPROVED, prepared.Execution.Guards[0].NextToken)
	assert.Empty(t, prepared.Execution.Guards[1].ExpectedToken)
	assert.Equal(t, originItem.transactionID, prepared.Execution.Execution.Transactions[0].ID)
	assert.Equal(t, run.items[1].transactionID, prepared.Execution.Execution.Transactions[1].ID)
	assert.NotEmpty(t, prepared.Execution.IntentFingerprint)

	for index := range prepared.CompletionPlans {
		assert.Equal(t, executionID, prepared.CompletionPlans[index].ExecutionID)
		require.NotNil(t, prepared.CompletionPlans[index].GroupID)
		assert.Equal(t, groupID, *prepared.CompletionPlans[index].GroupID)
		assert.Equal(t, prepared.Execution.IntentFingerprint, prepared.CompletionPlans[index].IntentFingerprint)
	}

	seen := make(map[string]struct{})
	for _, snapshot := range prepared.Execution.Execution.Balances {
		key := completionScopedBalanceRef(snapshot.OrganizationID, snapshot.LedgerID, snapshot.BalanceRef)
		_, duplicate := seen[key]
		assert.False(t, duplicate, "the merged execution must not repeat a scoped balance")
		seen[key] = struct{}{}
	}
}

func preparedEngineExecutionItem(prepared PreparedEngineExecution, index int) PreparedEngineExecution {
	execution := prepared.Execution.Execution
	execution.Transactions = execution.Transactions[index : index+1]

	return PreparedEngineExecution{
		Execution: EngineExecution{
			Execution: execution,
			Guards: []ExecutionGuard{
				prepared.Execution.Guards[index],
			},
			CompletionPlans: []CompletionPlanRecord{
				prepared.Execution.CompletionPlans[index],
			},
		},
		CompletionPlans: []TransactionCompletionPlan{prepared.CompletionPlans[index]},
	}
}

func TestBuildCrossLedgerGroupExecution_StampsTheMembersOfThatExecution(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "commit lists the origin and the destination", status: constant.APPROVED},
		{name: "cancel lists only the origin", status: constant.CANCELED},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, repo, engine, target, in, group := newCrossLedgerLifecycleFixture(t, test.status)
			repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)
			repo.EXPECT().UpdateStatus(gomock.Any(), group.ID, constant.PENDING, test.status).Return(true, nil)

			_, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, test.status)
			require.NoError(t, err)
			require.Len(t, engine.executions, 1)

			records := engine.executions[0].CompletionPlans
			plans := make([]*TransactionCompletionPlan, 0, len(records))
			want := make([]TransactionCompletionMember, 0, len(records))
			for _, record := range records {
				plan, err := DecodeTransactionCompletionPlan(record.Payload)
				require.NoError(t, err)
				plans = append(plans, plan)
				want = append(want, TransactionCompletionMember{
					TransactionID: plan.TransactionID, OrganizationID: plan.OrganizationID, LedgerID: plan.LedgerID,
				})
			}

			if test.status == constant.APPROVED {
				require.Len(t, want, 2)
				assert.NotEqual(t, want[0].LedgerID, want[1].LedgerID, "the destination lives in the other ledger")
			} else {
				require.Len(t, want, 1)
			}
			assert.Equal(t, uuid.MustParse(target.ID), want[0].TransactionID)
			assert.Equal(t, uuid.MustParse(target.LedgerID), want[0].LedgerID)

			for index, plan := range plans {
				assert.Equal(t, want, plan.ExecutionMembers, "plan %d", index)
			}
		})
	}
}
