// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// accountBlockExceptionResource is the authz resource name the guard chain
// authorizes under — its OWN name, not "accounts". Minting an exception bypasses
// the account block, so the permission is deliberately separable from the
// account CRUD permission and from the permission to transact.
const accountBlockExceptionResource = "account-block-exceptions"

// accountBlockExceptionPath is the group-relative Huma path of the surface. It
// sits under the account collection because the grant is scoped to an account
// alias, and it collides with no account route: the account surface registers no
// POST under /accounts/{id}.
const (
	accountBlockExceptionPath = accountListPath + "/block-exceptions"
	accountBlockExceptionTag  = "Account Block Exceptions"
)

// RegisterAccountBlockExceptionRoutes registers the create operation on the
// shared Huma API. Paths are GROUP-RELATIVE: the Huma API is bound to a
// versioned Fiber group, so the humafiber adapter registers on that group and
// Fiber prepends the version prefix. The auth + tenant + ParseUUIDPathParameters
// chain is attached in registerAccountBlockExceptionRoutesToApp (Fiber-level)
// BEFORE the Huma terminal, not here.
//
// opSuffix distinguishes the operation IDs one version group publishes from
// another's — see v2OpSuffix. The surface is /v2 only today, so it is called
// once, but the parameter is kept so mounting it on a further contract needs no
// change here.
func RegisterAccountBlockExceptionRoutes(api huma.API, h *AccountBlockExceptionHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "createAccountBlockExceptions" + opSuffix,
		Method:      http.MethodPost,
		Path:        accountBlockExceptionPath,
		Summary:     "Create single-use account block exceptions",
		Description: "Creates a batch of single-use exceptions that authorize a debit of an exact amount out of a blocked account. Each exception is returned with its identifier and its expiry, lives only in the cache, and dies on first use or when its TTL elapses.",
		Tags:        []string{accountBlockExceptionTag},
		Security:    secAccountBlockExceptionBearer,
		// Body validated imperatively (http.DecodeAndValidate), so the batch-size
		// and per-item rules answer with the canonical Midaz envelope naming the
		// offending item — see CreateAccountBlockExceptions.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccountBlockExceptions)
	attachTypedRequestBody[mmodel.CreateAccountBlockExceptionsInput](api, "createAccountBlockExceptions"+opSuffix)
}

// RegisterAccountBlockExceptionV2RoutesToApp wires the surface onto the /v2
// contract.
//
// It is /v2 ONLY, matching every other resource the ledger introduced after the
// version split (CRM, fees/billing, composition): the resource is new, so no v1
// SDK binds it, and the transaction surfaces that will consume the identifier
// are themselves /v2 contracts. Mounting it on /v1 would publish a deprecated
// twin of a route that has never shipped.
func RegisterAccountBlockExceptionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountBlockExceptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountBlockExceptionRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerAccountBlockExceptionRoutesToApp is the single description of the
// route surface. It attaches the Fiber guard chain —
// protectedMidaz(auth, "account-block-exceptions", "post") (=
// auth.Authorize("midaz","account-block-exceptions","post") + tenant
// PostAuthMiddlewares) + ParseUUIDPathParameters — as MIDDLEWARE ONLY (no
// terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers the
// Huma terminal on the SAME group's Huma API.
//
// The dedicated resource name is the whole point of the chain: a caller holding
// every accounts and transactions grant still gets 403 here without the
// account-block-exceptions grant.
func registerAccountBlockExceptionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountBlockExceptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const path = "/organizations/:organization_id/ledgers/:ledger_id/accounts/block-exceptions"

	parse := pkgHTTP.ParseUUIDPathParameters("account_block_exception")

	routePost(group, path, protectedMidaz(auth, accountBlockExceptionResource, "post", routeOptions, parse))

	RegisterAccountBlockExceptionRoutes(api, h, opSuffix)
}
