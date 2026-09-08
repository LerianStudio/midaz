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

// RegisterPortfolioRoutes registers the six portfolio operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber
// group). The auth + tenant + ParseUUIDPathParameters chain is attached by
// registerPortfolioRoutesToApp (Fiber-level), NOT here.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterPortfolioRoutes(api huma.API, h *PortfolioHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Portfolios"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createPortfolio" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearer,
		SkipValidateBody: true, // body validated imperatively (DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreatePortfolio)
	attachTypedRequestBody[mmodel.CreatePortfolioInput](api, "createPortfolio"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listPortfolios" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all portfolios",
		Tags:        []string{tag},
		Security:    secPortfolioBearer,
	}, h.ListPortfolios)

	huma.Register(api, huma.Operation{
		OperationID: "getPortfolioByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific portfolio",
		Tags:        []string{tag},
		Security:    secPortfolioBearer,
	}, h.GetPortfolioByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePortfolio" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdatePortfolio)
	attachTypedRequestBody[mmodel.UpdatePortfolioInput](api, "updatePortfolio"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePortfolio" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a portfolio",
		Tags:          []string{tag},
		Security:      secPortfolioBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeletePortfolioByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countPortfolios" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total portfolios",
		Tags:          []string{tag},
		Security:      secPortfolioBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountPortfolios)
}

// RegisterPortfolioRoutesToApp wires the portfolio surface onto the /v1
// contract. See registerPortfolioRoutesToApp for what it attaches.
func RegisterPortfolioRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerPortfolioRoutesToApp(group, api, auth, ph, routeOptions, v1OpSuffix)
}

// RegisterPortfolioV2RoutesToApp wires the same portfolio surface onto the /v2 contract:
// same paths, same handlers, same authz tuples and tenant chain, differing only in the
// operation IDs the contract publishes. It is additive — /v1 keeps serving portfolios in
// parallel — and introduces no new policy surface.
func RegisterPortfolioV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerPortfolioRoutesToApp(group, api, auth, ph, routeOptions, v2OpSuffix)
}

// registerPortfolioRoutesToApp is the single description of the portfolio route surface,
// shared by every versioned contract that serves it, mirroring RegisterAssetRoutesToApp.
// For each of the six ops it attaches the Fiber auth chain —
// auth.Authorize("midaz","portfolios",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("portfolio") — as MIDDLEWARE ONLY (no terminal handler) on the
// VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterPortfolioRoutes on the SAME group's Huma API. The (resource, verb) authz tuples
// and tenant resolution therefore apply on whichever version group it is mounted on — no
// portfolio route becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerPortfolioRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/portfolios"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("portfolio")

	routePost(group, listPath, protectedMidaz(auth, "portfolios", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "portfolios", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "portfolios", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "portfolios", "head", routeOptions, parse))

	RegisterPortfolioRoutes(api, ph, opSuffix)
}
