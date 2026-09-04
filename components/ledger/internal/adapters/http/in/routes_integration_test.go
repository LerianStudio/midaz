// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	// orgLedgerPath is the org+ledger scope prefix the /v1 core-ledger routes hang off, in
	// the Fiber pattern form GetRoutes reports.
	orgLedgerPath = "/v1/organizations/:organization_id/ledgers/:ledger_id"
	// crmV2OrgLedger is the org+ledger scope prefix on /v2, where CRM and composition are
	// served. crmV2Org (crm_routes_test.go) is the org-only prefix.
	crmV2OrgLedger = crmV2Org + "/ledgers/:ledger_id"
	// settingsPath is the ledger-wide settings scope: no org/ledger params, and the
	// metadata-index params (entity_name, index_key) are not UUIDs.
	settingsPath = "/v1/settings/metadata-indexes"
	// concreteUUID stands in for any :uuid path param when a template is turned into a
	// request path.
	concreteUUID = "00000000-0000-0000-0000-000000000001"
)

// newLedgerHumaTestAPI builds the Fiber group and the Huma group for one version prefix the
// way the unified server's mountHumaContracts seam does: problem.Install() before any
// huma.Register, ONE document over the app root, then one Fiber group plus one
// huma.NewGroup per prefix, so op paths carry the prefix exactly as production serves them.
// Callers hand the returned pair to the production registrars.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError hook
// and Huma validation uses process-global sync.Pools — concurrent builds cross-contaminate.
func newLedgerHumaTestAPI(app *fiber.App, prefix string) (fiber.Router, huma.API) {
	libProblem.Install()

	root := openapi.New(app, app, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/"}})
	pkgHTTP.InstallLedgerSchemaNamer(root)

	return app.Group(prefix), huma.NewGroup(root, prefix)
}

// concreteRequestPath turns a Fiber route pattern into a requestable path by substituting a
// fixed UUID for every :param segment. Only the segments a route declares are touched, so a
// non-UUID param (entity_name) is substituted too — the auth chain runs before any parse
// handler, which is all the auth assertions need.
func concreteRequestPath(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = concreteUUID
		}
	}

	return strings.Join(segments, "/")
}

// splitRouteKey splits a "METHOD:path" route key back into its two halves.
func splitRouteKey(t *testing.T, key string) (method, path string) {
	t.Helper()

	method, path, ok := strings.Cut(key, ":")
	require.Truef(t, ok, "route key %q must be METHOD:path", key)

	return method, path
}

// mountBalanceRoutingSurface wires the balance, operation-read, transaction-count,
// operation-route and transaction-route registrars onto one /v1 group, exactly as the
// unified server mounts them. Registration never invokes the handlers, so zero-value
// handler structs are safe.
func mountBalanceRoutingSurface(app *fiber.App, auth *middleware.AuthClient) huma.API {
	group, hAPI := newLedgerHumaTestAPI(app, "/v1")

	RegisterBalanceRoutesToApp(group, hAPI, auth, &BalanceHandler{}, nil)
	RegisterOperationRoutesToApp(group, hAPI, auth, &OperationHandler{}, nil)
	RegisterCountTransactionRoutesToApp(group, hAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRouteRoutesToApp(group, hAPI, auth, &OperationRouteHandler{}, nil)
	RegisterTransactionRouteRoutesToApp(group, hAPI, auth, &TransactionRouteHandler{}, nil)

	return hAPI
}

// mountMoneyWriteSurface wires the money-write registrars onto one /v1 group: the
// transaction ops plus the operation PATCH (UpdateOperation, a double-entry leg).
func mountMoneyWriteSurface(app *fiber.App, auth *middleware.AuthClient) huma.API {
	group, hAPI := newLedgerHumaTestAPI(app, "/v1")

	RegisterTransactionHumaRoutesToApp(group, hAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRoutesToApp(group, hAPI, auth, &OperationHandler{}, nil)

	return hAPI
}

// mountCRMCompositionSurface wires the two v2-only additive registrars — composition and
// CRM (holders/instruments/holder-accounts/encryption/audit) — onto the /v2 group. Every
// conditional CRM handler is passed NON-nil so the FULL surface mounts; the nil-guard
// conditionality is TestCRMV2RoutesRespectNilGuards' subject (huma_contract_mount_test.go).
//
// Fees/billing are v2-only too but mount through their four per-resource registrars, exercised by
// fees_routes_test.go rather than here.
func mountCRMCompositionSurface(app *fiber.App, auth *middleware.AuthClient) huma.API {
	group, hAPI := newLedgerHumaTestAPI(app, "/v2")

	RegisterCompositionV2RoutesToApp(group, hAPI, auth, &CompositionHandler{}, nil)
	RegisterHolderV2RoutesToApp(group, hAPI, auth, &HolderHandler{}, nil)
	RegisterHolderAccountsV2RoutesToApp(group, hAPI, auth, &HolderAccountsHandler{}, nil)
	RegisterInstrumentV2RoutesToApp(group, hAPI, auth, &InstrumentHandler{}, nil)
	RegisterEncryptionV2RoutesToApp(group, hAPI, auth, &EncryptionHandler{}, nil)
	RegisterAuditV2RoutesToApp(group, hAPI, auth, &AuditHandler{}, nil)

	return hAPI
}

// mountMetadataIndexSurface wires the metadata-index registrar onto the /v1 group with the
// caller's route options, so a test can observe what the options chain contributes.
func mountMetadataIndexSurface(app *fiber.App, auth *middleware.AuthClient, opts *pkgHTTP.ProtectedRouteOptions) huma.API {
	group, hAPI := newLedgerHumaTestAPI(app, "/v1")

	RegisterMetadataIndexRoutesToApp(group, hAPI, auth, &MetadataIndexHandler{}, opts)

	return hAPI
}

// balanceRoutingRoutes is the balance + operation-read + count + routing surface the
// registrars must serve on the version group they are mounted on.
var balanceRoutingRoutes = []string{
	// Balance (10)
	"GET:" + orgLedgerPath + "/balances",
	"GET:" + orgLedgerPath + "/balances/:balance_id",
	"PATCH:" + orgLedgerPath + "/balances/:balance_id",
	"DELETE:" + orgLedgerPath + "/balances/:balance_id",
	"GET:" + orgLedgerPath + "/balances/:balance_id/history",
	"GET:" + orgLedgerPath + "/accounts/:account_id/balances",
	"POST:" + orgLedgerPath + "/accounts/:account_id/balances",
	"GET:" + orgLedgerPath + "/accounts/:account_id/balances/history",
	"GET:" + orgLedgerPath + "/accounts/alias/:alias/balances",
	"GET:" + orgLedgerPath + "/accounts/external/:code/balances",
	// Operation read (2)
	"GET:" + orgLedgerPath + "/accounts/:account_id/operations",
	"GET:" + orgLedgerPath + "/accounts/:account_id/operations/:operation_id",
	// Transaction count (1, explicit HEAD)
	"HEAD:" + orgLedgerPath + "/transactions/metrics/count",
	// Operation-route (5)
	"POST:" + orgLedgerPath + "/operation-routes",
	"GET:" + orgLedgerPath + "/operation-routes",
	"GET:" + orgLedgerPath + "/operation-routes/:operation_route_id",
	"PATCH:" + orgLedgerPath + "/operation-routes/:operation_route_id",
	"DELETE:" + orgLedgerPath + "/operation-routes/:operation_route_id",
	// Transaction-route (5)
	"POST:" + orgLedgerPath + "/transaction-routes",
	"GET:" + orgLedgerPath + "/transaction-routes",
	"GET:" + orgLedgerPath + "/transaction-routes/:transaction_route_id",
	"PATCH:" + orgLedgerPath + "/transaction-routes/:transaction_route_id",
	"DELETE:" + orgLedgerPath + "/transaction-routes/:transaction_route_id",
}

// moneyWriteRoutes is the byte-for-byte money-write surface the money-write registrars
// mount on the version group. Paths, methods and (resource, verb) tuples are
// non-negotiable — this is the money path.
var moneyWriteRoutes = []string{
	// Six CREATE ops — ("transactions","post").
	"POST:" + orgLedgerPath + "/transactions/json",
	"POST:" + orgLedgerPath + "/transactions/inflow",
	"POST:" + orgLedgerPath + "/transactions/outflow",
	"POST:" + orgLedgerPath + "/transactions/annotation",
	"POST:" + orgLedgerPath + "/transactions/block",
	"POST:" + orgLedgerPath + "/transactions/unblock",
	// Three STATE ops (id-only, bodiless) — ("transactions","post").
	"POST:" + orgLedgerPath + "/transactions/:transaction_id/commit",
	"POST:" + orgLedgerPath + "/transactions/:transaction_id/cancel",
	"POST:" + orgLedgerPath + "/transactions/:transaction_id/revert",
	// PATCH — ("transactions","patch").
	"PATCH:" + orgLedgerPath + "/transactions/:transaction_id",
	// Two READ ops — ("transactions","get").
	"GET:" + orgLedgerPath + "/transactions/:transaction_id",
	"GET:" + orgLedgerPath + "/transactions",
	// Operation PATCH (UpdateOperation, money-write leg) — ("operations","patch").
	"PATCH:" + orgLedgerPath + "/transactions/:transaction_id/operations/:operation_id",
}

// crmCompositionRoutes is the byte-for-byte surface the composition and CRM registrars
// mount on /v2 when every conditional handler is present.
var crmCompositionRoutes = []string{
	// CRM holders (5)
	"POST:" + crmV2Org + "/holders",
	"GET:" + crmV2Org + "/holders/:id",
	"PATCH:" + crmV2Org + "/holders/:id",
	"DELETE:" + crmV2Org + "/holders/:id",
	"GET:" + crmV2Org + "/holders",
	// CRM holder-accounts (1, conditional on hah)
	"GET:" + crmV2Org + "/holders/:id/accounts",
	// CRM instruments (6)
	"GET:" + crmV2Org + "/instruments",
	"POST:" + crmV2Org + "/holders/:holder_id/instruments",
	"GET:" + crmV2Org + "/holders/:holder_id/instruments/:instrument_id",
	"PATCH:" + crmV2Org + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + crmV2Org + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + crmV2Org + "/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id",
	// CRM encryption (2, conditional on eh)
	"POST:" + crmV2Org + "/encryption/provision",
	"GET:" + crmV2Org + "/encryption/status",
	// CRM audit (1, conditional on auditHandler)
	"GET:" + crmV2Org + "/protection/audit",
	// Composition (1)
	"POST:" + crmV2OrgLedger + "/holders/:id/accounts",
}

// metadataIndexRoutes is the settings surface the metadata-index registrar mounts. The path
// params are names, not UUIDs, so no ParseUUIDPathParameters sits in the chain.
var metadataIndexRoutes = []string{
	"POST:" + settingsPath + "/entities/:entity_name",
	"GET:" + settingsPath,
	"DELETE:" + settingsPath + "/entities/:entity_name/key/:index_key",
}

// TestBalanceRoutingSurfaceMountedOnGroup asserts every balance, operation-read,
// transaction-count and routing route is served on the /v1 group. A missing route means
// the auth-middleware attach or the Huma registration regressed.
func TestBalanceRoutingSurfaceMountedOnGroup(t *testing.T) {
	// NOT parallel: mountBalanceRoutingSurface mutates process-global huma state.
	app := fiber.New()

	mountBalanceRoutingSurface(app, &middleware.AuthClient{Enabled: false})

	assertRoutesMounted(t, app, balanceRoutingRoutes)
}

// TestMoneyWriteSurfaceMountedOnGroup asserts every money-write route is served on the
// /v1 group. A missing route means the auth-middleware attach or the Huma registration
// regressed on the double-entry path.
func TestMoneyWriteSurfaceMountedOnGroup(t *testing.T) {
	// NOT parallel: mountMoneyWriteSurface mutates process-global huma state.
	app := fiber.New()

	mountMoneyWriteSurface(app, &middleware.AuthClient{Enabled: false})

	assertRoutesMounted(t, app, moneyWriteRoutes)
}

// TestMoneyWriteSurfaceRequiresAuth proves NO money-write route is public: with auth
// enabled and no bearer token, every route on the surface is rejected with 401 before
// reaching the body-parsing or business handler. Mounting a route without its auth chain
// leaves the Huma terminal serving the path alone, which this catches as a non-401.
func TestMoneyWriteSurfaceRequiresAuth(t *testing.T) {
	// NOT parallel: mountMoneyWriteSurface mutates process-global huma state.
	app := fiber.New()

	// Address must be non-empty so Authorize enforces the token check. It is never
	// dialed: a missing token short-circuits with 401 first.
	mountMoneyWriteSurface(app, &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	for _, route := range moneyWriteRoutes {
		method, pattern := splitRouteKey(t, route)

		t.Run(method+" "+pattern, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(method, concreteRequestPath(pattern), nil), fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
				"money-write route must be guarded by its auth chain and reject a tokenless request with 401")
		})
	}
}

// TestCRMCompositionSurfaceMountedOnGroup asserts every composition and CRM route is
// served on the /v2 group when all conditional handlers are present. A missing route means
// the auth-middleware attach or the Huma registration regressed.
func TestCRMCompositionSurfaceMountedOnGroup(t *testing.T) {
	// NOT parallel: mountCRMCompositionSurface mutates process-global huma state.
	app := fiber.New()

	mountCRMCompositionSurface(app, &middleware.AuthClient{Enabled: false})

	assertRoutesMounted(t, app, crmCompositionRoutes)
}

// TestMetadataIndexSurfaceMountedOnGroup asserts the three metadata-index ops are served on
// the /v1 group at their exact paths and methods.
func TestMetadataIndexSurfaceMountedOnGroup(t *testing.T) {
	// NOT parallel: mountMetadataIndexSurface mutates process-global huma state.
	app := fiber.New()

	mountMetadataIndexSurface(app, &middleware.AuthClient{Enabled: false}, nil)

	assertRoutesMounted(t, app, metadataIndexRoutes)
}

// TestMetadataIndexSurfaceRunsPostAuthMiddlewares proves the ProtectedRouteOptions chain is
// actually invoked on a metadata-index request, not merely registered: the post-auth
// middleware is the seam multi-tenancy hangs tenant resolution off, so a route that mounts
// it but never runs it would resolve no tenant. The middleware short-circuits with 418 so
// the assertion needs no live business handler behind it.
func TestMetadataIndexSurfaceRunsPostAuthMiddlewares(t *testing.T) {
	// NOT parallel: mountMetadataIndexSurface mutates process-global huma state.
	app := fiber.New()

	called := 0
	opts := &pkgHTTP.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{
			func(c fiber.Ctx) error {
				called++

				return c.SendStatus(fiber.StatusTeapot)
			},
		},
	}

	mountMetadataIndexSurface(app, &middleware.AuthClient{Enabled: false}, opts)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, settingsPath, nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusTeapot, resp.StatusCode,
		"the post-auth middleware must run and own the response")
	assert.Equal(t, 1, called, "the post-auth middleware must run exactly once per request")
}

// assertRoutesMounted asserts every "METHOD:path" in want is registered on app.
func assertRoutesMounted(t *testing.T, app *fiber.App, want []string) {
	t.Helper()

	mounted := routeSetOf(app, "")

	for _, route := range want {
		assert.Truef(t, mounted[route], "expected mounted route %q", route)
	}
}
