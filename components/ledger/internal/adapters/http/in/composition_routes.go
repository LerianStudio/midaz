// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
)

// RegisterCompositionRoutesToApp mounts the holder-account composition surface on
// an existing Fiber router. It mirrors RegisterCRMRoutesToApp/RegisterFeesRoutesToApp:
// the route is protected by the ledger ProtectedRouteChain (auth -> route-scoped
// post-auth middleware via routeOptions -> UUID path validation -> body binding).
//
// The route authorizes under the host ledger's midazName namespace with the
// "accounts" resource: a tenant that can already open accounts can use composition
// with no new RBAC grant, and no plugin-* namespace is introduced. The :id path
// param is the holder; ParseUUIDPathParameters("holder") validates it.
func RegisterCompositionRoutesToApp(f fiber.Router, auth *middleware.AuthClient, ch *CompositionHandler, routeOptions *http.ProtectedRouteOptions) {
	f.Post("/v1/holders/:id/accounts", http.ProtectedRouteChain(
		auth.Authorize(midazName, "accounts", "post"),
		routeOptions,
		http.ParseUUIDPathParameters("holder"),
		http.WithBody(new(mmodel.CreateHolderAccountInput), ch.CreateHolderAccount),
	)...)
}

// CreateCompositionRouteRegistrar returns a registrar that mounts the composition
// route on the unified ledger server. The routeOptions carries the cross-store
// composition tenant middleware (built in the ledger composition root) so it
// applies ONLY to composition routes.
func CreateCompositionRouteRegistrar(auth *middleware.AuthClient, ch *CompositionHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(router fiber.Router) {
		RegisterCompositionRoutesToApp(router, auth, ch, routeOptions)
	}
}
