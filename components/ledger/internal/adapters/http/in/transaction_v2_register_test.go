// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const directV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/direct"

const holdV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/hold"

const blockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/block"

const unblockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/unblock"

const commitV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit"

const cancelV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel"

const revertV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert"

// v2Routes enumerates every registered v2 transaction op: the Fiber path the protected
// chain mounts, the group-relative path the Huma contract advertises, and the OperationID
// clients key off. The create actions take the action name straight off the collection
// path; the lifecycle actions hang off :transaction_id. Defaulting to 201 Created holds
// for all of them, so it is asserted as a shared invariant instead of a per-case field.
// opPath is spelled out rather than derived from fiberPath so a typo in either const
// cannot pass both the mount and the contract assertion.
var v2Routes = []struct {
	action      string
	fiberPath   string
	opPath      string
	operationID string
}{
	{
		action:      "direct",
		fiberPath:   directV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/direct",
		operationID: "createTransactionDirectV2",
	},
	{
		action:      "hold",
		fiberPath:   holdV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/hold",
		operationID: "createTransactionHoldV2",
	},
	{
		action:      "block",
		fiberPath:   blockV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/block",
		operationID: "createTransactionBlockV2",
	},
	{
		action:      "unblock",
		fiberPath:   unblockV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/unblock",
		operationID: "createTransactionUnblockV2",
	},
	{
		action:      "commit",
		fiberPath:   commitV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit",
		operationID: "commitTransactionV2",
	},
	{
		action:      "cancel",
		fiberPath:   cancelV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel",
		operationID: "cancelTransactionV2",
	},
	{
		action:      "revert",
		fiberPath:   revertV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert",
		operationID: "revertTransactionV2",
	},
}

// concreteV2Path substitutes the Fiber path params with fixed UUIDs so a request reaches
// the mounted route (ParseUUIDPathParameters passes) instead of 404ing. Deriving it from
// fiberPath is deliberate here: it aims the auth assertion at the SAME route the mount
// test proves is registered, and a wrong path would surface as a 404 rather than the
// expected 401.
func concreteV2Path(fiberPath string) string {
	return strings.NewReplacer(
		":organization_id", "00000000-0000-0000-0000-000000000001",
		":ledger_id", "00000000-0000-0000-0000-000000000002",
		":transaction_id", "00000000-0000-0000-0000-000000000003",
	).Replace(fiberPath)
}

// registerV2TransactionRoutesForTest wires the v2 transaction ops onto a fresh Fiber app +
// its own /v2 Huma contract, exactly as the production humaMountV2 seam does. A zero-value
// TransactionHandler is safe because registration never invokes the handler.
func registerV2TransactionRoutesForTest(auth *middleware.AuthClient) *fiber.App {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPI, auth, &TransactionHandler{}, nil)

	return app
}

// TestRegisterTransactionV2RoutesToApp_MountsRoutes asserts every v2 transaction op is
// mounted as a POST on the /v2 group (Fiber chain), sharing the v1 transactions:post
// protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsRoutes(t *testing.T) {
	t.Parallel()

	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: false})

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			assert.Truef(t, routeSet[http.MethodPost+":"+rt.fiberPath],
				"should register POST %s", rt.fiberPath)
		})
	}
}

// TestRegisterTransactionV2Routes_RegistersHumaOperations asserts every v2 transaction op
// is present on the v2 Huma document at its group-relative path, advertising the canonical
// OperationID and defaulting to 201 Created.
func TestRegisterTransactionV2Routes_RegistersHumaOperations(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	paths := humaAPI.OpenAPI().Paths

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)
			require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)

			assert.Equalf(t, rt.operationID, pathItem.Post.OperationID,
				"%s op should advertise OperationID %s", rt.action, rt.operationID)
			assert.Equalf(t, http.StatusCreated, pathItem.Post.DefaultStatus,
				"%s op should default to 201 Created (create/lifecycle parity)", rt.action)
		})
	}
}

// TestV2Routes_RequireAuth proves every v2 transaction route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before reaching
// the handler.
func TestV2Routes_RequireAuth(t *testing.T) {
	t.Parallel()

	// Address must be non-empty so Authorize enforces the token check (it is never
	// dialed: a missing token short-circuits with 401 first).
	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, concreteV2Path(rt.fiberPath), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, fiber.StatusUnauthorized, resp.StatusCode,
				"tokenless v2 %s request must be rejected by the transactions:post auth chain", rt.action)
		})
	}
}
