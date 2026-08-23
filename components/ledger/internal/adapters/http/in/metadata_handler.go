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
