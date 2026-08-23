// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the Huma terminals for the metadata-index (settings) resource,
// following the asset exemplar (asset_handler.go). Metadata differs from asset
// in two ways: it lives under /v1/settings (no org/ledger path, no UUID path params,
// so NO ParseUUIDPathParameters middleware), and its path params are plain strings
// (entity_name enum, index_key). The shared conventions:
//
//  1. Body ops carry RawBody []byte + SkipValidateBody so the imperative
//     http.DecodeAndValidate stays the sole body validator — never a native Huma 422.
//  2. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     that the shared core feeds to http.ValidateParameters.
//  3. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json), which
//     fixes each canonical Midaz error at one code and one HTTP status.
//  4. Auth is a Fiber middleware chain (auth.Authorize("midaz","settings",verb) +
//     tenant PostAuthMiddlewares) attached in routes.go/unified-server.go BEFORE the
//     Huma registration — NOT a Huma Security scheme. The per-op Security metadata
//     below is SPEC-ONLY (for the generated OAS/SDK).
//
// The transport-agnostic cores (createMetadataIndex / getAllMetadataIndexes /
// deleteMetadataIndex) live in metadata.go.

// secMetadataBearer advertises that each metadata operation accepts a JWT bearer
// token. SPEC metadata only; runtime auth is the Fiber guard chain
// (auth.Authorize("midaz","settings",verb)).
var secMetadataBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /settings/metadata-indexes/entities/{entity_name} -------------------

// CreateMetadataIndexRequest is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header); entity_name is a plain string
// path param (no UUID, so no format tag).
type CreateMetadataIndexRequest struct {
	EntityName string `path:"entity_name" doc:"Entity name (organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)"`
	RawBody    []byte `contentType:"application/json"`
}

// CreateMetadataIndexResponse pins 201 (matching http.Created).
type CreateMetadataIndexResponse struct {
	Status int
	Body   *mmodel.MetadataIndex
}

// CreateMetadataIndex decodes+validates the raw body imperatively then delegates
// to the shared createMetadataIndex core. It passes an empty query map: the POST
// route carries no query params, so the core's ValidateParameters call is a no-op
// that keeps the canonical flow identical across the three operations.
func (handler *MetadataIndexHandler) CreateMetadataIndex(ctx context.Context, in *CreateMetadataIndexRequest) (*CreateMetadataIndexResponse, error) {
	payload := new(mmodel.CreateMetadataIndexInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	index, err := handler.createMetadataIndex(ctx, in.EntityName, map[string]string{}, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateMetadataIndexResponse{Status: http.StatusCreated, Body: index}, nil
}

// --- GET /settings/metadata-indexes (list) ------------------------------------

// ListMetadataIndexesRequest advertises the entity_name query filter in the spec
// (doc-only, no validation tags) and captures the raw query via Resolve for the
// shared core's http.ValidateParameters binder.
type ListMetadataIndexesRequest struct {
	EntityName string `query:"entity_name" doc:"Optional entity name filter"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag field above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListMetadataIndexesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListMetadataIndexesRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListMetadataIndexesResponse carries the flat index slice verbatim (matching the
// Fiber http.OK body — a JSON array, not a pagination envelope).
type ListMetadataIndexesResponse struct {
	Status int
	Body   []*mmodel.MetadataIndex
}

// ListMetadataIndexes delegates to the shared getAllMetadataIndexes core.
func (handler *MetadataIndexHandler) ListMetadataIndexes(ctx context.Context, in *ListMetadataIndexesRequest) (*ListMetadataIndexesResponse, error) {
	indexes, err := handler.getAllMetadataIndexes(ctx, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListMetadataIndexesResponse{Status: http.StatusOK, Body: indexes}, nil
}

// --- DELETE /settings/metadata-indexes/entities/{entity_name}/key/{index_key} --

// DeleteMetadataIndexRequest is the delete request envelope. Both path params are
// plain strings (no UUID, no format tag).
type DeleteMetadataIndexRequest struct {
	EntityName string `path:"entity_name" doc:"Entity name (organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)"`
	IndexKey   string `path:"index_key" doc:"Index key (metadata key, e.g. 'tier')"`
}

// DeleteMetadataIndexResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204.
type DeleteMetadataIndexResponse struct{}

// DeleteMetadataIndex delegates to the shared deleteMetadataIndex core; returns
// a bodiless 204 on success.
func (handler *MetadataIndexHandler) DeleteMetadataIndex(ctx context.Context, in *DeleteMetadataIndexRequest) (*DeleteMetadataIndexResponse, error) {
	if err := handler.deleteMetadataIndex(ctx, in.EntityName, in.IndexKey); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteMetadataIndexResponse{}, nil
}

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
