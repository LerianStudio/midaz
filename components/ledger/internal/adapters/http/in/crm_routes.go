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

// ApplicationName is the authz namespace the CRM (holders/instruments) routes
// register under. CRM is folded into the ledger binary, so it authorizes under
// the host ledger's "midaz" namespace rather than a standalone plugin namespace.
// This value is the X1 RBAC contract: tenant-manager grants migrate from
// plugin-crm:* to midaz:{holders,instruments}:* at v4 release (X1, Fred-owned).
const ApplicationName = "midaz"

// CRM operation-ID suffixes. The ledger serves the CRM surface on both the /v1 and
// /v2 version groups of a single OpenAPI document. huma.OpenAPI.AddOperation scans the
// whole document and panics on a duplicate operation ID, so a v1 op and its v2 twin —
// same handler, same path shape under a different version prefix — MUST carry distinct
// operation IDs or the ledger panics at boot. The V2 suffix makes that disjunction a
// boot invariant; it secondarily keeps IDs unique across the ledger↔tracer hub-spec
// join. The v1 suffix is empty so the /v1 operation IDs — the ones published SDKs
// already bind to — stay exactly what they were.
const (
	crmOpSuffixV1 = ""
	crmOpSuffixV2 = "V2"
)

// RegisterCRMRoutesToApp wires the Huma-migrated CRM holder/instrument surface onto
// the /v1 contract. See registerCRMRoutesToApp for what it attaches.
func RegisterCRMRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions) {
	registerCRMRoutesToApp(group, api, auth, hh, ah, hah, eh, auditHandler, routeOptions, crmOpSuffixV1)
}

// RegisterCRMV2RoutesToApp wires the same CRM surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the
// operation IDs the contract publishes. It is additive — /v1 keeps serving CRM in
// parallel — and introduces no new policy surface.
func RegisterCRMV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions) {
	registerCRMRoutesToApp(group, api, auth, hh, ah, hah, eh, auditHandler, routeOptions, crmOpSuffixV2)
}

// registerCRMRoutesToApp is the single description of the CRM route surface, shared by
// every versioned contract that serves it. For each op it attaches the Fiber auth
// chain — auth.Authorize("midaz",resource,verb) + the CRM-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters — as MIDDLEWARE ONLY
// (no terminal handler, no body binder) on the VERSIONED GROUP with GROUP-RELATIVE
// paths, then registers the Huma terminals via RegisterHolderRoutes/
// RegisterInstrumentRoutes on the SAME group's Huma API. This preserves the pre-Huma
// (resource, verb) authz tuples and tenant resolution BYTE-FOR-BYTE — no CRM route
// becomes public — while the Huma terminal owns request/response shaping. The
// ParseUUIDPathParameters labels (holder/instruments/related-parties) are the
// span-attribute names the inline routes used; the middleware validates every UUID
// path param regardless of label.
//
// opSuffix distinguishes the operation IDs one contract publishes from another's — see
// crmOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
//
// hah may be nil (no ledger account-query backing); when nil the holder-accounts route
// is neither auth-attached nor Huma-registered, matching the pre-Huma `if hah != nil`
// guard.
//
// eh and auditHandler are non-nil only in envelope encryption mode
// (KMS_VENDOR=hashicorp-vault); when nil the encryption/audit routes stay unregistered,
// matching the legacy-mode posture where no KMS provisioning surface exists.
func registerCRMRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, eh *EncryptionHandler, auditHandler *AuditHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
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
	routePost(group, holdersPath, protectedMidaz(auth, "holders", "post", routeOptions, holderParse))
	routeGet(group, holderIDPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse))
	routePatch(group, holderIDPath, protectedMidaz(auth, "holders", "patch", routeOptions, holderParse))
	routeDelete(group, holderIDPath, protectedMidaz(auth, "holders", "delete", routeOptions, holderParse))
	routeGet(group, holdersPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse))

	RegisterHolderRoutes(api, hh, opSuffix)

	if hah != nil {
		routeGet(group, acctsPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse))
		RegisterHolderAccountsRoutes(api, hah, opSuffix)
	}

	// Instruments (auth resource "instruments" under midaz).
	routeGet(group, instrumentsPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse))
	routePost(group, holderInstruments, protectedMidaz(auth, "instruments", "post", routeOptions, instrumentParse))
	routeGet(group, instrumentIDPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse))
	routePatch(group, instrumentIDPath, protectedMidaz(auth, "instruments", "patch", routeOptions, instrumentParse))
	routeDelete(group, instrumentIDPath, protectedMidaz(auth, "instruments", "delete", routeOptions, instrumentParse))
	routeDelete(group, relatedPartyPath, protectedMidaz(auth, "instruments", "delete", routeOptions, http.ParseUUIDPathParameters("related-parties")))

	RegisterInstrumentRoutes(api, ah, opSuffix)

	// Encryption provisioning + protection audit (envelope mode only). In legacy mode
	// eh and auditHandler are nil, so these routes stay unregistered.
	if eh != nil {
		routePost(group, encProvisionPath, protectedMidaz(auth, "encryption", "post", routeOptions, orgParse))
		routeGet(group, encStatusPath, protectedMidaz(auth, "encryption", "get", routeOptions, orgParse))
		RegisterEncryptionRoutes(api, eh, opSuffix)
	}

	if auditHandler != nil {
		routeGet(group, auditPath, protectedMidaz(auth, "protection", "get", routeOptions, orgParse))
		RegisterAuditRoutes(api, auditHandler, opSuffix)
	}
}
