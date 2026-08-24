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

// RegisterLedgerRoutes registers the eight ledger operations on the shared Huma API.
// It is the per-file seam registerLedgerRoutesToApp calls; the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached at the Fiber
// level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterLedgerRoutes(api huma.API, h *LedgerHandler, opSuffix string) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers"
		idPath       = listPath + "/{ledger_id}"
		countPath    = listPath + "/metrics/count"
		settingsPath = idPath + "/settings"
		tag          = "Ledgers"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createLedger" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateLedger)
	attachTypedRequestBody[mmodel.CreateLedgerInput](api, "createLedger"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listLedgers" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all ledgers",
		Tags:        []string{tag},
		Security:    secLedgerBearer,
	}, h.ListLedgers)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific ledger",
		Tags:        []string{tag},
		Security:    secLedgerBearer,
	}, h.GetLedgerByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedger" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an existing ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearer,
		SkipValidateBody: true, // body validated imperatively — see createLedger.
	}, h.UpdateLedger)
	attachTypedRequestBody[mmodel.UpdateLedgerInput](api, "updateLedger"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteLedger" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a ledger",
		Tags:          []string{tag},
		Security:      secLedgerBearer,
		DefaultStatus: http.StatusNoContent, // Out has no Body field => bodiless 204.
	}, h.DeleteLedgerByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countLedgers" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total ledgers",
		Tags:          []string{tag},
		Security:      secLedgerBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountLedgers)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerSettings" + opSuffix,
		Method:      http.MethodGet,
		Path:        settingsPath,
		Summary:     "Get ledger settings",
		Tags:        []string{tag},
		Security:    secLedgerBearer,
	}, h.GetLedgerSettings)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedgerSettings" + opSuffix,
		Method:           http.MethodPatch,
		Path:             settingsPath,
		Summary:          "Update ledger settings",
		Tags:             []string{tag},
		Security:         secLedgerBearer,
		SkipValidateBody: true, // free-form map; allowlist enforced imperatively in the core.
	}, h.UpdateLedgerSettings)
	// updateLedgerSettings decodes into a free-form map[string]any (allowlist enforced
	// in the core), so the published schema is a structured object, not a $ref component.
	attachTypedRequestBody[map[string]any](api, "updateLedgerSettings"+opSuffix)
}

// RegisterLedgerRoutesToApp wires the ledger surface onto the /v1
// contract. See registerLedgerRoutesToApp for what it attaches.
func RegisterLedgerRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerLedgerRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterLedgerV2RoutesToApp wires the same ledger surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving ledgers in parallel — and
// introduces no new policy surface.
func RegisterLedgerV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerLedgerRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerLedgerRoutesToApp is the single description of the ledger route surface, shared by
// every versioned contract that serves it, mirroring RegisterAssetRoutesToApp. For each of
// the eight ops it attaches the Fiber auth chain — protectedMidaz(auth,"ledgers",verb) (=
// auth.Authorize("midaz","ledgers",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("ledger") — as MIDDLEWARE ONLY (no terminal) on the VERSIONED
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via RegisterLedgerRoutes
// on the SAME group's Huma API. The ("ledgers", verb) authz tuples and tenant resolution
// therefore apply on whichever version group it is mounted on — no ledger route becomes
// public. Every one of the eight ops carries ParseUUIDPathParameters("ledger"). Body
// handling is owned by the Huma terminal, and the
// body limit was never an authz concern.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerLedgerRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath     = "/organizations/:organization_id/ledgers"
		idPath       = listPath + "/:ledger_id"
		countPath    = listPath + "/metrics/count"
		settingsPath = idPath + "/settings"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("ledger")

	routePost(group, listPath, protectedMidaz(auth, "ledgers", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "ledgers", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routeGet(group, settingsPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routePatch(group, settingsPath, protectedMidaz(auth, "ledgers", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "ledgers", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "ledgers", "head", routeOptions, parse))

	RegisterLedgerRoutes(api, h, opSuffix)
}
