// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterHolderAccountsRoutes registers the holder-scoped account listing on the
// given Huma API. It is a separate seam so the unified server can mount it
// conditionally, only when the ledger account-query backing is wired (the
// `if hah != nil` guard in routes_op_suffix.go). Auth is ("midaz","holders","get")
// + ParseUUIDPathParameters("holder"), attached BEFORE the Huma terminal.
//
// opSuffix is appended to the operation ID — see v2OpSuffix.
func RegisterHolderAccountsRoutes(api huma.API, h *HolderAccountsHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "listAccountsByHolder" + opSuffix,
		Method:      http.MethodGet,
		Path:        "/organizations/{organization_id}/holders/{id}/accounts",
		Summary:     "List Accounts by Holder",
		Tags:        []string{"Holders"},
		Security:    secHolderBearer,
	}, h.GetAccountsByHolder)
}

// RegisterHolderAccountsV2RoutesToApp wires the holder-scoped account listing onto the
// /v2 contract, which is the ONLY version group that serves it.
//
// h may be nil (no ledger account-query backing); when nil the route is neither
// auth-attached nor Huma-registered, so the path answers 404 rather than reaching a
// broken terminal. Holder and instrument carry no such guard because they are always
// wired; this surface is conditional on the ledger account query being available.
func RegisterHolderAccountsV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *HolderAccountsHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	if h == nil {
		return
	}

	registerHolderAccountsRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerHolderAccountsRoutesToApp is the single description of the holder-accounts
// route surface. It attaches auth.Authorize("midaz","holders","get") + the
// holder-accounts-scoped tenant PostAuthMiddlewares + ParseUUIDPathParameters("holder") as MIDDLEWARE ONLY on
// the versioned group, then registers the Huma terminal on the same group's Huma API.
func registerHolderAccountsRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *HolderAccountsHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const acctsPath = "/organizations/:organization_id/holders/:id/accounts"

	routeGet(group, acctsPath, protectedMidaz(auth, "holders", "get", routeOptions, pkgHTTP.ParseUUIDPathParameters("holder")))

	RegisterHolderAccountsRoutes(api, h, opSuffix)
}
