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

// RegisterAccountTypeRoutes registers the five account-type operations on the
// shared Huma API. It is the per-file seam registerAccountTypeRoutesToApp calls; the auth +
// tenant + ParseUUIDPathParameters middleware chain for these routes is attached in
// registerAccountTypeRoutesToApp (Fiber-level) BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterAccountTypeRoutes(api huma.API, h *AccountTypeHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/account-types"
		idPath   = listPath + "/{id}"
		tag      = "Account Types"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createAccountType" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see asset header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccountType)
	attachTypedRequestBody[mmodel.CreateAccountTypeInput](api, "createAccountType"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccountTypes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all account types",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
	}, h.ListAccountTypes)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountTypeByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
	}, h.GetAccountTypeByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccountType" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account type",
		Tags:             []string{tag},
		Security:         secAccountTypeBearer,
		SkipValidateBody: true, // body validated imperatively — see createAccountType.
	}, h.UpdateAccountType)
	attachTypedRequestBody[mmodel.UpdateAccountTypeInput](api, "updateAccountType"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteAccountType" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteAccountTypeByID)
}

// RegisterAccountTypeRoutesToApp wires the account-type surface onto the
// /v1 contract. See registerAccountTypeRoutesToApp for what it attaches.
func RegisterAccountTypeRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountTypeRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterAccountTypeV2RoutesToApp wires the same account-type surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only
// in the operation IDs the contract publishes. It is additive — /v1 keeps serving
// account-types in parallel — and introduces no new policy surface.
func RegisterAccountTypeV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountTypeRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerAccountTypeRoutesToApp is the single description of the account-type route
// surface, shared by every versioned contract that serves it, mirroring
// RegisterAssetRoutesToApp. For each of the five ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"account-types",verb) (= auth.Authorize("midaz","account-types",
// verb) + tenant PostAuthMiddlewares) + ParseUUIDPathParameters("account_type") — as
// MIDDLEWARE ONLY (no terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths, then
// registers the Huma terminals via RegisterAccountTypeRoutes on the SAME group's Huma API.
// The ("midaz","account-types",verb) authz tuples and tenant resolution hold on whichever
// version group it is mounted on; no account-type route becomes public. The op order
// (post, patch, get-by-id, list, delete) matches routes.go.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerAccountTypeRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/account-types"
		idPath   = listPath + "/:id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account_type")

	routePost(group, listPath, protectedMidaz(auth, "account-types", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "account-types", "patch", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "account-types", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "account-types", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "account-types", "delete", routeOptions, parse))

	RegisterAccountTypeRoutes(api, h, opSuffix)
}
