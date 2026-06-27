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
//
// eh and auditHandler are non-nil only in envelope encryption mode
// (KMS_VENDOR=hashicorp-vault); when nil the encryption/audit routes are not
// mounted, matching the legacy-mode posture where no KMS provisioning surface exists.
func RegisterCRMRoutesToApp(f fiber.Router, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions) {
	// Holders
	f.Post("/v1/organizations/:organization_id/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "post"), routeOptions, http.ParseUUIDPathParameters("holder"), http.WithBody(new(mmodel.CreateHolderInput), hh.CreateHolder))...)
	f.Get("/v1/organizations/:organization_id/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.GetHolderByID)...)

	if hah != nil {
		f.Get("/v1/organizations/:organization_id/holders/:id/accounts", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hah.GetAccountsByHolder)...)
	}

	f.Patch("/v1/organizations/:organization_id/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "patch"), routeOptions, http.ParseUUIDPathParameters("holder"), http.WithBody(new(mmodel.UpdateHolderInput), hh.UpdateHolder))...)
	f.Delete("/v1/organizations/:organization_id/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "delete"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.DeleteHolderByID)...)
	f.Get("/v1/organizations/:organization_id/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.GetAllHolders)...)

	// Instruments
	f.Get("/v1/organizations/:organization_id/instruments", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "get"), routeOptions, http.ParseUUIDPathParameters("instruments"), ah.GetAllInstruments)...)
	f.Post("/v1/organizations/:organization_id/holders/:holder_id/instruments", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "post"), routeOptions, http.ParseUUIDPathParameters("instruments"), http.WithBody(new(mmodel.CreateInstrumentInput), ah.CreateInstrument))...)
	f.Get("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "get"), routeOptions, http.ParseUUIDPathParameters("instruments"), ah.GetInstrumentByID)...)
	f.Patch("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "patch"), routeOptions, http.ParseUUIDPathParameters("instruments"), http.WithBody(new(mmodel.UpdateInstrumentInput), ah.UpdateInstrument))...)
	f.Delete("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "delete"), routeOptions, http.ParseUUIDPathParameters("instruments"), ah.DeleteInstrumentByID)...)
	f.Delete("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "instruments", "delete"), routeOptions, http.ParseUUIDPathParameters("related-parties"), ah.DeleteRelatedParty)...)

	// Encryption provisioning + protection audit (envelope mode only). In legacy
	// mode eh and auditHandler are nil, so these routes stay unregistered.
	if eh != nil {
		f.Post("/v1/organizations/:organization_id/encryption/provision", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "encryption", "post"), routeOptions, http.ParseUUIDPathParameters("organization"), http.WithBody(new(mmodel.ProvisionEncryptionInput), eh.Provision))...)
		f.Get("/v1/organizations/:organization_id/encryption/status", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "encryption", "get"), routeOptions, http.ParseUUIDPathParameters("organization"), eh.GetProvisioningStatus)...)
	}

	if auditHandler != nil {
		f.Get("/v1/organizations/:organization_id/protection/audit", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "protection", "get"), routeOptions, http.ParseUUIDPathParameters("organization"), auditHandler.GetAuditEvents)...)
	}
}

// CreateCRMRouteRegistrar returns a registrar that mounts the CRM routes on the
// unified ledger server. The routeOptions carries the CRM-scoped tenant
// middleware (built in the ledger composition root) so it applies ONLY to CRM
// routes.
func CreateCRMRouteRegistrar(auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(router fiber.Router) {
		RegisterCRMRoutesToApp(router, auth, hh, ah, hah, eh, auditHandler, routeOptions)
	}
}
