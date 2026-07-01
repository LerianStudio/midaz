// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the fee-package CRUD surface. It mirrors
// the asset exemplar (asset_handler_huma.go); see that file's header for the full
// conventions. Package-specific notes:
//
//  1. AUTH is appName "plugin-fees" (fees_routes.go feesApplicationName — the LEGACY
//     RBAC namespace preserved verbatim), resource "packages". The Fiber guard chain
//     is Bearer-only, so the per-op Security metadata here is
//     Bearer-only too — SPEC metadata only; runtime auth stays the Fiber guard chain
//     (auth.Authorize("plugin-fees","packages",verb) + tenant +
//     ParseUUIDPathParameters("packages")) attached BEFORE the Huma terminal.
//  2. ORG-SCOPED (no ledger in the path): the shells resolve organization_id (and the
//     package id where present) via the shared parseOrg / parsePathUUID helpers.
//  3. LANDMINE — WithBodyTracing: the Fiber create/patch routes decode via the fee-
//     package feehttp.WithBodyTracing (fee validator + a dedicated body-parsing span),
//     NOT the standard http.WithBody. The create/patch shells preserve BOTH by
//     decoding through decodeFeeBodyInSpan (defined in fees_handler_huma.go) — the fee
//     validator inside a replicated "middleware.body_parsing" span. They do NOT use
//     pkgHTTP.DecodeAndValidate — that would swap the validator instance.
//  4. LIST binds the query imperatively: the shell captures the raw query via Resolve
//     and rebuilds the map[string]string the getAllPackages core feeds to the fee-
//     package feehttp.ValidateParameters (NOT pkg/net/http's), so the binder is byte-
//     identical to the Fiber c.Queries() path. The core, not Huma, owns ALL query
//     validation — no native Huma 422/400 on the query.
//  5. Errors go through the shared pkgHTTP.HumaProblem.

// secPackageBearer advertises that each package operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secPackageBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /packages -----------------------------------------------------------

// CreatePackageInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see file header).
type CreatePackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreatePackageOutputHuma pins 201 (matching the Fiber fiber.StatusCreated).
type CreatePackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// CreatePackageHuma decodes+validates the raw body imperatively (fee validator, inside
// the replicated body-parsing span) then delegates to the shared createPackage core.
func (handler *PackageHandler) CreatePackageHuma(ctx context.Context, in *CreatePackageInputHuma) (*CreatePackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.CreatePackageInput)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packOut, err := handler.createPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreatePackageOutputHuma{Status: http.StatusCreated, Body: packOut}, nil
}

// --- GET /packages (list) -----------------------------------------------------

// ListPackagesInputHuma advertises the list query params in the spec (doc-only, no
// validation tags — the fee core is the sole validator) and captures the raw query
// via Resolve for the imperative binder.
type ListPackagesInputHuma struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	SegmentID        string `query:"segmentId" doc:"Filter by segment ID (UUID)"`
	LedgerID         string `query:"ledgerId" doc:"Filter by ledger ID (UUID)"`
	TransactionRoute string `query:"transactionRoute" doc:"Filter by transaction route"`
	Enable           string `query:"enable" doc:"Filter by enabled flag (true, false)"`
	Limit            string `query:"limit" doc:"Number of items per page (default 10)"`
	Page             string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the getAllPackages core.
func (in *ListPackagesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListPackagesOutputHuma carries the pagination envelope verbatim.
type ListPackagesOutputHuma struct {
	Status int
	Body   model.Pagination
}

// GetAllPackagesHuma binds the query imperatively then delegates to getAllPackages.
func (handler *PackageHandler) GetAllPackagesHuma(ctx context.Context, in *ListPackagesInputHuma) (*ListPackagesOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPackages(ctx, orgID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPackagesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /packages/{id} -------------------------------------------------------

// GetPackageInputHuma is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator).
type GetPackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Package ID (UUID)"`
}

// GetPackageOutputHuma carries the package verbatim.
type GetPackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// GetPackageByIDHuma delegates to getPackageByID.
func (handler *PackageHandler) GetPackageByIDHuma(ctx context.Context, in *GetPackageInputHuma) (*GetPackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packModel, err := handler.getPackageByID(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetPackageOutputHuma{Status: http.StatusOK, Body: packModel}, nil
}

// --- PATCH /packages/{id} -----------------------------------------------------

// UpdatePackageInputHuma is the update request envelope (RawBody, see Create).
type UpdatePackageInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Package ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdatePackageOutputHuma carries the updated package (200, matching the Fiber
// fiber.StatusOK).
type UpdatePackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// UpdatePackageByIDHuma decodes+validates the raw body imperatively (fee validator,
// inside the replicated body-parsing span) then delegates to the shared
// updatePackageByID core.
func (handler *PackageHandler) UpdatePackageByIDHuma(ctx context.Context, in *UpdatePackageInputHuma) (*UpdatePackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.UpdatePackageInput)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packUpdated, err := handler.updatePackageByID(ctx, orgID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdatePackageOutputHuma{Status: http.StatusOK, Body: packUpdated}, nil
}

// --- DELETE /packages/{id} ----------------------------------------------------

// DeletePackageOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber fiber.StatusNoContent path.
type DeletePackageOutputHuma struct{}

// DeletePackageByIDHuma delegates to deletePackageByID; returns a bodiless 204 on success.
func (handler *PackageHandler) DeletePackageByIDHuma(ctx context.Context, in *GetPackageInputHuma) (*DeletePackageOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deletePackageByID(ctx, orgID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeletePackageOutputHuma{}, nil
}

// RegisterPackageRoutes registers the five migrated fee-package operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// ("plugin-fees","packages",verb) + tenant + ParseUUIDPathParameters("packages")
// middleware chain is attached on the /v1 group (Fiber-level) BEFORE the Huma
// terminal, not here. Paths are GROUP-RELATIVE (see asset_handler_huma.go's
// RegisterAssetRoutes header for the /v1 rationale).
func RegisterPackageRoutes(api huma.API, h *PackageHandler) {
	const (
		listPath = "/organizations/{organization_id}/packages"
		idPath   = listPath + "/{id}"
		tag      = "Packages"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createPackage",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a Package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.CreatePackageHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllPackages",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all packages",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetAllPackagesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getPackageByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetPackageByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePackage",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a package",
		Tags:             []string{tag},
		Security:         secPackageBearer,
		SkipValidateBody: true, // body validated imperatively — see createPackage.
	}, h.UpdatePackageByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deletePackage",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a Package by ID",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeletePackageByIDHuma)
}
