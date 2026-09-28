// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// handleSpanByErrorClass records err onto span using the helper appropriate to
// the error's class: business/4xx errors keep the span status green; technical/5xx
// errors flip it red. Use it at the handler boundary for errors returned from
// use cases, where the class is not known statically.
func handleSpanByErrorClass(span trace.Span, message string, err error) {
	spanattr.HandleSpanByErrorClass(span, message, err)
}

// recordSafePayloadAttributes records the presence-only shape of a request payload
// on span: which known fields are set, never their values.
func recordSafePayloadAttributes(span trace.Span, payload any) {
	spanattr.RecordSafePayloadAttributes(span, payload)
}

func recordSafeQueryAttributes(span trace.Span, query *midazhttp.QueryHeader) {
	if span == nil {
		return
	}

	span.SetAttributes(safeQueryAttributes(query)...)
}

func safeQueryAttributes(query *midazhttp.QueryHeader) []attribute.KeyValue {
	if query == nil {
		return []attribute.KeyValue{attribute.Bool("app.request.query.present", false)}
	}

	return []attribute.KeyValue{
		attribute.Bool("app.request.query.present", true),
		attribute.Int("app.request.query.limit", query.Limit),
		attribute.Int("app.request.query.page", query.Page),
		attribute.String("app.request.query.sort_order", query.SortOrder),
		attribute.Bool("app.request.query.has_cursor", query.Cursor != ""),
		attribute.Bool("app.request.query.has_metadata", query.Metadata != nil),
		attribute.Bool("app.request.query.has_date_range", !query.StartDate.IsZero() || !query.EndDate.IsZero()),
		// Onboarding queries
		attribute.Bool("app.request.query.has_portfolio_id", query.PortfolioID != ""),
		attribute.Bool("app.request.query.has_name_filters", query.HasNameFilters()),
		attribute.Bool("app.request.query.has_holder_id", query.HolderID != nil),
		// Shared queries
		attribute.Bool("app.request.query.has_document", query.Document != nil),
		attribute.Bool("app.request.query.has_account_id", query.AccountID != nil),
		attribute.Bool("app.request.query.has_ledger_id", query.LedgerID != nil),
		attribute.Bool("app.request.query.has_related_party_filters", query.InstrumentRelatedPartyDocument != nil || query.InstrumentRelatedPartyRole != nil),
		attribute.Bool("app.request.query.has_banking_details_filters", query.InstrumentBankingDetailsBranch != nil || query.InstrumentBankingDetailsAccount != nil || query.InstrumentBankingDetailsIban != nil),
		attribute.Int("app.request.query.to_asset_codes_count", len(query.ToAssetCodes)),
	}
}
