// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// RegisterCompositionV2RoutesToApp wires the holder-account composition surface onto the
// /v2 contract: it authorizes under the host ledger's "midaz" namespace with the "accounts"
// resource, and introduces no new policy surface. See registerCompositionRoutesToApp for
// what it attaches.
func RegisterCompositionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ch *CompositionHandler, routeOptions *http.ProtectedRouteOptions) {
	registerCompositionRoutesToApp(group, api, auth, ch, routeOptions, routeOpSuffixV2)
}

// registerCompositionRoutesToApp is the single description of the holder-account composition
// route surface, mirroring registerAssetRoutesToApp / registerCRMRoutesToApp. It attaches the
// Fiber auth chain —
// auth.Authorize("midaz","accounts","post") + the cross-store composition tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters("holder") — as MIDDLEWARE ONLY
// (no terminal handler, no body binder) on the VERSIONED GROUP with the GROUP-RELATIVE path,
// then registers the Huma terminal via RegisterCompositionRoutes on the SAME group's Huma API.
//
// The route authorizes under the host ledger's midazName namespace with the "accounts"
// resource: a tenant that can already open accounts can use composition with no new RBAC
// grant, and no plugin-* namespace is introduced. The :id path param is the holder;
// ParseUUIDPathParameters("holder") validates it (and org/ledger).
//
// opSuffix distinguishes the operation ID one version group publishes from another's — see
// routeOpSuffixV2. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerCompositionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ch *CompositionHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
	const path = "/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts"

	routePost(group, path, protectedMidaz(auth, "accounts", "post", routeOptions, http.ParseUUIDPathParameters("holder")))

	RegisterCompositionRoutes(api, ch, opSuffix)
}
