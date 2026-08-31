// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterOperationRoutes registers the two operation read ops plus the PATCH
// (money-write leg) on the shared Huma API. It is the per-file seam the unified
// server calls; the auth (auth.Authorize("midaz","operations",verb)) + tenant +
// ParseUUIDPathParameters("operation") chain for these routes is attached in the unified
// server (Fiber level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the
// group's PrefixModifier writes the version into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// v1OpSuffix. The v1 group passes the empty suffix so its IDs stay exactly what published
// SDKs bind to; the v2 group passes "V2" so its twins do not collide in the one document.
func RegisterOperationRoutes(api huma.API, h *OperationHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations"
		idPath    = listPath + "/{operation_id}"
		patchPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id}"
		tag       = "Operations"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getAllOperationsByAccount" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Operations by account",
		Tags:        []string{tag},
		Security:    secOperationBearer,
	}, h.GetAllOperationsByAccount)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationByAccount" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get Operation",
		Tags:        []string{tag},
		Security:    secOperationBearer,
	}, h.GetOperationByAccount)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOperation" + opSuffix,
		Method:           http.MethodPatch,
		Path:             patchPath,
		Summary:          "Update an Operation",
		Tags:             []string{tag},
		Security:         secTransactionBearer, // BearerAuth (Bearer-only), matching the Fiber guard chain on the transaction-path PATCH.
		SkipValidateBody: true,                 // body validated imperatively (http.DecodeAndValidate) — plain decode, not merge-patch.
	}, h.UpdateOperation)
	attachTypedRequestBody[operation.UpdateOperationInput](api, "updateOperation"+opSuffix)
}

// RegisterOperationRoutesToApp wires the operation surface onto the /v1
// contract. See registerOperationRoutesToApp for what it attaches.
func RegisterOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOperationRoutesToApp(group, api, auth, oh, routeOptions, v1OpSuffix)
}

// RegisterOperationV2RoutesToApp wires the same operation surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation IDs
// the contract publishes. It is additive — /v1 keeps serving operations in parallel — and
// introduces no new policy surface.
func RegisterOperationV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOperationRoutesToApp(group, api, auth, oh, routeOptions, v2OpSuffix)
}

// registerOperationRoutesToApp is the single description of the operation route surface, shared
// by every versioned contract that serves it. It attaches the Fiber auth chain for the three
// ops — two READ (GET, on the account path) plus the PATCH (UpdateOperation, on the transaction
// path — a money-write LEG of the double-entry) —
// auth.Authorize("midaz","operations",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("operation") — as MIDDLEWARE ONLY (group-relative paths, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminals via RegisterOperationRoutes on the
// SAME group's Huma API. The ("midaz","operations",verb) authz tuples
// and tenant resolution BYTE-FOR-BYTE on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// v1OpSuffix. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations"
		idPath    = listPath + "/:operation_id"
		patchPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("operation")

	// Two READ ops — ("operations","get").
	routeGet(group, listPath, protectedMidaz(auth, "operations", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "operations", "get", routeOptions, parse))

	// PATCH (money-write leg) — ("operations","patch").
	routePatch(group, patchPath, protectedMidaz(auth, "operations", "patch", routeOptions, parse))

	RegisterOperationRoutes(api, oh, opSuffix)
}
