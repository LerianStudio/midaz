// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// dashboardResource is the authz resource the three reads authorize under.
//
// It is DEDICATED rather than borrowed from `transactions` or `balances`, on
// the account-block-exceptions precedent: the dashboard publishes aggregate
// money figures over both tables, so folding it into either grant would widen
// what that grant means without anybody choosing it — a transactions:get would
// start disclosing balance positions, silently. A separate resource lets an
// operator hand somebody the dashboard without handing them row-level reads,
// and lets them withhold it while granting those.
//
// The cost is a deployment step: until the central Access-Manager seed grants
// (midaz, dashboard, get), a policy-enforcing environment answers 403. That is
// the fail-closed direction and it is the same sequence account-block-exceptions
// went through; components/ledger/permissions.yaml declares the pair, and
// components/ledger/docs/dashboard.md states the dependency for operators.
const dashboardResource = "dashboard"

// RegisterDashboardRoutes registers the three dashboard reads on the shared
// Huma API. The auth (auth.Authorize("midaz","dashboard","get")) + tenant +
// ParseUUIDPathParameters("dashboard") chain is attached in the unified server
// (Fiber level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (the group's PrefixModifier writes the version into each op's op.Path).
//
// opSuffix distinguishes the operation ID one version group publishes from
// another's — see v1OpSuffix.
func RegisterDashboardRoutes(api huma.API, h *DashboardHandler, opSuffix string) {
	const (
		base = "/organizations/{organization_id}/ledgers/{ledger_id}/dashboard"
		tag  = "Dashboard"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerDashboardMetrics" + opSuffix,
		Method:      http.MethodGet,
		Path:        base + "/metrics",
		Summary:     "Get ledger dashboard metrics",
		Description: "Returns, for the window, the total number of transactions, the count of each status the " +
			"ledger knows, and the settled volume per asset. Volume sums transaction.amount over SETTLED " +
			"transactions only — status APPROVED, the only status whose transactions moved money — and is " +
			"reported PER ASSET with no cross-asset total, because adding two assets together does not produce " +
			"money. Amounts are exact decimal strings whose scale is not normative; format them with the asset's " +
			"own exponent and never parse them as a binary float. volumeByAsset is the GROSS settled " +
			"amount: every settled leg inside the window, reversal legs included. reversalsByAsset is the " +
			"part of it made of reversal legs, same window and same settled filter. A true net is NOT " +
			"derivable from the two alone, because it depends on whether each reversed original also falls " +
			"inside the window; a consumer that needs net reads the transactions.",
		Tags:     []string{tag},
		Security: secDashboardBearer,
	}, h.GetDashboardMetrics)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerDashboardVolume" + opSuffix,
		Method:      http.MethodGet,
		Path:        base + "/volume",
		Summary:     "Get ledger dashboard volume series",
		Description: "Returns one point per UTC calendar day the window TOUCHES, including days with no " +
			"transactions, which are present with transactions 0 and an empty byAsset. A window is half-open and " +
			"snaps to the minute rather than to midnight, so a 7d window opened mid-afternoon returns EIGHT " +
			"points. Each point's transactions counts every status; its byAsset applies the same settled rule as " +
			"/metrics. Summing the points reproduces /metrics exactly, and is likewise GROSS of reversals.",
		Tags:     []string{tag},
		Security: secDashboardBearer,
	}, h.GetDashboardVolume)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerDashboardAssets" + opSuffix,
		Method:      http.MethodGet,
		Path:        base + "/assets",
		Summary:     "Get ledger current position per asset",
		Description: "Returns the ledger's CURRENT position in each asset — how many distinct accounts hold it, " +
			"and the summed available and on-hold figures — read from the balance table. It takes NO window and " +
			"ignores any window parameter a caller sends: a balance is a running total, not an aggregate over " +
			"history. Amounts are exact decimal strings, per asset, never summed across assets.",
		Tags:     []string{tag},
		Security: secDashboardBearer,
	}, h.GetDashboardAssets)
}

// RegisterDashboardRoutesToApp wires the dashboard surface onto the /v1
// contract. See registerDashboardRoutesToApp for what it attaches.
func RegisterDashboardRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, dh *DashboardHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerDashboardRoutesToApp(group, api, auth, dh, routeOptions, v1OpSuffix)
}

// RegisterDashboardV2RoutesToApp wires the same dashboard surface onto the /v2
// contract: same paths, same handlers, same authz tuple and tenant chain,
// differing only in the operation IDs the contract publishes.
//
// The mirror is not optional decoration here. MarkV1OperationsDeprecated flags
// every /v1 operation on the published document, so a dashboard served on /v1
// alone would be born deprecated and every generated SDK would warn on the only
// way to call it. /v1 exists because that is the path the console binds to.
func RegisterDashboardV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, dh *DashboardHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerDashboardRoutesToApp(group, api, auth, dh, routeOptions, v2OpSuffix)
}

// registerDashboardRoutesToApp is the single description of the dashboard route
// surface, shared by every versioned contract that serves it. It attaches the
// Fiber auth chain — auth.Authorize("midaz","dashboard","get") + tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("dashboard") — as MIDDLEWARE
// ONLY on the VERSIONED GROUP, then registers the Huma terminals on the SAME
// group's Huma API.
//
// The route options passed in are the TRANSACTION ones. That pairing is
// load-bearing: the reads aggregate the transaction and balance tables, which
// live in the transaction database, so the transaction tenant middleware is
// what puts the right connection in context. The onboarding options would bind
// a different database and the reads would find no tables at all.
func registerDashboardRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, dh *DashboardHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const base = "/organizations/:organization_id/ledgers/:ledger_id/dashboard"

	parse := pkgHTTP.ParseUUIDPathParameters("dashboard")

	routeGet(group, base+"/metrics", protectedMidaz(auth, dashboardResource, "get", routeOptions, parse))
	routeGet(group, base+"/volume", protectedMidaz(auth, dashboardResource, "get", routeOptions, parse))
	routeGet(group, base+"/assets", protectedMidaz(auth, dashboardResource, "get", routeOptions, parse))

	RegisterDashboardRoutes(api, dh, opSuffix)
}
