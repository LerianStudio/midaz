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

// RegisterInstrumentRoutes registers the six instrument operations on the
// given Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","instruments",verb) + tenant + ParseUUIDPathParameters middleware chain is
// attached on the versioned Fiber group BEFORE the Huma terminal, not here. The
// related-party delete uses ParseUUIDPathParameters("related-parties"); all others use
// "instruments" (see registerInstrumentRoutesToApp). Paths are GROUP-RELATIVE (see
// asset_handler.go's RegisterAssetRoutes header for the rationale).
//
// opSuffix is appended to every operation ID — see crmOpSuffixV2.
func RegisterInstrumentRoutes(api huma.API, h *InstrumentHandler, opSuffix string) {
	const (
		listPath     = "/organizations/{organization_id}/instruments"
		holderScoped = "/organizations/{organization_id}/holders/{holder_id}/instruments"
		idPath       = holderScoped + "/{instrument_id}"
		rpPath       = idPath + "/related-parties/{related_party_id}"
		tag          = "Instruments"
	)

	huma.Register(api, huma.Operation{
		OperationID: "listInstruments" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List Instruments",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
	}, h.GetAllInstruments)

	huma.Register(api, huma.Operation{
		OperationID: "createInstrument" + opSuffix,
		Method:      http.MethodPost,
		Path:        holderScoped,
		Summary:     "Create an Instrument Account",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateInstrument)
	attachTypedRequestBody[mmodel.CreateInstrumentInput](api, "createInstrument"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getInstrumentByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve Instrument details",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
	}, h.GetInstrumentByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateInstrument" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an Instrument",
		Tags:             []string{tag},
		Security:         secInstrumentBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateInstrument)
	attachTypedRequestBody[mmodel.UpdateInstrumentInput](api, "updateInstrument"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteInstrument" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an Instrument",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteInstrumentByID)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteRelatedParty" + opSuffix,
		Method:        http.MethodDelete,
		Path:          rpPath,
		Summary:       "Delete a Related Party",
		Tags:          []string{tag},
		Security:      secInstrumentBearer,
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteRelatedParty)
}

// RegisterInstrumentV2RoutesToApp wires the instrument surface onto the /v2 contract,
// which is the ONLY version group that serves it. See registerInstrumentRoutesToApp for
// the auth chain and tenant options it attaches.
func RegisterInstrumentV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *InstrumentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerInstrumentRoutesToApp(group, api, auth, h, routeOptions, crmOpSuffixV2)
}

// registerInstrumentRoutesToApp is the single description of the instrument route
// surface, shared by every versioned contract that serves it, mirroring
// registerAccountRoutesToApp. For each of the six ops it attaches the Fiber auth chain —
// auth.Authorize("midaz","instruments",verb) + the CRM-scoped tenant PostAuthMiddlewares
// (routeOptions) + ParseUUIDPathParameters — as MIDDLEWARE ONLY (no terminal handler, no
// body binder) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma
// terminals via RegisterInstrumentRoutes on the SAME group's Huma API.
//
// The related-party delete carries the "related-parties" ParseUUIDPathParameters label
// while the rest carry "instruments". The labels are span-attribute names; the middleware
// validates every UUID path param regardless of label.
func registerInstrumentRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *InstrumentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		instrumentsPath   = "/organizations/:organization_id/instruments"
		holderInstruments = "/organizations/:organization_id/holders/:holder_id/instruments"
		instrumentIDPath  = holderInstruments + "/:instrument_id"
		relatedPartyPath  = instrumentIDPath + "/related-parties/:related_party_id"
	)

	instrumentParse := pkgHTTP.ParseUUIDPathParameters("instruments")

	routeGet(group, instrumentsPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse))
	routePost(group, holderInstruments, protectedMidaz(auth, "instruments", "post", routeOptions, instrumentParse))
	routeGet(group, instrumentIDPath, protectedMidaz(auth, "instruments", "get", routeOptions, instrumentParse))
	routePatch(group, instrumentIDPath, protectedMidaz(auth, "instruments", "patch", routeOptions, instrumentParse))
	routeDelete(group, instrumentIDPath, protectedMidaz(auth, "instruments", "delete", routeOptions, instrumentParse))
	routeDelete(group, relatedPartyPath, protectedMidaz(auth, "instruments", "delete", routeOptions, pkgHTTP.ParseUUIDPathParameters("related-parties")))

	RegisterInstrumentRoutes(api, h, opSuffix)
}
