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

// accountExceptionResource is the authz resource governing every account-exception operation.
// It is separate from the "accounts" and "account-blocks" resources so registering the narrow
// carve-outs an exception describes can be granted independently of the power to rewrite an
// account or to freeze it.
//
// ⚠️ Deployment note: this resource MUST be registered in the Access Manager. Until it is, all
// five routes fail closed with 403 — the correct direction, but the endpoints are unusable. The
// ("account-exceptions", <method>) pair is frozen: block/unblock proved the fail-closed posture
// in phase 1, and this resource extends it to the exception surface.
const accountExceptionResource = "account-exceptions"

// RegisterAccountExceptionRoutes registers the five account-exception operations on the shared
// Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix). The auth +
// tenant + ParseUUIDPathParameters chain is attached in registerAccountExceptionRoutesToApp
// (Fiber-level) BEFORE the Huma terminals, not here.
//
// opSuffix is appended to every operation ID so the same surface can be published on more than
// one version group of the one document without colliding — see v1OpSuffix.
func RegisterAccountExceptionRoutes(api huma.API, h *AccountExceptionHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions"
		idPath   = listPath + "/{exception_id}"
		tag      = "Account Exceptions"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createAccountException" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create an account exception",
		Tags:             []string{tag},
		Security:         secAccountExceptionBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccountException)
	attachTypedRequestBody[mmodel.CreateAccountExceptionInput](api, "createAccountException"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccountExceptions" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List account exceptions",
		Tags:        []string{tag},
		Security:    secAccountExceptionBearer,
	}, h.GetAllAccountExceptions)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExceptionByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account exception",
		Tags:        []string{tag},
		Security:    secAccountExceptionBearer,
	}, h.GetAccountExceptionByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccountException" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account exception",
		Tags:             []string{tag},
		Security:         secAccountExceptionBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccountException)
	attachTypedRequestBody[mmodel.UpdateAccountExceptionInput](api, "updateAccountException"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccountException" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete an account exception",
		Tags:          []string{tag},
		Security:      secAccountExceptionBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountExceptionByID)
}

// RegisterAccountExceptionRoutesToApp wires the account-exception surface onto the /v1 contract.
// See registerAccountExceptionRoutesToApp for what it attaches.
func RegisterAccountExceptionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountExceptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountExceptionRoutesToApp(group, api, auth, h, routeOptions, v1OpSuffix)
}

// RegisterAccountExceptionV2RoutesToApp wires the same account-exception surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only in the
// operation IDs the contract publishes. It is additive — /v1 keeps serving exceptions in
// parallel — and introduces no new policy surface.
func RegisterAccountExceptionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountExceptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountExceptionRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerAccountExceptionRoutesToApp is the single description of the account-exception surface,
// shared by every versioned contract that serves it. Auth is the "midaz" appName:
// auth.Authorize("midaz","account-exceptions",verb) + tenant +
// ParseUUIDPathParameters("account_exception"), attached as MIDDLEWARE ONLY (group-relative
// paths, no terminal) on the versioned group, then it registers the Huma terminals via
// RegisterAccountExceptionRoutes on the SAME group's Huma API.
//
// The registrar mirrors operation-route's: the create POST and its Huma terminal collapse into
// one Fiber entry because the guard's last POST path (listPath) equals the first terminal POST
// path; the two GET rows stay split because the guard's last GET is idPath while the first GET
// terminal is listPath. Registering a wholly new path set, it only GAINS golden rows.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// v1OpSuffix.
func registerAccountExceptionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountExceptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/exceptions"
		idPath   = listPath + "/:exception_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account_exception")

	routePost(group, listPath, protectedMidaz(auth, accountExceptionResource, "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, accountExceptionResource, "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, accountExceptionResource, "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, accountExceptionResource, "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, accountExceptionResource, "delete", routeOptions, parse))

	RegisterAccountExceptionRoutes(api, h, opSuffix)
}
