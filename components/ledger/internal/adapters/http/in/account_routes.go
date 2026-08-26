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

// accountPaths is the group-relative Huma path set of the account surface, shared by
// every versioned contract that serves it.
const (
	accountListPath     = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts"
	accountIDPath       = accountListPath + "/{id}"
	accountAliasPath    = accountListPath + "/alias/{alias}"
	accountExternalPath = accountListPath + "/external/{code}"
	accountCountPath    = accountListPath + "/metrics/count"
	accountTag          = "Accounts"
)

// RegisterAccountRoutes registers the eight /v1 account operations on the shared Huma
// API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber group, so
// the humafiber adapter registers on that group and Fiber prepends the version prefix).
// The auth + tenant + ParseUUIDPathParameters chain is attached in
// attachAccountRouteChain (Fiber-level) BEFORE the Huma terminals, not here.
//
// The account surface is NOT a straight v1/v2 mirror: the holder seam is /v2 only, so
// the five account-bearing ops bind the /v1 shells (AccountV1 bodies, HolderOffV1 on
// create) while DELETE and the HEAD count — which carry no account — bind the same
// methods on both contracts. RegisterAccountV2Routes is the /v2 half.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix.
func RegisterAccountRoutes(api huma.API, h *AccountHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID:      "createAccount" + opSuffix,
		Method:           http.MethodPost,
		Path:             accountListPath,
		Summary:          "Create a new account",
		Tags:             []string{accountTag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccount)
	attachTypedRequestBody[mmodel.CreateAccountInput](api, "createAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccounts" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountListPath,
		Summary:     "List all accounts",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.ListAccounts)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountIDPath,
		Summary:     "Retrieve a specific account",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountByID)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountAliasPath,
		Summary:     "Retrieve an account by alias",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountByAlias)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountExternalPath,
		Summary:     "Retrieve an account by external code",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountExternalByCode)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccount" + opSuffix,
		Method:           http.MethodPatch,
		Path:             accountIDPath,
		Summary:          "Update an account",
		Tags:             []string{accountTag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccount)
	attachTypedRequestBody[mmodel.UpdateAccountInput](api, "updateAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccount" + opSuffix,
		Method:        http.MethodDelete,
		Path:          accountIDPath,
		Summary:       "Delete an account",
		Tags:          []string{accountTag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countAccounts" + opSuffix,
		Method:        http.MethodHead,
		Path:          accountCountPath,
		Summary:       "Count accounts",
		Tags:          []string{accountTag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountAccounts)
}

// RegisterAccountV2Routes registers the eight /v2 account operations on the shared
// Huma API. It is the /v2 half of RegisterAccountRoutes: identical paths, authz tuples,
// tenant chain and summaries, differing only in the operation IDs and in the holder
// seam — the five account-bearing ops bind the /v2 shells (canonical mmodel.Account
// bodies, HolderOnV2 on create), while DELETE and the HEAD count carry no account and
// therefore bind the same handler methods as /v1.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v2OpSuffix.
func RegisterAccountV2Routes(api huma.API, h *AccountHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID:      "createAccount" + opSuffix,
		Method:           http.MethodPost,
		Path:             accountListPath,
		Summary:          "Create a new account",
		Tags:             []string{accountTag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccountV2)
	attachTypedRequestBody[mmodel.CreateAccountInput](api, "createAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccounts" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountListPath,
		Summary:     "List all accounts",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.ListAccountsV2)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountIDPath,
		Summary:     "Retrieve a specific account",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountByIDV2)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountAliasPath,
		Summary:     "Retrieve an account by alias",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountByAliasV2)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        accountExternalPath,
		Summary:     "Retrieve an account by external code",
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.GetAccountExternalByCodeV2)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccount" + opSuffix,
		Method:           http.MethodPatch,
		Path:             accountIDPath,
		Summary:          "Update an account",
		Tags:             []string{accountTag},
		Security:         secAccountBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccountV2)
	attachTypedRequestBody[mmodel.UpdateAccountInput](api, "updateAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccount" + opSuffix,
		Method:        http.MethodDelete,
		Path:          accountIDPath,
		Summary:       "Delete an account",
		Tags:          []string{accountTag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countAccounts" + opSuffix,
		Method:        http.MethodHead,
		Path:          accountCountPath,
		Summary:       "Count accounts",
		Tags:          []string{accountTag},
		Security:      secAccountBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountAccounts)
}

// RegisterAccountRoutesToApp wires the account surface onto the /v1 contract:
// attachAccountRouteChain for the Fiber guard chain, then the /v1 Huma terminals.
func RegisterAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	attachAccountRouteChain(group, auth, routeOptions)
	RegisterAccountRoutes(api, h, v1OpSuffix)
}

// RegisterAccountV2RoutesToApp wires the same account surface onto the /v2 contract: same
// paths, same authz tuples and tenant chain (attachAccountRouteChain is shared), differing
// in the operation IDs the contract publishes and in the holder seam the /v2 shells carry.
// It is additive — /v1 keeps serving accounts in parallel — and introduces no new policy
// surface.
func RegisterAccountV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	attachAccountRouteChain(group, auth, routeOptions)
	RegisterAccountV2Routes(api, h, v2OpSuffix)
}

// attachAccountRouteChain is the single description of the account route GUARD chain,
// shared by every versioned contract that serves the surface, mirroring
// RegisterAssetRoutesToApp / RegisterPortfolioRoutesToApp. For each of the eight ops it
// attaches protectedMidaz(auth,"accounts",verb) (= auth.Authorize("midaz","accounts",verb)
// + tenant PostAuthMiddlewares) + ParseUUIDPathParameters("account") as MIDDLEWARE ONLY
// (no terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths. The Huma terminals are
// registered separately by the caller on the SAME group's Huma API. The (accounts, verb)
// authz tuples and tenant resolution therefore apply on whichever version group it is
// mounted on — no account route becomes public.
//
// The chain is byte-identical across contracts, so a change to it reaches every version
// the surface is mounted on. Only the terminals differ (see RegisterAccountRoutes vs
// RegisterAccountV2Routes).
func attachAccountRouteChain(group fiber.Router, auth *middleware.AuthClient, routeOptions *pkgHTTP.ProtectedRouteOptions) {
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
}
