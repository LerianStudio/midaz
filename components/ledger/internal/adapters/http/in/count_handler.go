// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the transaction-count HEAD operation. It mirrors the asset
// exemplar's HEAD-count shape (asset_handler.go, CountAssets*) and the
// balance/operation siblings; see those headers for the full conventions.
// Count-specific notes:
//
//  1. org+ledger are UUID-validated by ParseUUIDPathParameters("transaction") — the
//     sole UUID validator on this route — so the In struct carries them as plain
//     strings with only `doc:` (no format tag => no native Huma 422).
//  2. Unlike the asset HEAD count (no filters), this op carries optional query
//     filters (route, status, start_date, end_date). They are declared doc-only (NO
//     format/enum tags) and captured via Resolve, then validated imperatively by
//     buildCountFilter — never a native Huma 422. A bad filter yields the canonical
//     400.
//  3. The Out struct is header-only (X-Total-Count + Content-Length, no Body field);
//     paired with DefaultStatus 204 it emits a bodiless 204 carrying the count
//     header.
//  4. Errors go through the shared pkgHTTP.HumaProblem; auth is the Fiber guard
//     chain (auth.Authorize("midaz","transactions","head") + tenant + ParseUUID)
//     attached in the unified server BEFORE the Huma terminal — the per-op Security
//     metadata below is SPEC-ONLY.

// --- HEAD /transactions/metrics/count -----------------------------------------

// CountTransactionsRequest advertises the count query filters in the spec (doc-
// only, no validation tags) and captures the raw query via Resolve for the
// imperative buildCountFilter binder. org+ledger are UUID-validated by
// ParseUUIDPathParameters (no format tag).
type CountTransactionsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Route          string `query:"route" doc:"Filter by transaction route"`
	Status         string `query:"status" doc:"Filter by transaction status (CREATED, APPROVED, PENDING, CANCELED, NOTED)"`
	StartDate      string `query:"start_date" doc:"Start of date range (RFC 3339, defaults to today 00:00:00 UTC)"`
	EndDate        string `query:"end_date" doc:"End of date range (RFC 3339, defaults to today 23:59:59 UTC)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Query() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in buildCountFilter.
func (in *CountTransactionsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// CountTransactionsResponse carries the HEAD-count response headers: X-Total-Count
// holds the count, Content-Length is pinned to 0, and the body is empty at status
// 204 (DefaultStatus 204 + no Body field).
type CountTransactionsResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountTransactionsByFilters binds the query filters imperatively (buildCountFilter)
// then delegates to the countTransactionsByFilters core, setting the count headers.
func (handler *TransactionHandler) CountTransactionsByFilters(ctx context.Context, in *CountTransactionsRequest) (*CountTransactionsResponse, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	filter, err := buildCountFilter(in.rawQuery.Get("route"), in.rawQuery.Get("status"), in.rawQuery.Get("start_date"), in.rawQuery.Get("end_date"))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countTransactionsByFilters(ctx, organizationID, ledgerID, filter)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountTransactionsResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterCountTransactionRoutes registers the transaction-count HEAD op on the
// shared Huma API. It is the per-file seam the unified server calls;
// the auth (auth.Authorize("midaz","transactions","head")) + tenant +
// ParseUUIDPathParameters("transaction") chain for this route is attached in the
// unified server (Fiber level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (the group's PrefixModifier writes the version into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation ID one version group publishes from another's — see
// routeOpSuffixV1. The v1 group passes the empty suffix so its ID stays exactly what published
// SDKs bind to; the v2 group passes "V2" so its twin does not collide in the one document.
func RegisterCountTransactionRoutes(api huma.API, h *TransactionHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "countTransactionsByFilters" + opSuffix,
		Method:      http.MethodHead,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/metrics/count",
		Summary:     "Count Transactions by Filters",
		Tags:        []string{"Transactions"},
		Security:    secAssetBearerOrAPIKey,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set on
		// the Out struct).
		DefaultStatus: http.StatusNoContent,
	}, h.CountTransactionsByFilters)
}
