// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
		attribute.Bool("app.request.query.has_metadata", query.Metadata != nil),
		attribute.Bool("app.request.query.has_date_range", !query.StartDate.IsZero() || !query.EndDate.IsZero()),
		attribute.Bool("app.request.query.has_holder_id", query.HolderID != nil),
		attribute.Bool("app.request.query.has_external_id", query.ExternalID != nil),
		attribute.Bool("app.request.query.has_document", query.Document != nil),
		attribute.Bool("app.request.query.has_account_id", query.AccountID != nil),
		attribute.Bool("app.request.query.has_ledger_id", query.LedgerID != nil),
		attribute.Bool("app.request.query.has_related_party_filters", query.RelatedPartyDocument != nil || query.RelatedPartyRole != nil),
		attribute.Bool("app.request.query.has_banking_details_filters", query.BankingDetailsBranch != nil || query.BankingDetailsAccount != nil || query.BankingDetailsIban != nil),
	}
}
