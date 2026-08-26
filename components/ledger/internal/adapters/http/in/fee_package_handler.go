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

// This file is the Huma transport of the fee-package resource: the response
// envelopes, the per-operation security metadata, and the five shells that decode a
// request, call a core in fee_package_core.go, and render the envelope.
//
// Every shell names the ledger in its path and resolves it via parseFeeV2Path, then
// hands it to the core, which passes it to the by-ID scope filters and pins the
// listings. A body that also carries a ledger must agree with the path — the field is
// required on the models shared with the in-process fee seam, so it cannot be dropped
// for one caller. See requireBodyLedgerMatchesPath in fee_ledger_scope.go.
//
// Body ops carry RawBody + SkipValidateBody, and the fee body validator runs inside
// the replicated body-parsing span (decodeFeeBodyInSpan, NOT
// pkgHTTP.DecodeAndValidate). The listing binds its query imperatively. Errors go out
// through the pkgHTTP.HumaProblem envelope.
//
// AUTH is appName "midaz" (routes.go midazName), resource "packages". The Fiber guard
// chain — auth.Authorize("midaz","packages",verb) + the fees-scoped tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("packages") — is attached on the /v2
// group BEFORE these terminals (see fee_package_routes.go), so the per-op Security
// metadata is SPEC metadata only.

// secPackageBearer advertises that each package operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secPackageBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreatePackageResponse pins 201.
type CreatePackageResponse struct {
	Status int
	Body   *pack.Package
}

// ListPackagesResponse carries the pagination envelope verbatim.
type ListPackagesResponse struct {
	Status int
	Body   model.Pagination
}

// GetPackageResponse carries the package verbatim.
type GetPackageResponse struct {
	Status int
	Body   *pack.Package
}

// UpdatePackageResponse carries the updated package at 200.
type UpdatePackageResponse struct {
	Status int
	Body   *pack.Package
}

// DeletePackageResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204.
type DeletePackageResponse struct{}

// --- POST /ledgers/{ledger_id}/packages -----------------------------------------

// CreatePackageV2Request is the ledger-scoped create envelope. RawBody keeps the
// body out of Huma's validator (see file header).
type CreatePackageV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// CreatePackageV2 decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared createPackage
// core.
func (handler *PackageHandler) CreatePackageV2(ctx context.Context, in *CreatePackageV2Request) (*CreatePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.CreatePackageInput)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packOut, err := handler.createPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreatePackageResponse{Status: http.StatusCreated, Body: packOut}, nil
}

// --- GET /ledgers/{ledger_id}/packages (list) -----------------------------------

// ListPackagesV2Request advertises the ledger-scoped list query params in the spec
// (doc-only, no validation tags — the fee core is the sole validator) and captures the
// raw query via Resolve for the imperative binder.
//
// There is no ledgerId param: the path names the ledger and the core refuses the key.
type ListPackagesV2Request struct {
	FeeV2Path

	SegmentID        string `query:"segmentId" doc:"Filter by segment ID (UUID)"`
	TransactionRoute string `query:"transactionRoute" doc:"Filter by transaction route"`
	Enable           string `query:"enable" doc:"Filter by enabled flag (true, false)"`
	Limit            string `query:"limit" doc:"Number of items per page (default 10)"`
	Page             string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the core.
func (in *ListPackagesV2Request) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAllPackagesV2 binds the query imperatively then delegates to the
// ledger-scoped listing.
func (handler *PackageHandler) GetAllPackagesV2(ctx context.Context, in *ListPackagesV2Request) (*ListPackagesResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPackagesInLedger(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPackagesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET/DELETE /ledgers/{ledger_id}/packages/{id} -------------------------------

// PackageIDV2Request is the ledger-scoped by-id envelope, shared by the read and the
// delete. The id path param carries no format tag (ParseUUIDPathParameters is the sole
// validator).
type PackageIDV2Request struct {
	FeeV2Path

	ID string `path:"id" doc:"Package ID (UUID)"`
}

// GetPackageByIDV2 delegates to the shared getPackageByID core with the path
// ledger, so a package another ledger of the organization owns reads as absent.
func (handler *PackageHandler) GetPackageByIDV2(ctx context.Context, in *PackageIDV2Request) (*GetPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packModel, err := handler.getPackageByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetPackageResponse{Status: http.StatusOK, Body: packModel}, nil
}

// DeletePackageByIDV2 delegates to deletePackageByID with the path ledger; returns
// a bodiless 204 on success.
func (handler *PackageHandler) DeletePackageByIDV2(ctx context.Context, in *PackageIDV2Request) (*DeletePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deletePackageByID(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeletePackageResponse{}, nil
}

// --- PATCH /ledgers/{ledger_id}/packages/{id} ------------------------------------

// UpdatePackageV2Request is the ledger-scoped update envelope (RawBody, see Create).
type UpdatePackageV2Request struct {
	FeeV2Path

	ID      string `path:"id" doc:"Package ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdatePackageByIDV2 decodes+validates the raw body imperatively then delegates
// to the shared updatePackageByID core with the path ledger. The update body carries
// no ledger, so there is nothing to reconcile against the path.
func (handler *PackageHandler) UpdatePackageByIDV2(ctx context.Context, in *UpdatePackageV2Request) (*UpdatePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
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

	packUpdated, err := handler.updatePackageByID(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdatePackageResponse{Status: http.StatusOK, Body: packUpdated}, nil
}
