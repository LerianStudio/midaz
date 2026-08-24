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

// RegisterSegmentRoutes registers the six segment operations on the shared
// Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber group;
// the group's PrefixModifier writes the version into each op's op.Path, not into a servers
// entry). Mirrors RegisterAssetRoutes.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterSegmentRoutes(api huma.API, h *SegmentHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/segments"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Segments"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createSegment" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new segment",
		Tags:             []string{tag},
		Security:         secSegmentBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateSegment)
	attachTypedRequestBody[mmodel.CreateSegmentInput](api, "createSegment"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listSegments" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all segments",
		Tags:        []string{tag},
		Security:    secSegmentBearer,
	}, h.ListSegments)

	huma.Register(api, huma.Operation{
		OperationID: "getSegmentByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific segment",
		Tags:        []string{tag},
		Security:    secSegmentBearer,
	}, h.GetSegmentByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateSegment" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a segment",
		Tags:             []string{tag},
		Security:         secSegmentBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateSegment)
	attachTypedRequestBody[mmodel.UpdateSegmentInput](api, "updateSegment"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteSegment" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a segment",
		Tags:          []string{tag},
		Security:      secSegmentBearer,
		DefaultStatus: http.StatusNoContent, // Out struct with no Body field => bodiless 204.
	}, h.DeleteSegmentByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countSegments" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total segments",
		Tags:          []string{tag},
		Security:      secSegmentBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountSegments)
}

// RegisterSegmentRoutesToApp wires the segment surface onto the /v1
// contract. See registerSegmentRoutesToApp for what it attaches.
func RegisterSegmentRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *SegmentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerSegmentRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterSegmentV2RoutesToApp wires the same segment surface onto the /v2 contract:
// same paths, same handlers, same authz tuples and tenant chain, differing only in the
// operation IDs the contract publishes. It is additive — /v1 keeps serving segments in
// parallel — and introduces no new policy surface.
func RegisterSegmentV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *SegmentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerSegmentRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerSegmentRoutesToApp is the single description of the segment route surface,
// shared by every versioned contract that serves it, mirroring RegisterAssetRoutesToApp.
// For each of the six ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"segments",verb) (= auth.Authorize("midaz","segments",verb) +
// tenant PostAuthMiddlewares) + ParseUUIDPathParameters("segment") — as MIDDLEWARE ONLY
// (no terminal handler) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers
// the Huma terminals via RegisterSegmentRoutes on the SAME group's Huma API. The
// (segments, verb) authz tuples and tenant resolution therefore apply on whichever
// version group it is mounted on — no segment route becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerSegmentRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *SegmentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/segments"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("segment")

	routePost(group, listPath, protectedMidaz(auth, "segments", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "segments", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "segments", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "segments", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "segments", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "segments", "head", routeOptions, parse))

	RegisterSegmentRoutes(api, h, opSuffix)
}
