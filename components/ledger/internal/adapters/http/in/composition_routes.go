// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

// RegisterCompositionRoutesToApp wires the Huma-migrated holder-account composition
// surface, mirroring RegisterAssetRoutesToApp / RegisterCRMRoutesToApp. It attaches the
// Fiber auth chain — auth.Authorize("midaz","accounts","post") + the cross-store
// composition tenant PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters
// ("holder") — as MIDDLEWARE ONLY (no terminal handler, no body binder) on the /v1
// GROUP with the GROUP-RELATIVE path, then registers the Huma terminal via
// RegisterCompositionRoutes on the SAME group's Huma API.
//
// The route authorizes under the host ledger's midazName namespace with the "accounts"
// resource: a tenant that can already open accounts can use composition with no new RBAC
// grant, and no plugin-* namespace is introduced. The :id path param is the holder;
// ParseUUIDPathParameters("holder") validates it (and org/ledger).
func RegisterCompositionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ch *CompositionHandler, routeOptions *http.ProtectedRouteOptions) {
	const path = "/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts"

	group.Post(path, protectedMidaz(auth, "accounts", "post", routeOptions, http.ParseUUIDPathParameters("holder"))...)

	RegisterCompositionRoutes(api, ch)
}
