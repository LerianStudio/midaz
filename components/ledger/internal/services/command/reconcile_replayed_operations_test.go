// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	replayedTransactionID = "33333333-3333-4333-8333-333333333333"
	balanceA              = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	balanceB              = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

// replayLeg builds one operation leg with the fields reconciliation compares.
func replayLeg(id, balanceID, balanceKey, opType string, versionBefore int64, amount string) *operation.Operation {
	value := decimal.RequireFromString(amount)
	version := versionBefore

	return &operation.Operation{
		ID: id, TransactionID: replayedTransactionID, BalanceID: balanceID, BalanceKey: balanceKey, Type: opType,
		Amount: operation.Amount{Value: &value}, Balance: operation.Balance{Version: &version},
	}
}

func legIDs(ops []*operation.Operation) []string {
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, op.ID)
	}

	return ids
}

func TestReconcileReplayedOperations(t *testing.T) {
	t.Parallel()

	const def = constant.DefaultBalanceKey

	for _, tc := range []struct {
		name      string
		rebuilt   []*operation.Operation
		persisted []*operation.Operation
		wantIDs   []string
		adopted   int
		reason    ReplayReconciliationMismatchReason
	}{
		{
			name:    "nothing persisted keeps every rebuilt leg",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			wantIDs: []string{"new-1", "new-2"},
		},
		{
			name:      "everything persisted adopts every persisted ID",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			persisted: []*operation.Operation{replayLeg("old-2", balanceB, def, constant.CREDIT, 7, "100.00"), replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100")},
			wantIDs:   []string{"old-1", "old-2"},
			adopted:   2,
		},
		{
			name:      "a partial write adopts the persisted leg and inserts the missing one",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			persisted: []*operation.Operation{replayLeg("old-2", balanceB, def, constant.CREDIT, 7, "100")},
			wantIDs:   []string{"new-1", "old-2"},
			adopted:   1,
		},
		{
			name:    "a persisted hold does not hide the rebuilt commit legs",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 5, "100"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			persisted: []*operation.Operation{
				replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("old-2", balanceA, def, constant.ONHOLD, 4, "100"),
			},
			wantIDs: []string{"new-1", "new-2"},
		},
		{
			name:    "a persisted hold does not hide the rebuilt cancel legs",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, constant.RELEASE, 5, "100"), replayLeg("new-2", balanceA, def, constant.CREDIT, 6, "100")},
			persisted: []*operation.Operation{
				replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("old-2", balanceA, def, constant.ONHOLD, 4, "100"),
			},
			wantIDs: []string{"new-1", "new-2"},
		},
		{
			name:      "a fully persisted repeated annotation leg adopts both persisted IDs in ID order",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 0, "10"), replayLeg("new-2", balanceA, def, constant.DEBIT, 0, "10")},
			persisted: []*operation.Operation{replayLeg("old-b", balanceA, def, constant.DEBIT, 0, "10"), replayLeg("old-a", balanceA, def, constant.DEBIT, 0, "10")},
			wantIDs:   []string{"old-a", "old-b"},
			adopted:   2,
		},
		{
			name:      "a partially persisted repeated annotation leg cannot tell which is missing",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 0, "10"), replayLeg("new-2", balanceA, def, constant.DEBIT, 0, "10")},
			persisted: []*operation.Operation{replayLeg("old-1", balanceA, def, constant.DEBIT, 0, "10")},
			reason:    ReplayMismatchCountMismatch,
		},
		{
			name:      "repeated legs on one balance with different amounts are told apart by amount",
			rebuilt:   []*operation.Operation{replayLeg("new-10", balanceA, def, constant.DEBIT, 0, "10"), replayLeg("new-20", balanceA, def, constant.DEBIT, 0, "20")},
			persisted: []*operation.Operation{replayLeg("old-10", balanceA, def, constant.DEBIT, 0, "10")},
			wantIDs:   []string{"old-10", "new-20"},
			adopted:   1,
		},
		{
			name:      "a leg persisted with the amount clipped by overdraft has no exact pair",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			persisted: []*operation.Operation{replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "50")},
			reason:    ReplayMismatchUnpairedPersisted,
		},
		{
			name:      "an earlier duplicate is not adopted",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "100")},
			persisted: []*operation.Operation{replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("old-2", balanceA, def, constant.DEBIT, 3, "100")},
			reason:    ReplayMismatchCountMismatch,
		},
		{
			name:    "an overdraft companion without a rebuilt pair is ignored",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, constant.DEBIT, 3, "50"), replayLeg("new-2", balanceB, def, constant.CREDIT, 7, "100")},
			persisted: []*operation.Operation{
				replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "50"), replayLeg("old-2", balanceB, def, constant.CREDIT, 7, "100"),
				replayLeg("old-3", balanceA, constant.OverdraftBalanceKey, constant.OVERDRAFT, 0, "50"),
			},
			wantIDs: []string{"old-1", "old-2"},
			adopted: 2,
		},
		{
			name:    "a hold persisted under another route setting is a mismatch, not a missing leg",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, constant.ONHOLD, 3, "100")},
			persisted: []*operation.Operation{
				replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100"), replayLeg("old-2", balanceA, def, constant.ONHOLD, 4, "100"),
			},
			reason: ReplayMismatchUnpairedPersisted,
		},
		{
			name:    "a rebuilt leg without type cannot be compared",
			rebuilt: []*operation.Operation{replayLeg("new-1", balanceA, def, "", 3, "100")},
			reason:  ReplayMismatchIncompleteRebuilt,
		},
		{
			name:      "an empty balance key matches the default balance key",
			rebuilt:   []*operation.Operation{replayLeg("new-1", balanceA, "", constant.DEBIT, 3, "100")},
			persisted: []*operation.Operation{replayLeg("old-1", balanceA, def, constant.DEBIT, 3, "100")},
			wantIDs:   []string{"old-1"},
			adopted:   1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rebuiltIDs := legIDs(tc.rebuilt)

			result := reconcileReplayedOperations(replayedTransactionID, tc.rebuilt, tc.persisted)

			assert.Equal(t, rebuiltIDs, legIDs(tc.rebuilt), "reconciliation must not mutate the rebuilt legs")

			if tc.reason != "" {
				assert.Equal(t, ReplayReconciliationMismatch, result.Verdict)
				assert.Equal(t, tc.reason, result.MismatchReason)
				assert.Nil(t, result.Operations, "a mismatch must hand back nothing to write")

				return
			}

			require.Equal(t, ReplayReconciliationResolved, result.Verdict, "mismatch reason %q", result.MismatchReason)
			assert.Equal(t, tc.wantIDs, legIDs(result.Operations))
			assert.Equal(t, tc.adopted, result.Adopted)
			assert.Equal(t, len(tc.rebuilt)-tc.adopted, result.Inserted)

			for i, op := range result.Operations {
				assert.Equal(t, tc.rebuilt[i].Type, op.Type, "adoption changes only the ID")
				assert.True(t, tc.rebuilt[i].Amount.Value.Equal(*op.Amount.Value), "adoption changes only the ID")
			}
		})
	}
}

func TestReconcileReplayedOperationsReadsThePersistedLegsOfTheTransaction(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.MustParse(replayedTransactionID)
	rebuilt := []*operation.Operation{replayLeg("new-1", balanceA, constant.DefaultBalanceKey, constant.DEBIT, 3, "100")}

	t.Run("reconciles against the legs the repository reads for the transaction", func(t *testing.T) {
		t.Parallel()

		repo := operation.NewMockRepository(gomock.NewController(t))
		repo.EXPECT().ListByTransactionPrimary(gomock.Any(), organizationID, ledgerID, transactionID).
			Return([]*operation.Operation{replayLeg("old-1", balanceA, constant.DefaultBalanceKey, constant.DEBIT, 3, "100")}, nil)

		result, err := (&UseCase{OperationRepo: repo}).ReconcileReplayedOperations(context.Background(), organizationID, ledgerID, transactionID, rebuilt)

		require.NoError(t, err)
		assert.Equal(t, []string{"old-1"}, legIDs(result.Operations))
	})

	t.Run("a failed read returns no verdict", func(t *testing.T) {
		t.Parallel()

		repo := operation.NewMockRepository(gomock.NewController(t))
		readErr := errors.New("primary unavailable")
		repo.EXPECT().ListByTransactionPrimary(gomock.Any(), organizationID, ledgerID, transactionID).Return(nil, readErr)

		result, err := (&UseCase{OperationRepo: repo}).ReconcileReplayedOperations(context.Background(), organizationID, ledgerID, transactionID, rebuilt)

		require.ErrorIs(t, err, readErr)
		assert.Empty(t, result.Verdict)
		assert.Nil(t, result.Operations)
	})
}
