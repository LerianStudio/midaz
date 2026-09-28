// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// heldWriteBehindDispatcher accepts every envelope without projecting it, the
// state an asynchronous deployment is in between the 201 and its consumer.
type heldWriteBehindDispatcher struct {
	mu        sync.Mutex
	envelopes []*command.TransactionWriteBehindEnvelope
}

func (dispatcher *heldWriteBehindDispatcher) DispatchTransactionWriteBehind(_ context.Context, envelope *command.TransactionWriteBehindEnvelope) error {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	dispatcher.envelopes = append(dispatcher.envelopes, envelope)

	return nil
}

// drain projects every held envelope in publication order, as the consumer would.
func (dispatcher *heldWriteBehindDispatcher) drain(t *testing.T, completer command.AppliedTransactionCompleter) {
	t.Helper()

	dispatcher.mu.Lock()
	held := dispatcher.envelopes
	dispatcher.envelopes = nil
	dispatcher.mu.Unlock()

	for _, envelope := range held {
		_, err := completer.Complete(context.Background(), &envelope.Record)
		require.NoError(t, err, "projecting transaction %s", envelope.Record.TransactionID)
	}
}

func TestIntegration_CrossLedgerLifecycleBeforeProjection(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	dispatcher := &heldWriteBehindDispatcher{}
	fixture.infra.handler.Command.TransactionWriteBehindAsync = true
	fixture.infra.handler.Command.TransactionWriteBehindDispatcher = dispatcher
	db := fixture.infra.pgContainer.DB

	crossLedgerPair := func(t *testing.T, sourceAlias, destinationAlias string) (uuid.UUID, uuid.UUID) {
		t.Helper()

		ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
		fixture.setCrossLedgerEnabled(t, ledgerA, true)
		fixture.setCrossLedgerEnabled(t, ledgerB, true)
		seedTransfer(t, db, fixture.infra.orgID, ledgerA, sourceAlias, "@external/USD", 100)
		seedTransfer(t, db, fixture.infra.orgID, ledgerB, "@external/USD", destinationAlias, 100)

		return ledgerA, ledgerB
	}

	create := func(t *testing.T, action string, request CreateTransactionV2Request, key string) CreateTransactionV2Response {
		t.Helper()

		raw, err := json.Marshal(request)
		require.NoError(t, err)
		response := postTransaction(t, fixture.app, v2CreateURL(action), string(raw), key)
		body := drainBody(t, response)
		require.Equal(t, http.StatusCreated, response.StatusCode, "%s body: %s", action, string(body))

		var created CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(body, &created))
		require.NotNil(t, created.GroupID)

		return created
	}

	lifecycle := func(t *testing.T, url, key string, wantStatus int) (CreateTransactionV2Response, []byte) {
		t.Helper()

		response := postTransaction(t, fixture.app, url, "", key)
		body := drainBody(t, response)
		require.Equal(t, wantStatus, response.StatusCode, "body: %s", string(body))

		var decoded CreateTransactionV2Response
		if wantStatus == http.StatusCreated {
			require.NoError(t, json.Unmarshal(body, &decoded))
		}

		return decoded, body
	}

	indexedExecution := func(t *testing.T, ledgerID uuid.UUID, transactionID string) uuid.UUID {
		t.Helper()

		repository, ok := fixture.infra.redisRepo.(redistransaction.EngineWriteBehindRepository)
		require.True(t, ok)
		raw, err := repository.GetEngineTransactionIndex(context.Background(), fixture.infra.orgID, ledgerID, uuid.MustParse(transactionID))
		require.NoError(t, err)
		index, err := command.DecodeTransactionEvidenceIndex(raw)
		require.NoError(t, err)

		return index.ExecutionID
	}

	requireUnprojected := func(t *testing.T, ledgers ...uuid.UUID) {
		t.Helper()

		for _, ledgerID := range ledgers {
			require.Zero(t, countTransactionsInLedger(t, db, ledgerID), "the scenario must run before any projection")
		}
	}

	requireConverged := func(t *testing.T, groupID string, want []*AtomicTransactionBatchV2Transaction) {
		t.Helper()

		rows, err := db.Query(`SELECT id, status FROM transaction WHERE group_id = $1 AND deleted_at IS NULL`, groupID)
		require.NoError(t, err)
		defer rows.Close()

		got := make(map[string]string)
		for rows.Next() {
			var id, status string
			require.NoError(t, rows.Scan(&id, &status))
			got[id] = status
		}
		require.NoError(t, rows.Err())

		// A direct or reversal part answers CREATED and is persisted APPROVED.
		expected := make(map[string]string, len(want))
		for _, tran := range want {
			expected[tran.ID] = tran.Status.Code
			if tran.Status.Code == constant.CREATED {
				expected[tran.ID] = constant.APPROVED
			}
		}
		require.Equal(t, expected, got, "the projected rows of group %s must match the responses", groupID)
	}

	t.Run("commit, repeat, then revert the committed group", func(t *testing.T) {
		ledgerA, ledgerB := crossLedgerPair(t, "@fresh-commit-source", "@fresh-commit-destination")
		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "fresh hold commit", "@fresh-commit-source", "@fresh-commit-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		held := create(t, "hold", request, "fresh-hold-commit")
		require.Len(t, held.Transactions, 1)
		requireUnprojected(t, ledgerA, ledgerB)

		holdExecution := indexedExecution(t, ledgerA, held.Transactions[0].ID)
		counted := &countingAtomicBatchEngine{delegate: fixture.engine}
		fixture.infra.handler.Command.Engine = counted
		t.Cleanup(func() { fixture.infra.handler.Command.Engine = fixture.engine })

		originID := uuid.MustParse(held.Transactions[0].ID)
		commitURL := v2CommitURL(fixture.infra.orgID, ledgerA, originID)
		committed, _ := lifecycle(t, commitURL, "", http.StatusCreated)
		require.NotNil(t, committed.GroupID)
		require.Equal(t, *held.GroupID, *committed.GroupID)
		require.Len(t, committed.Transactions, 2, "the commit answers with the origin and the destination it created")
		require.Equal(t, originID.String(), committed.Transactions[0].ID)
		for _, tran := range committed.Transactions {
			require.Equal(t, constant.APPROVED, tran.Status.Code)
		}
		require.Equal(t, int64(1), counted.calls.Load())
		requireCachedOnHold(t, fixture, ledgerA, "@fresh-commit-source", 0)
		requireCachedAvailable(t, fixture, ledgerB, "@fresh-commit-destination", 100)
		fixture.requireGroupEvent(t, *held.GroupID, events.TransactionGroupCommittedDefinition.Key())
		requireUnprojected(t, ledgerA, ledgerB)

		_, repeatBody := lifecycle(t, commitURL, "", http.StatusUnprocessableEntity)
		requireProblemCode(t, repeatBody, constant.ErrCrossLedgerGroupNotPending.Error())
		require.Equal(t, int64(1), counted.calls.Load(), "a repeated commit must not reach the engine")

		// The origin's index now names the commit execution, whose manifest
		// lists the destination too, so a revert through the origin sees the
		// complete group.
		commitExecution := indexedExecution(t, ledgerB, committed.Transactions[1].ID)
		require.NotEqual(t, holdExecution, commitExecution)
		require.Equal(t, commitExecution, indexedExecution(t, ledgerA, originID.String()))

		reverted, _ := lifecycle(t, v2RevertURL(fixture.infra.orgID, ledgerA, originID), "fresh-committed-group-revert", http.StatusCreated)
		require.NotNil(t, reverted.RevertedGroupID)
		require.Equal(t, *held.GroupID, *reverted.RevertedGroupID)
		require.Len(t, reverted.Transactions, 2, "the revert reverses the complete approved group")
		require.Equal(t, int64(2), counted.calls.Load())
		requireCachedAvailable(t, fixture, ledgerA, "@fresh-commit-source", 100)
		requireCachedAvailable(t, fixture, ledgerB, "@fresh-commit-destination", 0)
		requireUnprojected(t, ledgerA, ledgerB)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		requireConverged(t, *held.GroupID, committed.Transactions)
		requireConverged(t, *reverted.GroupID, reverted.Transactions)
		require.Equal(t, constant.APPROVED, crossLedgerGroupStatus(t, fixture, *held.GroupID))
	})

	t.Run("cancel releases the origin without creating a destination", func(t *testing.T) {
		ledgerA, ledgerB := crossLedgerPair(t, "@fresh-cancel-source", "@fresh-cancel-destination")
		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "fresh hold cancel", "@fresh-cancel-source", "@fresh-cancel-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		held := create(t, "hold", request, "fresh-hold-cancel")
		require.Len(t, held.Transactions, 1)
		requireUnprojected(t, ledgerA, ledgerB)

		canceled, _ := lifecycle(t, v2CancelURL(fixture.infra.orgID, ledgerA, uuid.MustParse(held.Transactions[0].ID)), "", http.StatusCreated)
		require.Equal(t, *held.GroupID, *canceled.GroupID)
		require.Len(t, canceled.Transactions, 1)
		require.Equal(t, constant.CANCELED, canceled.Transactions[0].Status.Code)
		requireCachedAvailable(t, fixture, ledgerA, "@fresh-cancel-source", 100)
		requireCachedOnHold(t, fixture, ledgerA, "@fresh-cancel-source", 0)
		fixture.requireGroupEvent(t, *held.GroupID, events.TransactionGroupCanceledDefinition.Key())
		requireUnprojected(t, ledgerA, ledgerB)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		requireConverged(t, *held.GroupID, canceled.Transactions)
		require.Zero(t, countTransactionsInLedger(t, db, ledgerB))
		require.Equal(t, constant.CANCELED, crossLedgerGroupStatus(t, fixture, *held.GroupID))
	})

	t.Run("commit through a non-first origin approves every origin", func(t *testing.T) {
		ledgerA, ledgerB, ledgerC := fixture.newLedger(t), fixture.newLedger(t), fixture.newLedger(t)
		for _, ledgerID := range []uuid.UUID{ledgerA, ledgerB, ledgerC} {
			fixture.setCrossLedgerEnabled(t, ledgerID, true)
		}
		seedTransfer(t, db, fixture.infra.orgID, ledgerA, "@fresh-multi-a", "@external/USD", 100)
		seedTransfer(t, db, fixture.infra.orgID, ledgerC, "@fresh-multi-c", "@external/USD", 100)
		seedTransfer(t, db, fixture.infra.orgID, ledgerB, "@external/USD", "@fresh-multi-destination", 100)
		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "fresh two origin commit", "@fresh-multi-a", "@fresh-multi-destination", 100)
		request.Debits[0].Amount = "50"
		request.Debits = append(request.Debits, TransactionV2LegRequest{
			Alias: "@fresh-multi-c", OrganizationID: fixture.infra.orgID.String(), LedgerID: ledgerC.String(), Amount: "50",
		})
		request.Credits[0].LedgerID = ledgerB.String()
		held := create(t, "hold", request, "fresh-two-origin-hold")
		require.Len(t, held.Transactions, 2)
		requireUnprojected(t, ledgerA, ledgerB, ledgerC)

		second := held.Transactions[1]
		committed, _ := lifecycle(t, v2CommitURL(fixture.infra.orgID, uuid.MustParse(second.LedgerID), uuid.MustParse(second.ID)), "", http.StatusCreated)
		require.Len(t, committed.Transactions, 3, "both origins and the destination")
		committedIDs := make([]string, 0, len(committed.Transactions))
		for _, tran := range committed.Transactions {
			require.Equal(t, constant.APPROVED, tran.Status.Code)
			committedIDs = append(committedIDs, tran.ID)
		}
		heldIDs := []string{held.Transactions[0].ID, held.Transactions[1].ID}
		require.Subset(t, committedIDs, heldIDs)
		requireCachedOnHold(t, fixture, ledgerA, "@fresh-multi-a", 0)
		requireCachedOnHold(t, fixture, ledgerC, "@fresh-multi-c", 0)
		requireCachedAvailable(t, fixture, ledgerB, "@fresh-multi-destination", 100)
		requireUnprojected(t, ledgerA, ledgerB, ledgerC)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		requireConverged(t, *held.GroupID, committed.Transactions)
		for _, ledgerID := range []uuid.UUID{ledgerA, ledgerB, ledgerC} {
			require.Equal(t, 1, countTransactionsInLedger(t, db, ledgerID))
		}
	})

	t.Run("revert a direct group", func(t *testing.T) {
		ledgerA, ledgerB := crossLedgerPair(t, "@fresh-direct-source", "@fresh-direct-destination")
		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "fresh direct revert", "@fresh-direct-source", "@fresh-direct-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		direct := create(t, "direct", request, "fresh-direct")
		require.Len(t, direct.Transactions, 2)
		requireUnprojected(t, ledgerA, ledgerB)

		member := direct.Transactions[1]
		revertURL := v2RevertURL(fixture.infra.orgID, uuid.MustParse(member.LedgerID), uuid.MustParse(member.ID))
		reverted, _ := lifecycle(t, revertURL, "fresh-direct-group-revert", http.StatusCreated)
		require.NotNil(t, reverted.RevertedGroupID)
		require.Equal(t, *direct.GroupID, *reverted.RevertedGroupID)
		require.NotEqual(t, *direct.GroupID, *reverted.GroupID)
		require.Len(t, reverted.Transactions, 2)
		for index, reversal := range reverted.Transactions {
			require.NotNil(t, reversal.ParentTransactionID)
			require.Equal(t, direct.Transactions[len(direct.Transactions)-1-index].ID, *reversal.ParentTransactionID)
		}
		requireCachedAvailable(t, fixture, ledgerA, "@fresh-direct-source", 100)
		requireCachedAvailable(t, fixture, ledgerB, "@fresh-direct-destination", 0)
		fixture.requireGroupEvent(t, *reverted.GroupID, events.TransactionGroupRevertedDefinition.Key())
		requireUnprojected(t, ledgerA, ledgerB)

		replay := postTransaction(t, fixture.app, revertURL, "", "fresh-direct-group-revert")
		replayBody := drainBody(t, replay)
		require.Equal(t, http.StatusCreated, replay.StatusCode, "body: %s", string(replayBody))
		require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
		var replayed CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(replayBody, &replayed))
		require.Equal(t, *reverted.GroupID, *replayed.GroupID)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		requireConverged(t, *direct.GroupID, direct.Transactions)
		requireConverged(t, *reverted.GroupID, reverted.Transactions)
	})
}
