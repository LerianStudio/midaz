// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
)

// ApplicationName is the authz namespace the CRM (holders/instruments) routes
// register under. CRM is folded into the ledger binary, so it authorizes under
// the host ledger's "midaz" namespace rather than a standalone plugin namespace.
// This value is the X1 RBAC contract: tenant-manager grants migrate from
// plugin-crm:* to midaz:{holders,instruments}:* at v4 release (X1, Fred-owned).
const ApplicationName = "midaz"

// RegisterCRMRoutesToApp registers the CRM holder/instrument routes on an
// existing Fiber router. It is used by the unified ledger server, which passes a
// CRM-scoped routeOptions carrying a route-local tenant middleware so CRM's
// tenant Mongo never overwrites the onboarding/transaction tenant DB injected
// for ledger routes.
//
// The routes, paths, authz namespace (midaz via ApplicationName),
// UUID-path validation and body binding are identical to the standalone CRM
// surface they were folded from.
//
// hah may be nil (no ledger account-query backing); when nil the
// holder-accounts route is not mounted.
func RegisterCRMRoutesToApp(f fiber.Router, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, routeOptions *http.ProtectedRouteOptions) {
	// Holders
	f.Post("/v1/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "post"), routeOptions, http.WithBody(new(mmodel.CreateHolderInput), hh.CreateHolder))...)
	f.Get("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.GetHolderByID)...)
	if hah != nil {
		f.Get("/v1/holders/:id/accounts", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hah.GetAccountsByHolder)...)
	}
	f.Patch("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "patch"), routeOptions, http.ParseUUIDPathParameters("holder"), http.WithBody(new(mmodel.UpdateHolderInput), hh.UpdateHolder))...)
	f.Delete("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "delete"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.DeleteHolderByID)...)
	f.Get("/v1/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, hh.GetAllHolders)...)

	// Instruments
	f.Get("/v1/instruments", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "get"), routeOptions, ah.GetAllInstruments)...)
	f.Post("/v1/holders/:holder_id/instruments", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "post"), routeOptions, http.ParseUUIDPathParameters("instruments"), http.WithBody(new(mmodel.CreateInstrumentInput), ah.CreateInstrument))...)
	f.Get("/v1/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "get"), routeOptions, http.ParseUUIDPathParameters("instruments"), ah.GetInstrumentByID)...)
	f.Patch("/v1/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "patch"), routeOptions, http.ParseUUIDPathParameters("instruments"), http.WithBody(new(mmodel.UpdateInstrumentInput), ah.UpdateInstrument))...)
	f.Delete("/v1/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "delete"), routeOptions, http.ParseUUIDPathParameters("instruments"), ah.DeleteInstrumentByID)...)
	f.Delete("/v1/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "delete"), routeOptions, http.ParseUUIDPathParameters("related-parties"), ah.DeleteRelatedParty)...)
}

// CreateCRMRouteRegistrar returns a registrar that mounts the CRM routes on the
// unified ledger server. The routeOptions carries the CRM-scoped tenant
// middleware (built in the ledger composition root) so it applies ONLY to CRM
// routes.
func CreateCRMRouteRegistrar(auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(router fiber.Router) {
		RegisterCRMRoutesToApp(router, auth, hh, ah, hah, routeOptions)
	}
}
