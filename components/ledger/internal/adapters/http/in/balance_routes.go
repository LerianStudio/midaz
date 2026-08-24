// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterBalanceRoutes registers the ten balance operations on the
// shared Huma API. The auth
// (auth.Authorize("midaz","balances",verb)) + tenant + ParseUUIDPathParameters
// ("balance") chain for these routes is attached in the unified server (Fiber
// level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the Huma API
// is bound to a versioned Fiber group; the group's PrefixModifier writes the version
// into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterBalanceRoutes(api huma.API, h *BalanceHandler, opSuffix string) {
	const (
		orgLedger      = "/organizations/{organization_id}/ledgers/{ledger_id}"
		balancesPath   = orgLedger + "/balances"
		balanceIDPath  = balancesPath + "/{balance_id}"
		balanceHistory = balanceIDPath + "/history"
		acctBalances   = orgLedger + "/accounts/{account_id}/balances"
		acctHistory    = acctBalances + "/history"
		aliasBalances  = orgLedger + "/accounts/alias/{alias}/balances"
		codeBalances   = orgLedger + "/accounts/external/{code}/balances"
		tag            = "Balances"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBalances" + opSuffix,
		Method:      http.MethodGet,
		Path:        balancesPath,
		Summary:     "Get all balances",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAllBalances)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        balanceIDPath,
		Summary:     "Get Balance by id",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalanceByID)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceAtTimestamp" + opSuffix,
		Method:      http.MethodGet,
		Path:        balanceHistory,
		Summary:     "Get Balance history at date",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalanceAtTimestamp)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBalancesByAccountID" + opSuffix,
		Method:      http.MethodGet,
		Path:        acctBalances,
		Summary:     "Get all balances by account id",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAllBalancesByAccountID)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountBalancesAtTimestamp" + opSuffix,
		Method:      http.MethodGet,
		Path:        acctHistory,
		Summary:     "Get Account Balances history at date",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAccountBalancesAtTimestamp)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        aliasBalances,
		Summary:     "Get Balances using Alias",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalancesByAlias)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        codeBalances,
		Summary:     "Get External balances using code",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalancesExternalByCode)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBalance" + opSuffix,
		Method:           http.MethodPatch,
		Path:             balanceIDPath,
		Summary:          "Update Balance",
		Tags:             []string{tag},
		Security:         secBalanceBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateBalance)
	attachTypedRequestBody[mmodel.UpdateBalance](api, "updateBalance"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:      "createAdditionalBalance" + opSuffix,
		Method:           http.MethodPost,
		Path:             acctBalances,
		Summary:          "Create Additional Balance",
		Tags:             []string{tag},
		Security:         secBalanceBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAdditionalBalance)
	attachTypedRequestBody[mmodel.CreateAdditionalBalance](api, "createAdditionalBalance"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBalance" + opSuffix,
		Method:      http.MethodDelete,
		Path:        balanceIDPath,
		Summary:     "Delete Balance by account",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBalanceByID)
}

// RegisterBalanceRoutesToApp wires the balance surface onto the /v1
// contract. See registerBalanceRoutesToApp for what it attaches.
func RegisterBalanceRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerBalanceRoutesToApp(group, api, auth, bh, routeOptions, v1OpSuffix)
}

// RegisterBalanceV2RoutesToApp wires the same balance surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving balances in parallel — and
// introduces no new policy surface.
func RegisterBalanceV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerBalanceRoutesToApp(group, api, auth, bh, routeOptions, v2OpSuffix)
}

// registerBalanceRoutesToApp is the single description of the balance route surface, shared by
// every versioned contract that serves it. It attaches the Fiber auth chain —
// auth.Authorize("midaz","balances",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("balance") — as MIDDLEWARE ONLY (group-relative paths, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminals via RegisterBalanceRoutes on the
// SAME group's Huma API. The alias/code path segments are NOT UUIDs;
// ParseUUIDPathParameters("balance") only validates org/ledger/balance_id/account_id, so those
// routes pass alias/code through raw. The ("midaz","balances",verb) authz tuples and tenant
// resolution therefore apply on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// v1OpSuffix. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerBalanceRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		orgLedger      = "/organizations/:organization_id/ledgers/:ledger_id"
		balancesPath   = orgLedger + "/balances"
		balanceIDPath  = balancesPath + "/:balance_id"
		balanceHistory = balanceIDPath + "/history"
		acctBalances   = orgLedger + "/accounts/:account_id/balances"
		acctHistory    = acctBalances + "/history"
		aliasBalances  = orgLedger + "/accounts/alias/:alias/balances"
		codeBalances   = orgLedger + "/accounts/external/:code/balances"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("balance")

	routeGet(group, balancesPath, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, balanceIDPath, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routePatch(group, balanceIDPath, protectedMidaz(auth, "balances", "patch", routeOptions, parse))
	routeDelete(group, balanceIDPath, protectedMidaz(auth, "balances", "delete", routeOptions, parse))
	routeGet(group, balanceHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, acctBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routePost(group, acctBalances, protectedMidaz(auth, "balances", "post", routeOptions, parse))
	routeGet(group, acctHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, aliasBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, codeBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))

	RegisterBalanceRoutes(api, bh, opSuffix)
}
