// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger-scoped (v2) shell layer over the fee and billing surface.
// Every operation here is the ledger-scoped twin of an organization-scoped shell in
// fees_package_handler_huma.go, fees_handler_huma.go, billing_package_handler_huma.go
// or billing_calculate_handler_huma.go, and it delegates to the SAME transport-
// agnostic core. Only two things differ, and both are about which ledger the request
// acts within:
//
//  1. The path names the ledger, so the shells resolve it via parseFeeV2Path and hand
//     it to the core, which passes it to the by-ID scope filters and pins the listings.
//     The organization-scoped shells pass uuid.Nil there and keep their old query.
//  2. The bodies that carry a ledger still carry it — the field is required on models
//     shared with the organization-scoped surface and with the in-process fee seam, so
//     it cannot be dropped for one caller — and the shells refuse a value that names a
//     different ledger than the path. See requireBodyLedgerMatchesPath.
//
// Everything else is preserved verbatim from the organization-scoped shells: the fee
// body validator inside the replicated body-parsing span (decodeFeeBodyInSpan, NOT
// pkgHTTP.DecodeAndValidate), the imperative query binder on the two listings, the
// pkgHTTP.HumaProblem error envelope, and the response types — the two contracts name
// the same component schemas because they are generated from the same Go types into
// separate documents.
//
// AUTH is unchanged: appName "plugin-fees" with the same (resource, verb) tuples the
// organization-scoped routes carry, attached as the Fiber guard chain in
// RegisterFeesV2RoutesToApp BEFORE these terminals. The per-op Security metadata on
// the registrations is spec metadata only.

// FeeV2PathHuma is the ledger-scoped path prefix every v2 fee and billing operation
// carries. The parameters have no format tag: ParseUUIDPathParameters on the Fiber
// guard chain stays the sole path-UUID validator, as it is on every other migrated
// resource.
type FeeV2PathHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// parseFeeV2Path resolves the organization and the ledger a ledger-scoped fee request
// acts within.
//
// The nil identifier is refused rather than carried inward. It is a syntactically
// valid UUID, so ParseUUIDPathParameters admits it, and both fee repositories read it
// as "no ledger requested" — a by-ID read would then match a package on any ledger of
// the organization and a listing would return every ledger's. No ledger is created
// with it, so nothing legitimate is turned away.
func parseFeeV2Path(p FeeV2PathHuma) (organizationID, ledgerID uuid.UUID, err error) {
	organizationID, ledgerID, err = parseOrgLedger(p.OrganizationID, p.LedgerID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if ledgerID == uuid.Nil {
		return uuid.Nil, uuid.Nil, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", "ledger_id")
	}

	return organizationID, ledgerID, nil
}

// --- POST /ledgers/{ledger_id}/packages -----------------------------------------

// CreatePackageV2InputHuma is the ledger-scoped create envelope. RawBody keeps the
// body out of Huma's validator (see file header).
type CreatePackageV2InputHuma struct {
	FeeV2PathHuma
	RawBody []byte `contentType:"application/json"`
}

// CreatePackageV2Huma decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared createPackage
// core.
func (handler *PackageHandler) CreatePackageV2Huma(ctx context.Context, in *CreatePackageV2InputHuma) (*CreatePackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
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

	return &CreatePackageOutputHuma{Status: http.StatusCreated, Body: packOut}, nil
}

// --- GET /ledgers/{ledger_id}/packages (list) -----------------------------------

// ListPackagesV2InputHuma advertises the ledger-scoped list query params in the spec
// (doc-only, no validation tags — the fee core is the sole validator) and captures the
// raw query via Resolve for the imperative binder.
//
// There is no ledgerId param: the path names the ledger and the core refuses the key.
type ListPackagesV2InputHuma struct {
	FeeV2PathHuma

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
func (in *ListPackagesV2InputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAllPackagesV2Huma binds the query imperatively then delegates to the
// ledger-scoped listing.
func (handler *PackageHandler) GetAllPackagesV2Huma(ctx context.Context, in *ListPackagesV2InputHuma) (*ListPackagesOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPackagesInLedger(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPackagesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET/DELETE /ledgers/{ledger_id}/packages/{id} -------------------------------

// PackageIDV2InputHuma is the ledger-scoped by-id envelope, shared by the read and the
// delete. The id path param carries no format tag (ParseUUIDPathParameters is the sole
// validator).
type PackageIDV2InputHuma struct {
	FeeV2PathHuma

	ID string `path:"id" doc:"Package ID (UUID)"`
}

// GetPackageByIDV2Huma delegates to the shared getPackageByID core with the path
// ledger, so a package another ledger of the organization owns reads as absent.
func (handler *PackageHandler) GetPackageByIDV2Huma(ctx context.Context, in *PackageIDV2InputHuma) (*GetPackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
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

	return &GetPackageOutputHuma{Status: http.StatusOK, Body: packModel}, nil
}

// DeletePackageByIDV2Huma delegates to deletePackageByID with the path ledger; returns
// a bodiless 204 on success.
func (handler *PackageHandler) DeletePackageByIDV2Huma(ctx context.Context, in *PackageIDV2InputHuma) (*DeletePackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
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

	return &DeletePackageOutputHuma{}, nil
}

// --- PATCH /ledgers/{ledger_id}/packages/{id} ------------------------------------

// UpdatePackageV2InputHuma is the ledger-scoped update envelope (RawBody, see Create).
type UpdatePackageV2InputHuma struct {
	FeeV2PathHuma

	ID      string `path:"id" doc:"Package ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdatePackageByIDV2Huma decodes+validates the raw body imperatively then delegates
// to the shared updatePackageByID core with the path ledger. The update body carries
// no ledger, so there is nothing to reconcile against the path.
func (handler *PackageHandler) UpdatePackageByIDV2Huma(ctx context.Context, in *UpdatePackageV2InputHuma) (*UpdatePackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
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

	return &UpdatePackageOutputHuma{Status: http.StatusOK, Body: packUpdated}, nil
}

// --- POST /ledgers/{ledger_id}/estimates -----------------------------------------

// EstimateFeeV2InputHuma is the ledger-scoped estimate envelope (RawBody, see Create).
type EstimateFeeV2InputHuma struct {
	FeeV2PathHuma
	RawBody []byte `contentType:"application/json"`
}

// EstimateFeeCalculationV2Huma decodes+validates the raw body imperatively, refuses a
// body ledger that disagrees with the path, then delegates to the shared
// estimateFeeCalculation core and serializes the envelope verbatim.
//
// The response Body stays a pre-serialized []byte for the reason
// EstimateFeeOutputHuma documents: the estimate embeds the projected transaction tree,
// whose time alias makes Huma's schema generator panic. The escape hatch is a property
// of the response type, so it holds identically on both contracts.
func (handler *FeeHandler) EstimateFeeCalculationV2Huma(ctx context.Context, in *EstimateFeeV2InputHuma) (*EstimateFeeOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.FeeEstimate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.estimateFeeCalculation(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	body, err := json.Marshal(response)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "Fee"))
	}

	return &EstimateFeeOutputHuma{Status: http.StatusOK, Body: body}, nil
}

// --- POST /ledgers/{ledger_id}/billing-packages ----------------------------------

// CreateBillingPackageV2InputHuma is the ledger-scoped create envelope (RawBody, see
// CreatePackageV2InputHuma).
type CreateBillingPackageV2InputHuma struct {
	FeeV2PathHuma
	RawBody []byte `contentType:"application/json"`
}

// CreateBillingPackageV2Huma decodes+validates the raw body imperatively, refuses a
// body ledger that disagrees with the path, then delegates to the shared
// createBillingPackage core.
func (handler *BillingPackageHandler) CreateBillingPackageV2Huma(ctx context.Context, in *CreateBillingPackageV2InputHuma) (*CreateBillingPackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingPackage)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.createBillingPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateBillingPackageOutputHuma{Status: http.StatusCreated, Body: result}, nil
}

// --- GET /ledgers/{ledger_id}/billing-packages (list) ----------------------------

// ListBillingPackagesV2InputHuma advertises the ledger-scoped list query params in the
// spec (doc-only, no validation tags — the core is the sole validator) and captures the
// raw query via Resolve for the imperative binder.
//
// There is no ledgerId param: the path names the ledger and the core refuses the key.
type ListBillingPackagesV2InputHuma struct {
	FeeV2PathHuma

	Type  string `query:"type" doc:"Filter by billing package type (volume, maintenance)"`
	Limit string `query:"limit" doc:"Number of items per page (default 10)"`
	Page  string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the core.
func (in *ListBillingPackagesV2InputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAllBillingPackagesV2Huma binds the query imperatively then delegates to the
// ledger-scoped listing.
func (handler *BillingPackageHandler) GetAllBillingPackagesV2Huma(ctx context.Context, in *ListBillingPackagesV2InputHuma) (*ListBillingPackagesOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBillingPackagesInLedger(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBillingPackagesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET/DELETE /ledgers/{ledger_id}/billing-packages/{id} -----------------------

// BillingPackageIDV2InputHuma is the ledger-scoped by-id envelope, shared by the read
// and the delete.
type BillingPackageIDV2InputHuma struct {
	FeeV2PathHuma

	ID string `path:"id" doc:"BillingPackage ID (UUID)"`
}

// GetBillingPackageByIDV2Huma delegates to the shared getBillingPackageByID core with
// the path ledger, so a package another ledger of the organization owns reads as
// absent.
func (handler *BillingPackageHandler) GetBillingPackageByIDV2Huma(ctx context.Context, in *BillingPackageIDV2InputHuma) (*GetBillingPackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.getBillingPackageByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetBillingPackageOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DeleteBillingPackageV2Huma delegates to deleteBillingPackage with the path ledger;
// returns a bodiless 204 on success.
func (handler *BillingPackageHandler) DeleteBillingPackageV2Huma(ctx context.Context, in *BillingPackageIDV2InputHuma) (*DeleteBillingPackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteBillingPackage(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteBillingPackageOutputHuma{}, nil
}

// --- PATCH /ledgers/{ledger_id}/billing-packages/{id} ----------------------------

// UpdateBillingPackageV2InputHuma is the ledger-scoped update envelope (RawBody, see
// CreatePackageV2InputHuma).
type UpdateBillingPackageV2InputHuma struct {
	FeeV2PathHuma

	ID      string `path:"id" doc:"BillingPackage ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateBillingPackageV2Huma decodes+validates the raw body imperatively then
// delegates to the shared updateBillingPackage core with the path ledger. The update
// body carries no ledger, so there is nothing to reconcile against the path.
func (handler *BillingPackageHandler) UpdateBillingPackageV2Huma(ctx context.Context, in *UpdateBillingPackageV2InputHuma) (*UpdateBillingPackageOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
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

	result, err := handler.updateBillingPackage(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateBillingPackageOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// --- POST /ledgers/{ledger_id}/billing/calculate ---------------------------------

// CalculateBillingV2InputHuma is the ledger-scoped calculate envelope (RawBody, see
// CreatePackageV2InputHuma).
type CalculateBillingV2InputHuma struct {
	FeeV2PathHuma
	RawBody []byte `contentType:"application/json"`
}

// CalculateBillingV2Huma decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared calculateBilling
// core.
func (handler *BillingCalculateHandler) CalculateBillingV2Huma(ctx context.Context, in *CalculateBillingV2InputHuma) (*CalculateBillingOutputHuma, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2PathHuma)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingCalculateRequest)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.calculateBilling(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CalculateBillingOutputHuma{Status: http.StatusOK, Body: result}, nil
}
