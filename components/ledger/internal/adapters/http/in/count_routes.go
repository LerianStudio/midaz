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

// RegisterCountTransactionRoutes registers the transaction-count HEAD op on the
// shared Huma API. It is the per-file seam the unified server calls;
// the auth (auth.Authorize("midaz","transactions","head")) + tenant +
// ParseUUIDPathParameters("transaction") chain for this route is attached in the
// unified server (Fiber level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (the group's PrefixModifier writes the version into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation ID one version group publishes from another's — see
// routeOpSuffixV1. The v1 group passes the empty suffix so its ID stays exactly what published
// SDKs bind to; the v2 group passes "V2" so its twin does not collide in the one document.
func RegisterCountTransactionRoutes(api huma.API, h *TransactionHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "countTransactionsByFilters" + opSuffix,
		Method:      http.MethodHead,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/metrics/count",
		Summary:     "Count Transactions by Filters",
		Tags:        []string{"Transactions"},
		Security:    secCountBearer,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set on
		// the Out struct).
		DefaultStatus: http.StatusNoContent,
	}, h.CountTransactionsByFilters)
}

// RegisterCountTransactionRoutesToApp wires the transaction-count HEAD op onto the /v1
// contract. See registerCountTransactionRoutesToApp for what it attaches.
func RegisterCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerCountTransactionRoutesToApp(group, api, auth, th, routeOptions, routeOpSuffixV1)
}

// RegisterCountTransactionV2RoutesToApp wires the same transaction-count HEAD op onto the /v2
// contract: same path, same handler, same authz tuple and tenant chain, differing only in the
// operation ID the contract publishes. It is additive — /v1 keeps serving the count in parallel
// — and introduces no new policy surface.
func RegisterCountTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerCountTransactionRoutesToApp(group, api, auth, th, routeOptions, routeOpSuffixV2)
}

// registerCountTransactionRoutesToApp is the single description of the transaction-count route
// surface, shared by every versioned contract that serves it. It attaches the Fiber auth chain
// — auth.Authorize("midaz","transactions","head") + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("transaction") — as MIDDLEWARE ONLY (group-relative path, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminal via RegisterCountTransactionRoutes on
// the SAME group's Huma API. This preserves the ("midaz","transactions","head") authz tuple and
// tenant resolution BYTE-FOR-BYTE on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation ID one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const countPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count"

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	routeHead(group, countPath, protectedMidaz(auth, "transactions", "head", routeOptions, parse))

	RegisterCountTransactionRoutes(api, th, opSuffix)
}
