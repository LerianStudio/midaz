// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the billing-package CRUD surface. It
// mirrors the fee-package sibling (fees_package_handler_huma.go) and the asset
// exemplar (asset_handler_huma.go); see the asset header for the full conventions.
// Billing-package-specific notes:
//
//  1. AUTH is appName "plugin-fees" (fees_routes.go feesApplicationName — the LEGACY
//     RBAC namespace preserved verbatim), resource "billing-packages". The Fiber
//     guard chain is Bearer-only, so the per-op Security
//     metadata here is Bearer-only too — SPEC metadata only; runtime auth stays the
//     Fiber guard chain (auth.Authorize("plugin-fees","billing-packages",verb) +
//     tenant + ParseUUIDPathParameters("billing-packages")) attached BEFORE the Huma
//     terminal.
//  2. ORG-SCOPED (no ledger in the path): the shells resolve organization_id (and the
//     package id where present) via the shared parseOrg / parsePathUUID helpers.
//  3. LANDMINE — WithBodyTracing: the Fiber create/patch routes decode via the fee-
//     package feehttp.WithBodyTracing (fee validator + a dedicated body-parsing span),
//     NOT the standard http.WithBody. The create/patch shells preserve BOTH by
//     decoding through decodeFeeBodyInSpan (defined in fees_handler_huma.go) — the fee
//     validator inside a replicated "middleware.body_parsing" span. They do NOT use
//     pkgHTTP.DecodeAndValidate — that would swap the validator instance. The MERGE-
//     PATCH semantics (Validate + ToMap + ErrNothingToUpdate on an empty set) live in
//     the shared updateBillingPackage core, so the Huma path is byte-identical.
//  4. LIST binds the query imperatively: the shell captures the raw query via Resolve
//     and rebuilds the map[string]string the getAllBillingPackages core parses
//     (ledgerId/type/limit/page), so the binder is byte-identical to the Fiber
//     c.Queries() path. The core, not Huma, owns ALL query validation — no native Huma
//     422/400 on the query.
//  5. Errors go through the shared pkgHTTP.HumaProblem.

// secBillingBearer advertises that each billing-package operation accepts a JWT bearer
// token (Bearer-only, matching the Fiber guard chain). SPEC metadata
// only; runtime auth is the Fiber guard chain.
var secBillingBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /billing-packages ---------------------------------------------------

// CreateBillingPackageInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header).
type CreateBillingPackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateBillingPackageOutputHuma pins 201 (matching the Fiber fiber.StatusCreated).
type CreateBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// CreateBillingPackageHuma decodes+validates the raw body imperatively (fee validator,
// inside the replicated body-parsing span) then delegates to the shared
// createBillingPackage core.
func (handler *BillingPackageHandler) CreateBillingPackageHuma(ctx context.Context, in *CreateBillingPackageInputHuma) (*CreateBillingPackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingPackage)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.createBillingPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateBillingPackageOutputHuma{Status: http.StatusCreated, Body: result}, nil
}

// --- GET /billing-packages (list) ---------------------------------------------

// ListBillingPackagesInputHuma advertises the list query params in the spec (doc-only,
// no validation tags — the core is the sole validator) and captures the raw query via
// Resolve for the imperative binder.
type ListBillingPackagesInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `query:"ledgerId" doc:"Filter by ledger ID (UUID) — omit to list all packages for the organization"`
	Type           string `query:"type" doc:"Filter by billing package type (volume, maintenance)"`
	Limit          string `query:"limit" doc:"Number of items per page (default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the getAllBillingPackages core.
func (in *ListBillingPackagesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListBillingPackagesOutputHuma carries the pagination envelope verbatim.
type ListBillingPackagesOutputHuma struct {
	Status int
	Body   model.Pagination
}

// GetAllBillingPackagesHuma binds the query imperatively then delegates to
// getAllBillingPackages.
func (handler *BillingPackageHandler) GetAllBillingPackagesHuma(ctx context.Context, in *ListBillingPackagesInputHuma) (*ListBillingPackagesOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBillingPackages(ctx, orgID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBillingPackagesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /billing-packages/{id} -----------------------------------------------

// GetBillingPackageInputHuma is the by-id request envelope. The id path param carries
// no format tag (ParseUUIDPathParameters is the sole validator).
type GetBillingPackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"BillingPackage ID (UUID)"`
}

// GetBillingPackageOutputHuma carries the billing package verbatim.
type GetBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// GetBillingPackageByIDHuma delegates to getBillingPackageByID.
func (handler *BillingPackageHandler) GetBillingPackageByIDHuma(ctx context.Context, in *GetBillingPackageInputHuma) (*GetBillingPackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.getBillingPackageByID(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetBillingPackageOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// --- PATCH /billing-packages/{id} ---------------------------------------------

// UpdateBillingPackageInputHuma is the update request envelope (RawBody, see Create).
type UpdateBillingPackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"BillingPackage ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateBillingPackageOutputHuma carries the updated package (200, matching the Fiber
// fiber.StatusOK).
type UpdateBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// UpdateBillingPackageHuma decodes+validates the raw body imperatively (fee validator,
// inside the replicated body-parsing span) then delegates to the shared
// updateBillingPackage core (which owns the merge-patch Validate/ToMap/empty guard).
func (handler *BillingPackageHandler) UpdateBillingPackageHuma(ctx context.Context, in *UpdateBillingPackageInputHuma) (*UpdateBillingPackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingPackageUpdate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.updateBillingPackage(ctx, orgID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateBillingPackageOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// --- DELETE /billing-packages/{id} --------------------------------------------

// DeleteBillingPackageOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber fiber.StatusNoContent path.
type DeleteBillingPackageOutputHuma struct{}

// DeleteBillingPackageHuma delegates to deleteBillingPackage; returns a bodiless 204
// on success.
func (handler *BillingPackageHandler) DeleteBillingPackageHuma(ctx context.Context, in *GetBillingPackageInputHuma) (*DeleteBillingPackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteBillingPackage(ctx, orgID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteBillingPackageOutputHuma{}, nil
}

// RegisterBillingPackageRoutes registers the five migrated billing-package operations
// on the shared Huma API. It is the per-file seam the unified server calls; the auth
// ("plugin-fees","billing-packages",verb) + tenant +
// ParseUUIDPathParameters("billing-packages") middleware chain is attached on the /v1
// group (Fiber-level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterBillingPackageRoutes(api huma.API, h *BillingPackageHandler) {
	const (
		listPath = "/organizations/{organization_id}/billing-packages"
		idPath   = listPath + "/{id}"
		tag      = "Billing Packages"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createBillingPackage",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a BillingPackage",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.CreateBillingPackageHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBillingPackages",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all billing packages",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetAllBillingPackagesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getBillingPackageByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get billing package",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetBillingPackageByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBillingPackage",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a billing package",
		Tags:             []string{tag},
		Security:         secBillingBearer,
		SkipValidateBody: true, // body validated imperatively — see createBillingPackage.
	}, h.UpdateBillingPackageHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBillingPackage",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a BillingPackage by ID",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBillingPackageHuma)
}
