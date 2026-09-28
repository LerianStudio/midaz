// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// Accounting routes belong to the organization. These tests drive the production
// registrars against real PostgreSQL, MongoDB and Redis: routes created at organization
// level have no ledger, and the ledger paths reach every route of the organization.

// newAccountingRouteApp mounts the accounting-route registrars exactly as the unified
// server does — ledger paths on /v1 and /v2, organization paths on /v2 only — over the
// harness use cases, with the ledger's ErrorEnvelope on the app root.
func (h *feeHarness) newAccountingRouteApp() *fiber.App {
	h.commandUC.TransactionRouteRepo = h.queryUC.TransactionRouteRepo
	h.commandUC.OperationRouteRepo = h.queryUC.OperationRouteRepo

	trh := &TransactionRouteHandler{Command: h.commandUC, Query: h.queryUC}
	orh := &OperationRouteHandler{Command: h.commandUC, Query: h.queryUC}
	auth := &middleware.AuthClient{Enabled: false}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()
	pkgHTTP.InstallHumaFrameworkErrors()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	root := openapi.New(app, app, openapi.Config{Title: "accounting-routes", Version: "test", Servers: []string{"/"}})
	pkgHTTP.InstallLedgerSchemaNamer(root)

	v1, v1API := app.Group("/v1"), huma.NewGroup(root, "/v1")
	RegisterOperationRouteRoutesToApp(v1, v1API, auth, orh, nil)
	RegisterTransactionRouteRoutesToApp(v1, v1API, auth, trh, nil)

	v2, v2API := app.Group("/v2"), huma.NewGroup(root, "/v2")
	RegisterOperationRouteV2RoutesToApp(v2, v2API, auth, orh, nil)
	RegisterOrganizationOperationRouteV2RoutesToApp(v2, v2API, auth, orh, nil)
	RegisterTransactionRouteV2RoutesToApp(v2, v2API, auth, trh, nil)
	RegisterOrganizationTransactionRouteV2RoutesToApp(v2, v2API, auth, trh, nil)

	return app
}

func (h *feeHarness) ledgerRoutesPath(version string, ledgerID uuid.UUID, resource string) string {
	return "/" + version + "/organizations/" + h.orgID.String() + "/ledgers/" + ledgerID.String() + "/" + resource
}

func (h *feeHarness) organizationRoutesPath(resource string) string {
	return "/v2/organizations/" + h.orgID.String() + "/" + resource
}

// createRoute posts a route and returns its decoded body, failing unless it is created.
func createRoute(t *testing.T, app *fiber.App, path, body string) map[string]any {
	t.Helper()

	status, got := doJSON(t, app, http.MethodPost, path, body)
	require.Equalf(t, http.StatusCreated, status, "POST %s: %v", path, got)

	return got
}

func operationRouteBody(title, operationType, direction string) string {
	return `{"title":"` + title + `","operationType":"` + operationType + `",` +
		`"accountingEntries":{"direct":{"` + direction + `":{"code":"` + title + `","description":"` + title + `"}}}}`
}

func transactionRouteBody(title string, operationRouteIDs ...string) string {
	body := `{"title":"` + title + `","operationRoutes":[`
	for i, id := range operationRouteIDs {
		if i > 0 {
			body += ","
		}

		body += `"` + id + `"`
	}

	return body + `]}`
}

func listedIDs(t *testing.T, page map[string]any) []string {
	t.Helper()

	items, ok := page["items"].([]any)
	require.Truef(t, ok, "listing must carry items: %v", page)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}

	return ids
}

// TestOrganizationTransactionRoute_LinksOperationRoutesOfDifferentLedgers covers the
// organization-level create: operation routes created under two ledgers of the
// organization are linked by one transaction route that has no ledger, and a validating
// ledger accepts a transaction naming it.
func TestOrganizationTransactionRoute_LinksOperationRoutesOfDifferentLedgers(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	app := h.newAccountingRouteApp()

	ledgerA := postgrestestutil.CreateTestLedger(t, h.db, h.orgID)
	ledgerB := h.ledgerID

	source := createRoute(t, app, h.ledgerRoutesPath("v1", ledgerA, "operation-routes"), operationRouteBody("org-source", "source", "debit"))
	destination := createRoute(t, app, h.ledgerRoutesPath("v1", ledgerB, "operation-routes"), operationRouteBody("org-destination", "destination", "credit"))
	require.Equal(t, ledgerA.String(), source["ledgerId"])
	require.Equal(t, ledgerB.String(), destination["ledgerId"])

	sourceID, destinationID := source["id"].(string), destination["id"].(string)

	created := createRoute(t, app, h.organizationRoutesPath("transaction-routes"), transactionRouteBody("org settlement", sourceID, destinationID))

	assert.NotContains(t, created, "ledgerId", "a transaction route created at organization level has no ledger")
	assert.Equal(t, h.orgID.String(), created["organizationId"])

	links, ok := created["operationRoutes"].([]any)
	require.True(t, ok, "created route must list its operation routes: %v", created)

	linked := make([]string, 0, len(links))
	for _, link := range links {
		linked = append(linked, link.(map[string]any)["id"].(string))
	}

	assert.ElementsMatch(t, []string{sourceID, destinationID}, linked)

	var storedLedger *uuid.UUID

	require.NoError(t, h.db.QueryRow(`SELECT ledger_id FROM transaction_route WHERE id = $1`, created["id"]).Scan(&storedLedger))
	assert.Nil(t, storedLedger, "the stored row has no ledger")

	t.Run("organization path reads it back", func(t *testing.T) {
		status, got := doJSON(t, app, http.MethodGet, h.organizationRoutesPath("transaction-routes")+"/"+created["id"].(string), "")
		require.Equalf(t, http.StatusOK, status, "body: %v", got)
		assert.NotContains(t, got, "ledgerId")
	})

	t.Run("validating ledger accepts a transaction naming it", func(t *testing.T) {
		postgrestestutil.SetLedgerSettings(t, h.db, ledgerB, map[string]any{
			"accounting": map[string]any{"validateRoutes": true},
		})

		h.seedBalance(t, "@org-route-payer", "BRL", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@org-route-receiver", "BRL", decimal.Zero, "deposit")

		body := h.v2RoutedBody("organization route", "BRL", "100", uuid.MustParse(created["id"].(string)),
			[]string{h.v2RoutedLeg("@org-route-payer", "100", uuid.MustParse(sourceID))},
			[]string{h.v2RoutedLeg("@org-route-receiver", "100", uuid.MustParse(destinationID))})

		resp := h.createV2Direct(t, h.newV2App(), body, nil)
		require.Equalf(t, http.StatusCreated, resp.status, "body: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 2)

		for _, leg := range legs {
			require.NotNil(t, leg.Route, "every operation carries its route")
			assert.Contains(t, []string{sourceID, destinationID}, *leg.Route)
		}
	})
}

// TestLedgerAccountingRoutePaths_ReachEveryRouteOfTheOrganization covers the ledger
// paths after routes moved to the organization: create records the path ledger, and
// list, get, patch and delete reach routes created under another ledger or at
// organization level.
func TestLedgerAccountingRoutePaths_ReachEveryRouteOfTheOrganization(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newAccountingRouteApp()

	ledgerA := h.ledgerID
	ledgerB := postgrestestutil.CreateTestLedger(t, h.db, h.orgID)

	opUnderA := createRoute(t, app, h.ledgerRoutesPath("v1", ledgerA, "operation-routes"), operationRouteBody("source-a", "source", "debit"))
	opUnderB := createRoute(t, app, h.ledgerRoutesPath("v1", ledgerB, "operation-routes"), operationRouteBody("destination-b", "destination", "credit"))
	opAtOrg := createRoute(t, app, h.organizationRoutesPath("operation-routes"), operationRouteBody("source-org", "source", "debit"))

	assert.NotContains(t, opAtOrg, "ledgerId", "an operation route created at organization level has no ledger")

	opIDs := []string{opUnderA["id"].(string), opUnderB["id"].(string), opAtOrg["id"].(string)}

	trUnderA := createRoute(t, app, h.ledgerRoutesPath("v1", ledgerA, "transaction-routes"), transactionRouteBody("under-a", opIDs[0], opIDs[1]))
	trUnderB := createRoute(t, app, h.ledgerRoutesPath("v2", ledgerB, "transaction-routes"), transactionRouteBody("under-b", opIDs[2], opIDs[1]))
	trAtOrg := createRoute(t, app, h.organizationRoutesPath("transaction-routes"), transactionRouteBody("at-org", opIDs[0], opIDs[1]))

	t.Run("create under a ledger records that ledger", func(t *testing.T) {
		assert.Equal(t, ledgerA.String(), trUnderA["ledgerId"])
		assert.Equal(t, ledgerB.String(), trUnderB["ledgerId"])
	})

	trIDs := []string{trUnderA["id"].(string), trUnderB["id"].(string), trAtOrg["id"].(string)}

	t.Run("ledger listings return every route of the organization", func(t *testing.T) {
		for _, version := range []string{"v1", "v2"} {
			status, page := doJSON(t, app, http.MethodGet, h.ledgerRoutesPath(version, ledgerA, "transaction-routes")+"?limit=100", "")
			require.Equalf(t, http.StatusOK, status, "body: %v", page)
			assert.ElementsMatchf(t, trIDs, listedIDs(t, page), "%s transaction routes", version)

			status, page = doJSON(t, app, http.MethodGet, h.ledgerRoutesPath(version, ledgerA, "operation-routes")+"?limit=100", "")
			require.Equalf(t, http.StatusOK, status, "body: %v", page)
			assert.ElementsMatchf(t, opIDs, listedIDs(t, page), "%s operation routes", version)
		}
	})

	t.Run("organization listings return every route of the organization", func(t *testing.T) {
		status, page := doJSON(t, app, http.MethodGet, h.organizationRoutesPath("transaction-routes")+"?limit=100", "")
		require.Equalf(t, http.StatusOK, status, "body: %v", page)
		assert.ElementsMatch(t, trIDs, listedIDs(t, page))

		status, page = doJSON(t, app, http.MethodGet, h.organizationRoutesPath("operation-routes")+"?limit=100", "")
		require.Equalf(t, http.StatusOK, status, "body: %v", page)
		assert.ElementsMatch(t, opIDs, listedIDs(t, page))
	})

	t.Run("get and patch through another ledger reach the route", func(t *testing.T) {
		path := h.ledgerRoutesPath("v1", ledgerA, "transaction-routes") + "/" + trIDs[1]

		status, got := doJSON(t, app, http.MethodGet, path, "")
		require.Equalf(t, http.StatusOK, status, "body: %v", got)
		assert.Equal(t, ledgerB.String(), got["ledgerId"], "the route keeps the ledger it was created under")

		status, got = doJSON(t, app, http.MethodPatch, path, `{"title":"renamed under a"}`)
		require.Equalf(t, http.StatusOK, status, "body: %v", got)
		assert.Equal(t, "renamed under a", got["title"])
		assert.Equal(t, ledgerB.String(), got["ledgerId"])

		opPath := h.ledgerRoutesPath("v1", ledgerA, "operation-routes") + "/" + opIDs[1]

		status, got = doJSON(t, app, http.MethodPatch, opPath, `{"title":"destination renamed"}`)
		require.Equalf(t, http.StatusOK, status, "body: %v", got)
		assert.Equal(t, ledgerB.String(), got["ledgerId"])
	})

	t.Run("delete through another ledger removes the route", func(t *testing.T) {
		path := h.ledgerRoutesPath("v1", ledgerB, "transaction-routes") + "/" + trIDs[0]

		status, got := doJSON(t, app, http.MethodDelete, path, "")
		require.Equalf(t, http.StatusNoContent, status, "body: %v", got)

		status, got = doJSON(t, app, http.MethodGet, h.organizationRoutesPath("transaction-routes")+"/"+trIDs[0], "")
		assert.Equalf(t, http.StatusNotFound, status, "body: %v", got)
	})

	t.Run("organization path updates and deletes a route created under a ledger", func(t *testing.T) {
		path := h.organizationRoutesPath("transaction-routes") + "/" + trIDs[1]

		status, got := doJSON(t, app, http.MethodPatch, path, `{"title":"renamed at org"}`)
		require.Equalf(t, http.StatusOK, status, "body: %v", got)
		assert.Equal(t, ledgerB.String(), got["ledgerId"])

		status, got = doJSON(t, app, http.MethodDelete, path, "")
		require.Equalf(t, http.StatusNoContent, status, "body: %v", got)
	})
}
