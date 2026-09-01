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

	// The two dedicated block-state ops. They hang off the by-id path and are
	// governed by an authz resource of their own — see attachAccountRouteChain.
	accountBlockPath   = accountIDPath + "/block"
	accountUnblockPath = accountIDPath + "/unblock"
)

// accountBlockResource is the authz resource governing BOTH block-state directions.
// One permission covers block and unblock deliberately: an operator who can freeze an
// account must be able to release it, and splitting the two would strand accounts
// frozen by an operator whose grant was later narrowed. It is separate from the
// "accounts" resource so freezing an account can be granted without granting the power
// to rewrite it.
//
// ⚠️ Deployment note: this resource MUST be registered in the Access Manager. Until it
// is, both routes fail closed with 403 — which is the correct direction, but the
// endpoints are unusable.
const accountBlockResource = "account-blocks"

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

	huma.Register(api, huma.Operation{
		OperationID: "blockAccount" + opSuffix,
		Method:      http.MethodPost,
		Path:        accountBlockPath,
		Summary:     "Block an account",
		Description: accountBlockDescription,
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.BlockAccount)

	huma.Register(api, huma.Operation{
		OperationID: "unblockAccount" + opSuffix,
		Method:      http.MethodPost,
		Path:        accountUnblockPath,
		Summary:     "Unblock an account",
		Description: accountUnblockDescription,
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.UnblockAccount)
}

// accountBlockDescription / accountUnblockDescription document the two block-state ops
// for clients. They spell out the empty payload, the dedicated permission and the two
// business errors the ops can return that the generic account errors do not cover:
// 0052 (account not found) and 0074 (external accounts may never be manipulated).
const (
	accountBlockDescription = "Blocks an account, stopping it from taking part in new transactions. " +
		"The request takes no payload: record the reason for the block in the account's metadata via PATCH. " +
		"Answers with the updated account. Requires the dedicated `account-blocks` permission. " +
		"Returns 404 (code 0052) when the account does not exist and 422 (code 0074) for accounts of type `external`, " +
		"which anchor value crossing the ledger boundary and may never be blocked. " +
		"Repeating the call on an already-blocked account is safe."

	accountUnblockDescription = "Unblocks an account, letting it take part in transactions again. " +
		"The request takes no payload: record the reason for the release in the account's metadata via PATCH. " +
		"Answers with the updated account. Requires the dedicated `account-blocks` permission — the same permission " +
		"that governs blocking. Returns 404 (code 0052) when the account does not exist and 422 (code 0074) for " +
		"accounts of type `external`. Repeating the call on an already-unblocked account is safe."
)

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

	huma.Register(api, huma.Operation{
		OperationID: "blockAccount" + opSuffix,
		Method:      http.MethodPost,
		Path:        accountBlockPath,
		Summary:     "Block an account",
		Description: accountBlockDescription,
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.BlockAccountV2)

	huma.Register(api, huma.Operation{
		OperationID: "unblockAccount" + opSuffix,
		Method:      http.MethodPost,
		Path:        accountUnblockPath,
		Summary:     "Unblock an account",
		Description: accountUnblockDescription,
		Tags:        []string{accountTag},
		Security:    secAccountBearer,
	}, h.UnblockAccountV2)
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
		blockPath    = idPath + "/block"
		unblockPath  = idPath + "/unblock"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account")

	// The two block-state ops are the only account routes NOT guarded by the "accounts"
	// resource: they carry accountBlockResource so freezing an account is a permission of
	// its own, separate from the power to rewrite it. Both directions share the one tuple
	// — see accountBlockResource.
	//
	// ⚠️ ORDER IS LOAD-BEARING, and this is the reason these two sit ABOVE the create POST
	// rather than at the natural end of the list. Fiber keeps ONE route entry per method
	// and collapses a new registration into the previous one when it repeats that method's
	// most recent path; otherwise it opens a second entry. The create route relies on that
	// collapse — its guard chain and its Huma terminal share a single 4-handler entry — and
	// the terminals are all registered AFTER this function returns. Appending a POST here
	// therefore makes blockPath, not listPath, the last POST path seen, which splits the
	// pre-existing create entry in two.
	//
	// The split is behaviourally inert (the chain falls through to the terminal either way,
	// which is how every GET on this surface already works), but it rewrites a committed row
	// of testdata/route_table.golden for a route this change does not touch. Registering the
	// block ops first keeps listPath last, so the golden gains rows and changes none.
	routePost(group, blockPath, protectedMidaz(auth, accountBlockResource, "post", routeOptions, parse))
	routePost(group, unblockPath, protectedMidaz(auth, accountBlockResource, "post", routeOptions, parse))

	routePost(group, listPath, protectedMidaz(auth, "accounts", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "accounts", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, aliasPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, externalPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "accounts", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "accounts", "head", routeOptions, parse))
}
