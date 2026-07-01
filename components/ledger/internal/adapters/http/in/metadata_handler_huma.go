// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the metadata-index (settings) resource,
// following the asset exemplar (asset_handler_huma.go). Metadata differs from asset
// in two ways: it lives under /v1/settings (no org/ledger path, no UUID path params,
// so NO ParseUUIDPathParameters middleware), and its path params are plain strings
// (entity_name enum, index_key). The proven conventions still hold:
//
//  1. Body ops carry RawBody []byte + SkipValidateBody so the imperative
//     http.DecodeAndValidate (the SAME pipeline the Fiber WithBody decorator runs)
//     stays the sole body validator — never a native Huma 422.
//  2. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     that the shared core feeds to http.ValidateParameters, byte-identical to the
//     Fiber c.Queries() path.
//  3. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json,
//     field/status/code-identical to the Fiber http.WithError path).
//  4. Auth stays a Fiber middleware chain (auth.Authorize("midaz","settings",verb) +
//     tenant PostAuthMiddlewares) attached in routes.go/unified-server.go BEFORE the
//     Huma registration — NOT a Huma Security scheme. The per-op Security metadata
//     below is SPEC-ONLY (for the generated OAS/SDK).
//
// The transport-agnostic cores (createMetadataIndex / getAllMetadataIndexes /
// deleteMetadataIndex) live in metadata.go and are shared with the Fiber wrappers.

// secMetadataBearerOrAPIKey advertises that each metadata operation accepts EITHER a
// JWT bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime
// auth is the Fiber guard chain (auth.Authorize("midaz","settings",verb)).
var secMetadataBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /settings/metadata-indexes/entities/{entity_name} -------------------

// CreateMetadataIndexInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header); entity_name is a plain string
// path param (no UUID, so no format tag).
type CreateMetadataIndexInputHuma struct {
	EntityName string `path:"entity_name" doc:"Entity name (organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)"`
	RawBody    []byte `contentType:"application/json"`
}

// CreateMetadataIndexOutputHuma pins 201 (matching http.Created).
type CreateMetadataIndexOutputHuma struct {
	Status int
	Body   *mmodel.MetadataIndex
}

// CreateMetadataIndexHuma decodes+validates the raw body imperatively then delegates
// to the shared createMetadataIndex core. It passes an empty query map: the POST
// route has no meaningful query params (the Fiber path validated c.Queries(), which
// is empty here), so ValidateParameters over an empty map is a no-op that preserves
// the canonical flow.
func (handler *MetadataIndexHandler) CreateMetadataIndexHuma(ctx context.Context, in *CreateMetadataIndexInputHuma) (*CreateMetadataIndexOutputHuma, error) {
	payload := new(mmodel.CreateMetadataIndexInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	index, err := handler.createMetadataIndex(ctx, in.EntityName, map[string]string{}, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateMetadataIndexOutputHuma{Status: http.StatusCreated, Body: index}, nil
}

// --- GET /settings/metadata-indexes (list) ------------------------------------

// ListMetadataIndexesInputHuma advertises the entity_name query filter in the spec
// (doc-only, no validation tags) and captures the raw query via Resolve for the
// shared core's http.ValidateParameters binder.
type ListMetadataIndexesInputHuma struct {
	EntityName string `query:"entity_name" doc:"Optional entity name filter"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag field above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListMetadataIndexesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListMetadataIndexesInputHuma) queries() map[string]string {
	out := make(map[string]string, len(in.rawQuery))
	for k, vs := range in.rawQuery {
		if len(vs) == 0 {
			out[k] = ""
			continue
		}

		out[k] = vs[len(vs)-1]
	}

	return out
}

// ListMetadataIndexesOutputHuma carries the flat index slice verbatim (matching the
// Fiber http.OK body — a JSON array, not a pagination envelope).
type ListMetadataIndexesOutputHuma struct {
	Status int
	Body   []*mmodel.MetadataIndex
}

// ListMetadataIndexesHuma delegates to the shared getAllMetadataIndexes core.
func (handler *MetadataIndexHandler) ListMetadataIndexesHuma(ctx context.Context, in *ListMetadataIndexesInputHuma) (*ListMetadataIndexesOutputHuma, error) {
	indexes, err := handler.getAllMetadataIndexes(ctx, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListMetadataIndexesOutputHuma{Status: http.StatusOK, Body: indexes}, nil
}

// --- DELETE /settings/metadata-indexes/entities/{entity_name}/key/{index_key} --

// DeleteMetadataIndexInputHuma is the delete request envelope. Both path params are
// plain strings (no UUID, no format tag).
type DeleteMetadataIndexInputHuma struct {
	EntityName string `path:"entity_name" doc:"Entity name (organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)"`
	IndexKey   string `path:"index_key" doc:"Index key (metadata key, e.g. 'tier')"`
}

// DeleteMetadataIndexOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteMetadataIndexOutputHuma struct{}

// DeleteMetadataIndexHuma delegates to the shared deleteMetadataIndex core; returns
// a bodiless 204 on success.
func (handler *MetadataIndexHandler) DeleteMetadataIndexHuma(ctx context.Context, in *DeleteMetadataIndexInputHuma) (*DeleteMetadataIndexOutputHuma, error) {
	if err := handler.deleteMetadataIndex(ctx, in.EntityName, in.IndexKey); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteMetadataIndexOutputHuma{}, nil
}

// RegisterMetadataIndexRoutes registers the three migrated metadata-index operations
// on the shared Huma API. It is the per-file seam the RegisterMetadataRoutesToApp
// wiring calls; the auth + tenant middleware chain for these routes is attached at
// the Fiber level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends /v1. The /v1 prefix
// rides the OpenAPI `servers` entry (openapi.New Config), keeping op paths relative.
func RegisterMetadataIndexRoutes(api huma.API, h *MetadataIndexHandler) {
	const (
		listPath   = "/settings/metadata-indexes"
		entityPath = listPath + "/entities/{entity_name}"
		keyPath    = entityPath + "/key/{index_key}"
		tag        = "Metadata Indexes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createMetadataIndex",
		Method:      http.MethodPost,
		Path:        entityPath,
		Summary:     "Create Metadata Index",
		Tags:        []string{tag},
		Security:    secMetadataBearerOrAPIKey,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateMetadataIndexHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllMetadataIndexes",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Metadata Indexes",
		Tags:        []string{tag},
		Security:    secMetadataBearerOrAPIKey,
	}, h.ListMetadataIndexesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteMetadataIndex",
		Method:      http.MethodDelete,
		Path:        keyPath,
		Summary:     "Delete Metadata Index",
		Tags:        []string{tag},
		Security:    secMetadataBearerOrAPIKey,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteMetadataIndexHuma)
}
