// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// A hold committed on a ledger that validates routes splits its source across the two
// executions: the hold writes the source DEBIT and the commit only releases the hold.
// These tests revert such a transaction while the engine index still names the commit
// execution, once after its completion was acknowledged and once while it is still
// pending projection, and prove the reversal returns the funds with the revert rubrics.
func TestIntegration_RevertCommittedHoldUnderRouteValidation(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	v1App := buildHumaTransactionApp(t, fixture.infra.handler, true)
	organization := fixture.infra.orgID
	db := fixture.infra.pgContainer.DB

	// routedLedger is a route-validating ledger whose source holds 100 USD, with a
	// transaction route whose bidirectional client routes carry a revert rubric.
	routedLedger := func(t *testing.T, name string) (uuid.UUID, crossLedgerRouteParticipants, crossLedgerRouteTemplate) {
		t.Helper()

		ledgerID := fixture.newLedger(t)
		fixture.setLedgerRoutePolicy(t, organization, ledgerID, true)
		participants := crossLedgerRouteParticipants{
			organizationA: organization, organizationB: organization, ledgerA: ledgerID, ledgerB: ledgerID,
			sourceAlias: "@" + name + "-source", destinationAlias: "@" + name + "-destination",
		}
		seedTransfer(t, db, organization, ledgerID, participants.sourceAlias, participants.destinationAlias, 100)

		template := fixture.seedCrossLedgerRouteTemplate(t, organization, crossLedgerRouteOptions{
			bidirectionalClients: true, withoutBridge: true,
			sourceAlias: participants.sourceAlias, destinationAlias: participants.destinationAlias,
		})

		return ledgerID, participants, template
	}

	holdAndCommit := func(t *testing.T, ledgerID uuid.UUID, participants crossLedgerRouteParticipants, template crossLedgerRouteTemplate, key string) uuid.UUID {
		t.Helper()

		status, body, _ := fixture.postRouted(t, "hold", participants.routedTransfer(template, true), key)
		require.Equal(t, http.StatusCreated, status, "hold body: %s", string(body))

		var held CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(body, &held))
		require.Nil(t, held.GroupID, "a single-ledger hold is not a group")
		originID := uuid.MustParse(held.ID)

		commit := postTransaction(t, fixture.app, v2CommitURL(organization, ledgerID, originID), "", "")
		commitBody := drainBody(t, commit)
		require.Equal(t, http.StatusCreated, commit.StatusCode, "commit body: %s", string(commitBody))

		requireCachedOnHold(t, fixture, ledgerID, participants.sourceAlias, 0)
		requireCachedAvailable(t, fixture, ledgerID, participants.sourceAlias, 0)
		requireCachedAvailable(t, fixture, ledgerID, participants.destinationAlias, 100)

		return originID
	}

	indexDurability := func(t *testing.T, ledgerID, transactionID uuid.UUID) string {
		t.Helper()

		repository, ok := fixture.infra.redisRepo.(redistransaction.EngineWriteBehindRepository)
		require.True(t, ok)
		raw, err := repository.GetEngineTransactionIndex(context.Background(), organization, ledgerID, transactionID)
		require.NoError(t, err, "the engine index must still name the commit execution")
		index, err := command.DecodeTransactionEvidenceIndex(raw)
		require.NoError(t, err)
		require.Equal(t, constant.ActionCommit, index.Action, "the index must name the commit execution, whose rows lack the source DEBIT")

		return index.DurabilityState
	}

	requireReversal := func(t *testing.T, ledgerID uuid.UUID, participants crossLedgerRouteParticipants, template crossLedgerRouteTemplate, reversalID uuid.UUID) {
		t.Helper()

		operations := loadRoutedOperations(t, fixture, reversalID)
		require.Len(t, operations, 2, "the reversal has one leg per side: %+v", operations)
		requireOperationRoute(t, operations, participants.destinationAlias, constant.DEBIT, &template.destination, "D-revert-debit")
		requireOperationRoute(t, operations, participants.sourceAlias, constant.CREDIT, &template.source, "S-revert-credit")

		requireCachedAvailable(t, fixture, ledgerID, participants.sourceAlias, 100)
		requireCachedAvailable(t, fixture, ledgerID, participants.destinationAlias, 0)
	}

	for _, version := range []string{"v1", "v2"} {
		t.Run(version+" revert after the commit completion was acknowledged", func(t *testing.T) {
			ledgerID, participants, template := routedLedger(t, "acked-"+version)
			originID := holdAndCommit(t, ledgerID, participants, template, "acked-hold-"+version)
			require.Equal(t, command.TransactionDurabilityComplete, indexDurability(t, ledgerID, originID))

			reversalID := postRevert(t, fixture, v1App, version, organization, ledgerID, originID)
			requireReversal(t, ledgerID, participants, template, reversalID)
		})
	}

	holdProjection := func(t *testing.T) *heldWriteBehindDispatcher {
		t.Helper()

		dispatcher := &heldWriteBehindDispatcher{}
		fixture.infra.handler.Command.TransactionWriteBehindAsync = true
		fixture.infra.handler.Command.TransactionWriteBehindDispatcher = dispatcher
		t.Cleanup(func() {
			fixture.infra.handler.Command.TransactionWriteBehindAsync = false
			fixture.infra.handler.Command.TransactionWriteBehindDispatcher = nil
		})

		return dispatcher
	}

	t.Run("v2 revert while the commit is pending projection", func(t *testing.T) {
		dispatcher := holdProjection(t)

		ledgerID, participants, template := routedLedger(t, "in-flight")
		originID := holdAndCommit(t, ledgerID, participants, template, "in-flight-hold")
		require.Equal(t, command.TransactionDurabilityPending, indexDurability(t, ledgerID, originID))
		require.Zero(t, countTransactionsInLedger(t, db, ledgerID), "the revert must run before any projection")

		reversalID := postRevert(t, fixture, v1App, "v2", organization, ledgerID, originID)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		requireReversal(t, ledgerID, participants, template, reversalID)
	})

	t.Run("v2 group revert while the commit is pending projection", func(t *testing.T) {
		dispatcher := holdProjection(t)

		participants := fixture.crossLedgerRouteParticipants(t, "in-flight-group", organization, true, true)
		template := fixture.seedCrossLedgerRouteTemplate(t, organization, crossLedgerRouteOptions{
			bidirectionalClients: true,
			sourceAlias:          participants.sourceAlias, destinationAlias: participants.destinationAlias,
		})

		held := fixture.postRoutedGroup(t, "hold", participants.routedTransfer(template, true), "in-flight-group-hold")
		originID := uuid.MustParse(held.Transactions[0].ID)
		commit := postTransaction(t, fixture.app, v2CommitURL(organization, participants.ledgerA, originID), "", "")
		committed := decodeCrossLedgerGroup(t, commit.StatusCode, drainBody(t, commit), http.StatusCreated)
		require.Equal(t, command.TransactionDurabilityPending, indexDurability(t, participants.ledgerA, originID))
		require.Zero(t, countTransactionsInLedger(t, db, participants.ledgerA), "the revert must run before any projection")

		member := groupMember(t, committed, participants.ledgerB)
		response := postTransaction(t, fixture.app, v2RevertURL(organization, participants.ledgerB, member), "", "in-flight-group-revert")
		reverted := decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)
		require.Equal(t, *held.GroupID, *reverted.RevertedGroupID)
		require.Len(t, reverted.Transactions, 2)
		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 100)
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 0)

		dispatcher.drain(t, fixture.infra.handler.Command.AppliedTransactionCompleter)
		reversalA := loadRoutedOperations(t, fixture, groupMember(t, reverted, participants.ledgerA))
		requireOperationRoute(t, reversalA, participants.sourceAlias, constant.CREDIT, &template.source, "S-revert-credit")
	})
}

// postRevert reverts transactionID through the contract version and returns the id of the
// reversal, requiring a 201.
func postRevert(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, v1App *fiber.App, version string, organizationID, ledgerID, transactionID uuid.UUID) uuid.UUID {
	t.Helper()

	app, url := fixture.app, v2RevertURL(organizationID, ledgerID, transactionID)
	if version == "v1" {
		app, url = v1App, v1RevertURL(organizationID, ledgerID, transactionID)
	}

	reverted := decodeTxResponse(t, postTransaction(t, app, url, "", ""), http.StatusCreated)
	reversalID, ok := reverted["id"].(string)
	require.True(t, ok, "the revert must answer the reversal: %+v", reverted)

	return uuid.MustParse(reversalID)
}
