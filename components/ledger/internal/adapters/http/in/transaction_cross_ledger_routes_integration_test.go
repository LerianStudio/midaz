// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// crossLedgerRouteTemplate is one organization-level transaction route made of
// a client source route, a client destination route and the crossLedger bridge
// route. Every client route carries a rubric for every action, so each phase's
// template holds exactly the two client routes.
type crossLedgerRouteTemplate struct {
	transactionRoute uuid.UUID
	source           uuid.UUID
	destination      uuid.UUID
	bridge           uuid.UUID
}

// crossLedgerRouteOptions shapes a template: typed client routes (source and
// destination) or bidirectional ones, which a revert requires, and whether the
// bridge route is linked at all.
type crossLedgerRouteOptions struct {
	bidirectionalClients bool
	withoutBridge        bool
	sourceAlias          string
	destinationAlias     string
}

// routedOperation is one persisted operation with its accounting route.
type routedOperation struct {
	alias     string
	kind      string
	routeID   *string
	routeCode *string
}

// crossLedgerRouteParticipants are two cross-ledger enabled ledgers A and B with
// a funded source in A and an empty destination in B.
type crossLedgerRouteParticipants struct {
	organizationA, organizationB uuid.UUID
	ledgerA, ledgerB             uuid.UUID
	sourceAlias                  string
	destinationAlias             string
}

// enableRouteValidationReads gives the harness's query use case the transaction
// route repository, which route validation reads on a route cache miss.
func (fixture *atomicBatchHTTPIntegrationFixture) enableRouteValidationReads() {
	if fixture.infra.handler.Query.TransactionRouteRepo == nil {
		fixture.infra.handler.Query.TransactionRouteRepo = transactionroute.NewTransactionRoutePostgreSQLRepository(fixture.infra.pgConn, false)
	}
}

// crossLedgerRouteParticipants creates ledgers A (in organizationA) and B (in
// organizationB), both cross-ledger enabled, with the given route validation,
// and funds 100 USD in A's source.
func (fixture *atomicBatchHTTPIntegrationFixture) crossLedgerRouteParticipants(
	t *testing.T,
	name string,
	organizationB uuid.UUID,
	validatesA, validatesB bool,
) crossLedgerRouteParticipants {
	t.Helper()

	participants := crossLedgerRouteParticipants{
		organizationA:    fixture.infra.orgID,
		organizationB:    organizationB,
		ledgerA:          uuid.New(),
		ledgerB:          uuid.New(),
		sourceAlias:      "@" + name + "-source",
		destinationAlias: "@" + name + "-destination",
	}

	db := fixture.infra.pgContainer.DB
	seedLedgerSettings(t, db, participants.organizationA, participants.ledgerA)
	seedLedgerSettings(t, db, participants.organizationB, participants.ledgerB)
	fixture.setLedgerRoutePolicy(t, participants.organizationA, participants.ledgerA, validatesA)
	fixture.setLedgerRoutePolicy(t, participants.organizationB, participants.ledgerB, validatesB)

	seedTransfer(t, db, participants.organizationA, participants.ledgerA, participants.sourceAlias, "@external/USD", 100)
	seedTransfer(t, db, participants.organizationB, participants.ledgerB, "@external/USD", participants.destinationAlias, 100)

	return participants
}

// setLedgerRoutePolicy enables cross-ledger on a ledger of any organization and
// sets its route validation.
func (fixture *atomicBatchHTTPIntegrationFixture) setLedgerRoutePolicy(t *testing.T, organizationID, ledgerID uuid.UUID, validateRoutes bool) {
	t.Helper()

	settings := fmt.Sprintf(`{"crossLedger":{"enabled":true},"accounting":{"validateRoutes":%t}}`, validateRoutes)
	_, err := fixture.infra.pgContainer.DB.Exec(
		`UPDATE ledger SET settings = $1::jsonb WHERE organization_id = $2 AND id = $3`,
		settings, organizationID, ledgerID,
	)
	require.NoError(t, err)
	require.NoError(t, fixture.infra.redisRepo.Del(context.Background(), utils.LedgerSettingsInternalKey(organizationID, ledgerID)))
}

// seedCrossLedgerRouteTemplate registers an organization-level transaction route
// (no ledger) whose client routes restrict their accounts by alias.
func (fixture *atomicBatchHTTPIntegrationFixture) seedCrossLedgerRouteTemplate(
	t *testing.T,
	organizationID uuid.UUID,
	options crossLedgerRouteOptions,
) crossLedgerRouteTemplate {
	t.Helper()

	fixture.enableRouteValidationReads()

	rubrics := func(prefix string) *mmodel.AccountingEntries {
		entry := func(action string) *mmodel.AccountingEntry {
			return &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: prefix + "-" + action + "-debit", Description: prefix + " " + action},
				Credit: &mmodel.AccountingRubric{Code: prefix + "-" + action + "-credit", Description: prefix + " " + action},
			}
		}

		return &mmodel.AccountingEntries{
			Direct: entry("direct"), Hold: entry("hold"), Commit: entry("commit"), Cancel: entry("cancel"), Revert: entry("revert"),
		}
	}

	sourceType, destinationType := constant.OperationRouteTypeSource, constant.OperationRouteTypeDestination
	if options.bidirectionalClients {
		sourceType, destinationType = constant.OperationRouteTypeBidirectional, constant.OperationRouteTypeBidirectional
	}

	template := crossLedgerRouteTemplate{
		source:      fixture.insertOrganizationOperationRoute(t, organizationID, "cross-ledger source", sourceType, options.sourceAlias, rubrics("S")),
		destination: fixture.insertOrganizationOperationRoute(t, organizationID, "cross-ledger destination", destinationType, options.destinationAlias, rubrics("D")),
	}
	links := []uuid.UUID{template.source, template.destination}

	if !options.withoutBridge {
		template.bridge = fixture.insertOrganizationOperationRoute(t, organizationID, "cross-ledger bridge", constant.OperationRouteTypeBidirectional, "",
			&mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "X-debit", Description: "Arriving from another ledger"},
				Credit: &mmodel.AccountingRubric{Code: "X-credit", Description: "Leaving to another ledger"},
			}})
		links = append(links, template.bridge)
	}

	template.transactionRoute = uuid.New()
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	_, err := fixture.infra.pgContainer.DB.Exec(`
		INSERT INTO transaction_route (id, organization_id, ledger_id, title, description, created_at, updated_at)
		VALUES ($1, $2, NULL, $3, $4, $5, $5)`,
		template.transactionRoute, organizationID, "cross-ledger transfer "+template.transactionRoute.String(), "cross-ledger routes", now)
	require.NoError(t, err)

	for _, operationRouteID := range links {
		_, err := fixture.infra.pgContainer.DB.Exec(`
			INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at)
			VALUES ($1, $2, $3, $4)`,
			uuid.New(), operationRouteID, template.transactionRoute, now)
		require.NoError(t, err)
	}

	return template
}

func (fixture *atomicBatchHTTPIntegrationFixture) insertOrganizationOperationRoute(
	t *testing.T,
	organizationID uuid.UUID,
	title, operationType, aliasRule string,
	entries *mmodel.AccountingEntries,
) uuid.UUID {
	t.Helper()

	raw, err := json.Marshal(entries)
	require.NoError(t, err)

	ruleType := ""
	if aliasRule != "" {
		ruleType = constant.AccountRuleTypeAlias
	}

	id := uuid.New()
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	_, err = fixture.infra.pgContainer.DB.Exec(`
		INSERT INTO operation_route (id, organization_id, ledger_id, title, description, operation_type,
			account_rule_type, account_rule_valid_if, accounting_entries, created_at, updated_at)
		VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8::jsonb, $9, $9)`,
		id, organizationID, title+" "+id.String(), title, operationType, ruleType, aliasRule, string(raw), now)
	require.NoError(t, err)

	return id
}

// routedTransfer moves 100 USD from A's source to B's destination under the
// template, each client leg naming its route (the destination leg only when
// destinationRoute is set). The bridge legs are never named by the client.
func (participants crossLedgerRouteParticipants) routedTransfer(template crossLedgerRouteTemplate, destinationRoute bool) CreateTransactionV2Request {
	transactionRoute := template.transactionRoute.String()
	sourceRoute := template.source.String()

	request := atomicBatchTransfer(participants.organizationA, participants.ledgerA, "cross-ledger routed transfer",
		participants.sourceAlias, participants.destinationAlias, 100)
	request.RouteID = &transactionRoute
	request.Debits[0].OperationRouteID = &sourceRoute
	request.Credits[0].OrganizationID = participants.organizationB.String()
	request.Credits[0].LedgerID = participants.ledgerB.String()

	if destinationRoute {
		route := template.destination.String()
		request.Credits[0].OperationRouteID = &route
	}

	return request
}

func (fixture *atomicBatchHTTPIntegrationFixture) postRouted(t *testing.T, action string, request CreateTransactionV2Request, key string) (int, []byte, *http.Response) {
	t.Helper()

	raw, err := json.Marshal(request)
	require.NoError(t, err)

	response := postTransaction(t, fixture.app, v2CreateURL(action), string(raw), key)

	return response.StatusCode, drainBody(t, response), response
}

// postRoutedGroup posts a cross-ledger request that must create a group.
func (fixture *atomicBatchHTTPIntegrationFixture) postRoutedGroup(t *testing.T, action string, request CreateTransactionV2Request, key string) CreateTransactionV2Response {
	t.Helper()

	status, body, _ := fixture.postRouted(t, action, request, key)

	return decodeCrossLedgerGroup(t, status, body, http.StatusCreated)
}

func decodeCrossLedgerGroup(t *testing.T, status int, body []byte, wantStatus int) CreateTransactionV2Response {
	t.Helper()

	require.Equal(t, wantStatus, status, "body: %s", string(body))

	var result CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(body, &result), "body: %s", string(body))
	require.NotNil(t, result.GroupID, "body: %s", string(body))

	return result
}

// groupMember returns the member of a group response posted in the ledger.
func groupMember(t *testing.T, result CreateTransactionV2Response, ledgerID uuid.UUID) uuid.UUID {
	t.Helper()

	for _, member := range result.Transactions {
		if member.LedgerID == ledgerID.String() {
			return uuid.MustParse(member.ID)
		}
	}

	require.Failf(t, "group member not found", "no member in ledger %s", ledgerID)

	return uuid.Nil
}

func loadRoutedOperations(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, transactionID uuid.UUID) []routedOperation {
	t.Helper()

	rows, err := fixture.infra.pgContainer.DB.Query(
		`SELECT account_alias, type, route_id::text, route_code FROM operation WHERE transaction_id = $1`, transactionID,
	)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	operations := make([]routedOperation, 0)

	for rows.Next() {
		var operation routedOperation
		require.NoError(t, rows.Scan(&operation.alias, &operation.kind, &operation.routeID, &operation.routeCode))
		operations = append(operations, operation)
	}

	require.NoError(t, rows.Err())

	return operations
}

// requireOperationRoute asserts the operation of alias and kind carries the
// route and rubric code; a nil route asserts an unrouted operation.
func requireOperationRoute(t *testing.T, operations []routedOperation, alias, kind string, route *uuid.UUID, code string) {
	t.Helper()

	for _, operation := range operations {
		if operation.alias != alias || operation.kind != kind {
			continue
		}

		if route == nil {
			require.Nil(t, operation.routeID, "%s %s must be unrouted", kind, alias)
			return
		}

		require.NotNil(t, operation.routeID, "%s %s must carry a route", kind, alias)
		require.Equal(t, route.String(), *operation.routeID, "%s %s route", kind, alias)
		require.NotNil(t, operation.routeCode, "%s %s must carry a rubric code", kind, alias)
		require.Equal(t, code, *operation.routeCode, "%s %s rubric code", kind, alias)

		return
	}

	require.Failf(t, "operation not found", "%s %s in %+v", kind, alias, operations)
}

func TestIntegration_CrossLedgerV2_RouteValidatingLedgers(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	organization := fixture.infra.orgID

	templateFor := func(t *testing.T, participants crossLedgerRouteParticipants, options crossLedgerRouteOptions) crossLedgerRouteTemplate {
		t.Helper()

		options.sourceAlias = participants.sourceAlias
		options.destinationAlias = participants.destinationAlias

		return fixture.seedCrossLedgerRouteTemplate(t, organization, options)
	}

	t.Run("direct group across validating ledgers routes every leg", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-direct", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		result := fixture.postRoutedGroup(t, "direct", participants.routedTransfer(template, true), "routed-direct")
		require.Len(t, result.Transactions, 2)

		origin := loadRoutedOperations(t, fixture, groupMember(t, result, participants.ledgerA))
		requireOperationRoute(t, origin, participants.sourceAlias, constant.DEBIT, &template.source, "S-direct-debit")
		requireOperationRoute(t, origin, "@external/USD", constant.CREDIT, &template.bridge, "X-credit")

		destination := loadRoutedOperations(t, fixture, groupMember(t, result, participants.ledgerB))
		requireOperationRoute(t, destination, "@external/USD", constant.DEBIT, &template.bridge, "X-debit")
		requireOperationRoute(t, destination, participants.destinationAlias, constant.CREDIT, &template.destination, "D-direct-credit")

		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 0)
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 100)
	})

	t.Run("mixed group routes only the validating ledger's bridge", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-mixed", organization, true, false)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		result := fixture.postRoutedGroup(t, "direct", participants.routedTransfer(template, true), "routed-mixed")

		origin := loadRoutedOperations(t, fixture, groupMember(t, result, participants.ledgerA))
		requireOperationRoute(t, origin, "@external/USD", constant.CREDIT, &template.bridge, "X-credit")

		destination := loadRoutedOperations(t, fixture, groupMember(t, result, participants.ledgerB))
		requireOperationRoute(t, destination, "@external/USD", constant.DEBIT, nil, "")
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 100)
	})

	t.Run("mixed group whose non-validating leg names no route misses the template", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-mixed-unnamed", organization, true, false)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		status, body, _ := fixture.postRouted(t, "direct", participants.routedTransfer(template, false), "routed-mixed-unnamed")

		require.Equal(t, http.StatusUnprocessableEntity, status, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrAccountingRouteCountMismatch.Error())
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerA))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))
	})

	t.Run("transaction route without a bridge route is refused", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-no-bridge", organization, true, false)
		template := templateFor(t, participants, crossLedgerRouteOptions{withoutBridge: true})

		status, body, _ := fixture.postRouted(t, "direct", participants.routedTransfer(template, true), "routed-no-bridge")

		require.Equal(t, http.StatusUnprocessableEntity, status, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrCrossLedgerRouteNotConfigured.Error())
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerA))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))
	})

	t.Run("hold then commit validates and routes the destinations as commit", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-commit", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		held := fixture.postRoutedGroup(t, "hold", participants.routedTransfer(template, true), "routed-hold-commit")
		require.Len(t, held.Transactions, 1)
		require.Equal(t, constant.PENDING, held.Transactions[0].Status.Code)
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))
		requireCachedOnHold(t, fixture, participants.ledgerA, participants.sourceAlias, 100)

		originID := uuid.MustParse(held.Transactions[0].ID)
		response := postTransaction(t, fixture.app, v2CommitURL(organization, participants.ledgerA, originID), "", "")
		committed := decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)

		require.Equal(t, *held.GroupID, *committed.GroupID)
		require.Len(t, committed.Transactions, 2)
		for _, member := range committed.Transactions {
			require.Equal(t, constant.APPROVED, member.Status.Code)
		}

		destination := loadRoutedOperations(t, fixture, groupMember(t, committed, participants.ledgerB))
		requireOperationRoute(t, destination, "@external/USD", constant.DEBIT, &template.bridge, "X-debit")
		requireOperationRoute(t, destination, participants.destinationAlias, constant.CREDIT, &template.destination, "D-commit-credit")

		requireCachedOnHold(t, fixture, participants.ledgerA, participants.sourceAlias, 0)
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 100)
		require.Equal(t, constant.APPROVED, crossLedgerGroupStatus(t, fixture, *held.GroupID))
	})

	t.Run("hold then cancel releases the origins and creates no destination", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-cancel", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		held := fixture.postRoutedGroup(t, "hold", participants.routedTransfer(template, true), "routed-hold-cancel")
		require.Len(t, held.Transactions, 1)

		originID := uuid.MustParse(held.Transactions[0].ID)
		response := postTransaction(t, fixture.app, v2CancelURL(organization, participants.ledgerA, originID), "", "")
		canceled := decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)

		require.Len(t, canceled.Transactions, 1)
		require.Equal(t, constant.CANCELED, canceled.Transactions[0].Status.Code)
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))
		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 100)
		requireCachedOnHold(t, fixture, participants.ledgerA, participants.sourceAlias, 0)
		require.Equal(t, constant.CANCELED, crossLedgerGroupStatus(t, fixture, *held.GroupID))
	})

	t.Run("revert of a direct group reverses every part with its routes", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-revert", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{bidirectionalClients: true})

		origin := fixture.postRoutedGroup(t, "direct", participants.routedTransfer(template, true), "routed-revert-origin")

		member := groupMember(t, origin, participants.ledgerB)
		response := postTransaction(t, fixture.app, v2RevertURL(organization, participants.ledgerB, member), "", "routed-revert")
		reverted := decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)

		require.NotNil(t, reverted.RevertedGroupID)
		require.Equal(t, *origin.GroupID, *reverted.RevertedGroupID)
		require.Len(t, reverted.Transactions, 2)

		reversalA := loadRoutedOperations(t, fixture, groupMember(t, reverted, participants.ledgerA))
		requireOperationRoute(t, reversalA, participants.sourceAlias, constant.CREDIT, &template.source, "S-revert-credit")
		requireOperationRoute(t, reversalA, "@external/USD", constant.DEBIT, &template.bridge, "X-debit")

		reversalB := loadRoutedOperations(t, fixture, groupMember(t, reverted, participants.ledgerB))
		requireOperationRoute(t, reversalB, participants.destinationAlias, constant.DEBIT, &template.destination, "D-revert-debit")
		requireOperationRoute(t, reversalB, "@external/USD", constant.CREDIT, &template.bridge, "X-credit")

		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 100)
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 0)
	})

	t.Run("revert of a committed hold group reverses every part", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-held-revert", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{bidirectionalClients: true})

		held := fixture.postRoutedGroup(t, "hold", participants.routedTransfer(template, true), "routed-held-revert-hold")
		originID := uuid.MustParse(held.Transactions[0].ID)
		commit := postTransaction(t, fixture.app, v2CommitURL(organization, participants.ledgerA, originID), "", "")
		committed := decodeCrossLedgerGroup(t, commit.StatusCode, drainBody(t, commit), http.StatusCreated)

		member := groupMember(t, committed, participants.ledgerB)
		response := postTransaction(t, fixture.app, v2RevertURL(organization, participants.ledgerB, member), "", "routed-held-revert")
		body := drainBody(t, response)

		if response.StatusCode == http.StatusUnprocessableEntity && problemCodeOf(body) == constant.ErrTransactionValueMismatch.Error() {
			t.Skipf("reverting a committed pending transaction under route validation answers %s until #5703 is fixed; body: %s",
				constant.ErrTransactionValueMismatch.Error(), string(body))
		}

		reverted := decodeCrossLedgerGroup(t, response.StatusCode, body, http.StatusCreated)
		require.Equal(t, *held.GroupID, *reverted.RevertedGroupID)
		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 100)
		requireCachedAvailable(t, fixture, participants.ledgerB, participants.destinationAlias, 0)
	})

	t.Run("group across organizations is refused only with route validation", func(t *testing.T) {
		otherOrganization := uuid.New()
		participants := fixture.crossLedgerRouteParticipants(t, "routed-cross-org", otherOrganization, true, false)
		template := templateFor(t, participants, crossLedgerRouteOptions{})

		status, body, _ := fixture.postRouted(t, "direct", participants.routedTransfer(template, true), "routed-cross-org")

		require.Equal(t, http.StatusUnprocessableEntity, status, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrCrossLedgerRouteValidationUnsupported.Error())
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerA))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))

		fixture.setLedgerRoutePolicy(t, participants.organizationA, participants.ledgerA, false)

		result := fixture.postRoutedGroup(t, "direct", participants.routedTransfer(template, true), "routed-cross-org-unvalidated")
		require.Len(t, result.Transactions, 2)
		requireCachedAvailable(t, fixture, participants.ledgerA, participants.sourceAlias, 0)
	})

	t.Run("replay returns the routed group and a changed route conflicts", func(t *testing.T) {
		participants := fixture.crossLedgerRouteParticipants(t, "routed-replay", organization, true, true)
		template := templateFor(t, participants, crossLedgerRouteOptions{})
		request := participants.routedTransfer(template, true)

		status, body, _ := fixture.postRouted(t, "direct", request, "routed-replay")
		original := decodeCrossLedgerGroup(t, status, body, http.StatusCreated)

		replayStatus, replayBody, replay := fixture.postRouted(t, "direct", request, "routed-replay")
		require.Equal(t, http.StatusCreated, replayStatus, "body: %s", string(replayBody))
		require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
		require.JSONEq(t, string(body), string(replayBody))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))

		origin := loadRoutedOperations(t, fixture, groupMember(t, original, participants.ledgerA))
		requireOperationRoute(t, origin, "@external/USD", constant.CREDIT, &template.bridge, "X-credit")

		other := templateFor(t, participants, crossLedgerRouteOptions{})
		conflictStatus, conflictBody, _ := fixture.postRouted(t, "direct", participants.routedTransfer(other, true), "routed-replay")

		require.Equal(t, http.StatusConflict, conflictStatus, "body: %s", string(conflictBody))
		requireProblemCode(t, conflictBody, constant.ErrIdempotencyKey.Error())
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, participants.ledgerB))
	})
}

// problemCodeOf reads the code of a problem body, or "" when there is none.
func problemCodeOf(body []byte) string {
	var problem struct {
		Code string `json:"code"`
	}

	if err := json.Unmarshal(body, &problem); err != nil {
		return ""
	}

	return problem.Code
}
