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

// RegisterAccountRoutes registers the eight account operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber
// group, so the humafiber adapter registers on that group and Fiber prepends the version
// prefix). The auth + tenant + ParseUUIDPathParameters chain is attached in
// registerAccountRoutesToApp (Fiber-level) BEFORE the Huma terminals, not here.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterAccountRoutes(api huma.API, h *AccountHandler, opSuffix string) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts"
		idPath       = listPath + "/{id}"
		aliasPath    = listPath + "/alias/{alias}"
		externalPath = listPath + "/external/{code}"
		countPath    = listPath + "/metrics/count"
		tag          = "Accounts"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createAccount" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new account",
		Tags:             []string{tag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccount)
	attachTypedRequestBody[mmodel.CreateAccountInput](api, "createAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccounts" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all accounts",
		Tags:        []string{tag},
		Security:    secAccountBearer,
	}, h.ListAccounts)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account",
		Tags:        []string{tag},
		Security:    secAccountBearer,
	}, h.GetAccountByID)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        aliasPath,
		Summary:     "Retrieve an account by alias",
		Tags:        []string{tag},
		Security:    secAccountBearer,
	}, h.GetAccountByAlias)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        externalPath,
		Summary:     "Retrieve an account by external code",
		Tags:        []string{tag},
		Security:    secAccountBearer,
	}, h.GetAccountExternalByCode)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccount" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account",
		Tags:             []string{tag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccount)
	attachTypedRequestBody[mmodel.UpdateAccountInput](api, "updateAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccount" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete an account",
		Tags:          []string{tag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countAccounts" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count accounts",
		Tags:          []string{tag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountAccounts)
}

// RegisterAccountRoutesToApp wires the account surface onto the /v1
// contract. See registerAccountRoutesToApp for what it attaches.
func RegisterAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountRoutesToApp(group, api, auth, h, routeOptions, v1OpSuffix)
}

// RegisterAccountV2RoutesToApp wires the same account surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving accounts in parallel — and
// introduces no new policy surface.
func RegisterAccountV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerAccountRoutesToApp is the single description of the account route surface, shared
// by every versioned contract that serves it, mirroring RegisterAssetRoutesToApp /
// RegisterPortfolioRoutesToApp. For each of the eight ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"accounts",verb) (= auth.Authorize("midaz","accounts",verb) + tenant
// PostAuthMiddlewares) + ParseUUIDPathParameters("account") — as MIDDLEWARE ONLY (no
// terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma
// terminals via RegisterAccountRoutes on the SAME group's Huma API. The (accounts, verb)
// authz tuples and tenant resolution therefore apply on whichever version group it is
// mounted on — no account route becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath     = "/organizations/:organization_id/ledgers/:ledger_id/accounts"
		idPath       = listPath + "/:id"
		aliasPath    = listPath + "/alias/:alias"
		externalPath = listPath + "/external/:code"
		countPath    = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account")

	routePost(group, listPath, protectedMidaz(auth, "accounts", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "accounts", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, aliasPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, externalPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "accounts", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "accounts", "head", routeOptions, parse))

	RegisterAccountRoutes(api, h, opSuffix)
}
