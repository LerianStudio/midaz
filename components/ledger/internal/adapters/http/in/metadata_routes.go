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

// RegisterMetadataIndexRoutes registers the three metadata-index operations
// on the shared Huma API. The auth + tenant middleware chain for these routes is
// attached at the Fiber level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix. The
// group's PrefixModifier writes the version prefix into each op's op.Path, not into a
// servers entry.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterMetadataIndexRoutes(api huma.API, h *MetadataIndexHandler, opSuffix string) {
	const (
		listPath   = "/settings/metadata-indexes"
		entityPath = listPath + "/entities/{entity_name}"
		keyPath    = entityPath + "/key/{index_key}"
		tag        = "Metadata Indexes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createMetadataIndex" + opSuffix,
		Method:      http.MethodPost,
		Path:        entityPath,
		Summary:     "Create Metadata Index",
		Tags:        []string{tag},
		Security:    secMetadataBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateMetadataIndex)
	attachTypedRequestBody[mmodel.CreateMetadataIndexInput](api, "createMetadataIndex"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getAllMetadataIndexes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Metadata Indexes",
		Tags:        []string{tag},
		Security:    secMetadataBearer,
	}, h.ListMetadataIndexes)

	huma.Register(api, huma.Operation{
		OperationID: "deleteMetadataIndex" + opSuffix,
		Method:      http.MethodDelete,
		Path:        keyPath,
		Summary:     "Delete Metadata Index",
		Tags:        []string{tag},
		Security:    secMetadataBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteMetadataIndex)
}

// RegisterMetadataIndexRoutesToApp wires the metadata-index surface onto the
// /v1 contract. See registerMetadataIndexRoutesToApp for what it attaches.
func RegisterMetadataIndexRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *MetadataIndexHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerMetadataIndexRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterMetadataIndexV2RoutesToApp wires the same metadata-index surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only
// in the operation IDs the contract publishes. It is additive — /v1 keeps serving
// metadata-indexes in parallel — and introduces no new policy surface.
func RegisterMetadataIndexV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *MetadataIndexHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerMetadataIndexRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerMetadataIndexRoutesToApp is the single description of the metadata-index route
// surface, shared by every versioned contract that serves it, mirroring
// RegisterAssetRoutesToApp. For each of the three ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"settings",verb) (= auth.Authorize("midaz","settings",verb) + tenant
// PostAuthMiddlewares) — as MIDDLEWARE ONLY (no terminal) on the VERSIONED GROUP with
// GROUP-RELATIVE paths, then registers the Huma terminals via RegisterMetadataIndexRoutes on
// the SAME group's Huma API. The authz tuples are ("settings", verb) — the resource is
// "settings", NOT "metadata-indexes" — and tenant resolution runs on whichever version
// group the surface is mounted on; no metadata-index route becomes public.
//
// No ParseUUIDPathParameters is attached: the path params — entity_name, index_key — are
// not UUIDs. The terminal auth/tenant middleware calls c.Next(), advancing into the Huma
// terminal.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerMetadataIndexRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *MetadataIndexHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath   = "/settings/metadata-indexes"
		entityPath = listPath + "/entities/:entity_name"
		keyPath    = entityPath + "/key/:index_key"
	)

	routePost(group, entityPath, protectedMidaz(auth, "settings", "post", routeOptions))
	routeGet(group, listPath, protectedMidaz(auth, "settings", "get", routeOptions))
	routeDelete(group, keyPath, protectedMidaz(auth, "settings", "delete", routeOptions))

	RegisterMetadataIndexRoutes(api, h, opSuffix)
}
