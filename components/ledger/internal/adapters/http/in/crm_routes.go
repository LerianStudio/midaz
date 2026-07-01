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

// ApplicationName is the authz namespace the CRM (holders/instruments) routes
// register under. CRM is folded into the ledger binary, so it authorizes under
// the host ledger's "midaz" namespace rather than a standalone plugin namespace.
// This value is the X1 RBAC contract: tenant-manager grants migrate from
// plugin-crm:* to midaz:{holders,instruments}:* at v4 release (X1, Fred-owned).
const ApplicationName = "midaz"

// RegisterCRMRoutesToApp wires the Huma-migrated CRM holder/instrument surface,
// mirroring RegisterAssetRoutesToApp. For each op it attaches the Fiber auth chain —
// auth.Authorize("midaz",resource,verb) + the CRM-scoped tenant PostAuthMiddlewares
// (routeOptions) + ParseUUIDPathParameters — as MIDDLEWARE ONLY (no terminal handler,
// no body binder) on the /v1 GROUP with GROUP-RELATIVE paths, then registers the Huma
// terminals via RegisterHolderRoutes/RegisterInstrumentRoutes on the SAME group's Huma
// API. This preserves the pre-Huma (resource, verb) authz tuples and tenant resolution
// BYTE-FOR-BYTE — no CRM route becomes public — while the Huma terminal owns
// request/response shaping. The ParseUUIDPathParameters labels (holder/instruments/
// related-parties) are the span-attribute names the inline routes used; the middleware
// validates every UUID path param regardless of label.
//
// hah may be nil (no ledger account-query backing); when nil the holder-accounts route
// is neither auth-attached nor Huma-registered, matching the pre-Huma `if hah != nil`
// guard.
//
// eh and auditHandler are non-nil only in envelope encryption mode
// (KMS_VENDOR=hashicorp-vault); when nil the encryption/audit routes stay unregistered,
// matching the legacy-mode posture where no KMS provisioning surface exists.
func RegisterCRMRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		holdersPath  = "/organizations/:organization_id/holders"
		holderIDPath = holdersPath + "/:id"
		acctsPath    = holderIDPath + "/accounts"

		instrumentsPath   = "/organizations/:organization_id/instruments"
		holderInstruments = holdersPath + "/:holder_id/instruments"
		instrumentIDPath  = holderInstruments + "/:instrument_id"
		relatedPartyPath  = instrumentIDPath + "/related-parties/:related_party_id"

		encProvisionPath = "/organizations/:organization_id/encryption/provision"
		encStatusPath    = "/organizations/:organization_id/encryption/status"
		auditPath        = "/organizations/:organization_id/protection/audit"
	)

	holderParse := http.ParseUUIDPathParameters("holder")
	instrumentParse := http.ParseUUIDPathParameters("instruments")
	orgParse := http.ParseUUIDPathParameters("organization")

	// Holders (auth resource "holders" under midaz).
	group.Post(holdersPath, protectedMidaz(auth, "holders", "post", routeOptions, holderParse)...)
	group.Get(holderIDPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse)...)
	group.Patch(holderIDPath, protectedMidaz(auth, "holders", "patch", routeOptions, holderParse)...)
	group.Delete(holderIDPath, protectedMidaz(auth, "holders", "delete", routeOptions, holderParse)...)
	group.Get(holdersPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse)...)

	RegisterHolderRoutes(api, hh)

	if hah != nil {
		group.Get(acctsPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse)...)
		RegisterHolderAccountsRoutes(api, hah)
	}

	// Instruments (auth resource "instruments" under midaz).
	group.Get(instrumentsPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse)...)
	group.Post(holderInstruments, protectedMidaz(auth, "instruments", "post", routeOptions, instrumentParse)...)
	group.Get(instrumentIDPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse)...)
	group.Patch(instrumentIDPath, protectedMidaz(auth, "instruments", "patch", routeOptions, instrumentParse)...)
	group.Delete(instrumentIDPath, protectedMidaz(auth, "instruments", "delete", routeOptions, instrumentParse)...)
	group.Delete(relatedPartyPath, protectedMidaz(auth, "instruments", "delete", routeOptions, http.ParseUUIDPathParameters("related-parties"))...)

	RegisterInstrumentRoutes(api, ah)

	// Encryption provisioning + protection audit (envelope mode only). In legacy mode
	// eh and auditHandler are nil, so these routes stay unregistered.
	if eh != nil {
		group.Post(encProvisionPath, protectedMidaz(auth, "encryption", "post", routeOptions, orgParse)...)
		group.Get(encStatusPath, protectedMidaz(auth, "encryption", "get", routeOptions, orgParse)...)
		RegisterEncryptionRoutes(api, eh)
	}

	if auditHandler != nil {
		group.Get(auditPath, protectedMidaz(auth, "protection", "get", routeOptions, orgParse)...)
		RegisterAuditRoutes(api, auditHandler)
	}
}
