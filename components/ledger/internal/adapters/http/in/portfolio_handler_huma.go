// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the portfolio resource, following the
// asset exemplar (asset_handler_huma.go) verbatim: shared parseOrgLedger /
// parsePathUUID / HumaProblem / DecodeAndValidate helpers, path params as plain
// strings (ParseUUIDPathParameters is the sole UUID validator — no format tag), raw
// body bytes decoded imperatively (no native Huma 422), and the query bound via the
// same ValidateParameters path. Auth stays the Fiber middleware chain attached in
// RegisterPortfolioRoutesToApp; the per-op Security metadata is SPEC-ONLY.

// secPortfolioBearerOrAPIKey advertises that each portfolio operation accepts EITHER
// a JWT bearer token OR an X-API-Key (SPEC metadata only; runtime auth is the Fiber
// guard chain). Scheme names are declared once on the shared Huma API.
var secPortfolioBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /portfolios ---------------------------------------------------------

// CreatePortfolioInputHuma is the Huma request envelope for POST.
type CreatePortfolioInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreatePortfolioOutputHuma pins 201 (matching http.Created).
type CreatePortfolioOutputHuma struct {
	Status int
	Body   *mmodel.Portfolio
}

// CreatePortfolioHuma decodes+validates the raw body imperatively then delegates to
// the shared createPortfolio core.
func (handler *PortfolioHandler) CreatePortfolioHuma(ctx context.Context, in *CreatePortfolioInputHuma) (*CreatePortfolioOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreatePortfolioInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.createPortfolio(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreatePortfolioOutputHuma{Status: http.StatusCreated, Body: portfolio}, nil
}

// --- GET /portfolios (list) ---------------------------------------------------

// ListPortfoliosInputHuma advertises the list query params (doc-only) and captures
// the raw query via Resolve for the imperative ValidateParameters binder.
type ListPortfoliosInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter portfolios by metadata fields"`
	EntityID       string `query:"entity_id" doc:"Filter portfolios by entity ID"`
	Status         string `query:"status" doc:"Filter portfolios by status"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	StartDate      string `query:"start_date" doc:"Filter portfolios created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter portfolios created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (NO validation, never errors).
func (in *ListPortfoliosInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that ValidateParameters consumes, matching
// Fiber's c.Queries() (last value wins for a repeated key, empty keys included).
func (in *ListPortfoliosInputHuma) queries() map[string]string {
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

// ListPortfoliosOutputHuma carries the pagination envelope verbatim.
type ListPortfoliosOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListPortfoliosHuma binds the query imperatively then delegates to getAllPortfolios.
func (handler *PortfolioHandler) ListPortfoliosHuma(ctx context.Context, in *ListPortfoliosInputHuma) (*ListPortfoliosOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPortfolios(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPortfoliosOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /portfolios/{id} -----------------------------------------------------

// GetPortfolioInputHuma is the by-id request envelope.
type GetPortfolioInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Portfolio ID (UUID)"`
}

// GetPortfolioOutputHuma carries the portfolio verbatim.
type GetPortfolioOutputHuma struct {
	Status int
	Body   *mmodel.Portfolio
}

// GetPortfolioByIDHuma delegates to getPortfolioByID.
func (handler *PortfolioHandler) GetPortfolioByIDHuma(ctx context.Context, in *GetPortfolioInputHuma) (*GetPortfolioOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.getPortfolioByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetPortfolioOutputHuma{Status: http.StatusOK, Body: portfolio}, nil
}

// --- PATCH /portfolios/{id} ---------------------------------------------------

// UpdatePortfolioInputHuma is the update request envelope (RawBody, see Create).
type UpdatePortfolioInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Portfolio ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdatePortfolioOutputHuma carries the updated portfolio (200, matching http.OK).
type UpdatePortfolioOutputHuma struct {
	Status int
	Body   *mmodel.Portfolio
}

// UpdatePortfolioHuma decodes+validates the raw body imperatively then delegates to
// the shared updatePortfolio core.
func (handler *PortfolioHandler) UpdatePortfolioHuma(ctx context.Context, in *UpdatePortfolioInputHuma) (*UpdatePortfolioOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdatePortfolioInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.updatePortfolio(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdatePortfolioOutputHuma{Status: http.StatusOK, Body: portfolio}, nil
}

// --- DELETE /portfolios/{id} --------------------------------------------------

// DeletePortfolioOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeletePortfolioOutputHuma struct{}

// DeletePortfolioByIDHuma delegates to deletePortfolio; returns a bodiless 204.
func (handler *PortfolioHandler) DeletePortfolioByIDHuma(ctx context.Context, in *GetPortfolioInputHuma) (*DeletePortfolioOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deletePortfolio(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeletePortfolioOutputHuma{}, nil
}

// --- HEAD /portfolios/metrics/count -------------------------------------------

// CountPortfoliosInputHuma is the HEAD-count request envelope (org+ledger only).
type CountPortfoliosInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountPortfoliosOutputHuma replicates the Fiber HEAD-count response: X-Total-Count
// carries the count, Content-Length is pinned to 0, body empty at 204.
type CountPortfoliosOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountPortfoliosHuma delegates to countPortfolios and sets the count headers.
func (handler *PortfolioHandler) CountPortfoliosHuma(ctx context.Context, in *CountPortfoliosInputHuma) (*CountPortfoliosOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countPortfolios(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountPortfoliosOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterPortfolioRoutes registers the six migrated portfolio operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to the /v1 Fiber
// group). The auth + tenant + ParseUUIDPathParameters chain is attached by
// RegisterPortfolioRoutesToApp (Fiber-level), NOT here.
func RegisterPortfolioRoutes(api huma.API, h *PortfolioHandler) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Portfolios"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createPortfolio",
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively (DecodeAndValidate).
	}, h.CreatePortfolioHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listPortfolios",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all portfolios",
		Tags:        []string{tag},
		Security:    secPortfolioBearerOrAPIKey,
	}, h.ListPortfoliosHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getPortfolioByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific portfolio",
		Tags:        []string{tag},
		Security:    secPortfolioBearerOrAPIKey,
	}, h.GetPortfolioByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePortfolio",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdatePortfolioHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePortfolio",
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a portfolio",
		Tags:          []string{tag},
		Security:      secPortfolioBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeletePortfolioByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "countPortfolios",
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total portfolios",
		Tags:          []string{tag},
		Security:      secPortfolioBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountPortfoliosHuma)
}

// RegisterPortfolioRoutesToApp wires the Huma-migrated portfolio resource,
// mirroring RegisterAssetRoutesToApp. For each of the six ops it attaches the Fiber
// auth chain — auth.Authorize("midaz","portfolios",verb) + tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("portfolio") — as MIDDLEWARE ONLY
// (no terminal handler) on the /v1 GROUP with GROUP-RELATIVE paths, then registers
// the Huma terminals via RegisterPortfolioRoutes on the SAME group's Huma API. This
// preserves the pre-Huma (resource, verb) authz tuples and tenant resolution
// BYTE-FOR-BYTE — no portfolio route becomes public.
//
// Called from the unified server's humaMount (integration task), NOT from routes.go.
func RegisterPortfolioRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *midazhttp.ProtectedRouteOptions) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/portfolios"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := midazhttp.ParseUUIDPathParameters("portfolio")

	group.Post(listPath, protectedMidaz(auth, "portfolios", "post", routeOptions, parse)...)
	group.Patch(idPath, protectedMidaz(auth, "portfolios", "patch", routeOptions, parse)...)
	group.Get(listPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse)...)
	group.Get(idPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse)...)
	group.Delete(idPath, protectedMidaz(auth, "portfolios", "delete", routeOptions, parse)...)
	group.Head(countPath, protectedMidaz(auth, "portfolios", "head", routeOptions, parse)...)

	RegisterPortfolioRoutes(api, ph)
}
