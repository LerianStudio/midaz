// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func logSafePayload(ctx context.Context, logger libLog.Logger, message string, payload any) {
	if logger == nil {
		return
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("%s (%s)", message, safePayloadSummary(payload)))
}

func recordSafePayloadAttributes(span trace.Span, payload any) {
	if span == nil {
		return
	}

	span.SetAttributes(safePayloadAttributes(payload)...)
}

func recordSafeQueryAttributes(span trace.Span, query *midazhttp.QueryHeader) {
	if span == nil {
		return
	}

	span.SetAttributes(safeQueryAttributes(query)...)
}

func safePayloadSummary(payload any) string {
	parts := []string{fmt.Sprintf("type=%s", payloadTypeName(payload))}

	for _, field := range []struct {
		name  string
		label string
	}{
		// Common
		{name: "Metadata", label: "hasMetadata"},
		{name: "Alias", label: "hasAlias"},
		// Onboarding entities
		{name: "ParentAccountID", label: "hasParentAccountID"},
		{name: "ParentOrganizationID", label: "hasParentOrganizationID"},
		{name: "PortfolioID", label: "hasPortfolioID"},
		{name: "SegmentID", label: "hasSegmentID"},
		{name: "EntityID", label: "hasEntityID"},
		{name: "LegalDocument", label: "hasLegalDocument"},
		// Transaction entities
		{name: "Key", label: "hasKey"},
		{name: "AccountID", label: "hasAccountID"},
		{name: "AccountId", label: "hasAccountID"},
		{name: "LedgerID", label: "hasLedgerID"},
		{name: "LedgerId", label: "hasLedgerID"},
		{name: "OrganizationID", label: "hasOrganizationID"},
		{name: "OrganizationId", label: "hasOrganizationID"},
		{name: "TransactionID", label: "hasTransactionID"},
		{name: "TransactionId", label: "hasTransactionID"},
		{name: "ParentTransactionID", label: "hasParentTransactionID"},
		{name: "ParentTransactionId", label: "hasParentTransactionID"},
		{name: "Document", label: "hasDocument"},
		{name: "Send", label: "hasSend"},
		{name: "Source", label: "hasSource"},
		{name: "Distribution", label: "hasDistribution"},
		{name: "Account", label: "hasAccountRule"},
		{name: "ValidIf", label: "hasValidIf"},
	} {
		if payloadFieldPresent(payload, field.name) {
			parts = append(parts, field.label+"=true")
		}
	}

	return strings.Join(parts, ", ")
}

func safePayloadAttributes(payload any) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("app.request.payload.type", payloadTypeName(payload)),
		attribute.Bool("app.request.payload.has_metadata", payloadFieldPresent(payload, "Metadata")),
		attribute.Bool("app.request.payload.has_alias", payloadFieldPresent(payload, "Alias")),
		// Onboarding entities
		attribute.Bool("app.request.payload.has_parent_account_id", payloadFieldPresent(payload, "ParentAccountID")),
		attribute.Bool("app.request.payload.has_parent_organization_id", payloadFieldPresent(payload, "ParentOrganizationID")),
		attribute.Bool("app.request.payload.has_portfolio_id", payloadFieldPresent(payload, "PortfolioID")),
		attribute.Bool("app.request.payload.has_segment_id", payloadFieldPresent(payload, "SegmentID")),
		attribute.Bool("app.request.payload.has_entity_id", payloadFieldPresent(payload, "EntityID")),
		attribute.Bool("app.request.payload.has_legal_document", payloadFieldPresent(payload, "LegalDocument")),
		// Transaction entities
		attribute.Bool("app.request.payload.has_key", payloadFieldPresent(payload, "Key")),
		attribute.Bool("app.request.payload.has_account_id", payloadFieldPresent(payload, "AccountID") || payloadFieldPresent(payload, "AccountId")),
		attribute.Bool("app.request.payload.has_ledger_id", payloadFieldPresent(payload, "LedgerID") || payloadFieldPresent(payload, "LedgerId")),
		attribute.Bool("app.request.payload.has_organization_id", payloadFieldPresent(payload, "OrganizationID") || payloadFieldPresent(payload, "OrganizationId")),
		attribute.Bool("app.request.payload.has_transaction_id", payloadFieldPresent(payload, "TransactionID") || payloadFieldPresent(payload, "TransactionId")),
		attribute.Bool("app.request.payload.has_parent_transaction_id", payloadFieldPresent(payload, "ParentTransactionID") || payloadFieldPresent(payload, "ParentTransactionId")),
		attribute.Bool("app.request.payload.has_document", payloadFieldPresent(payload, "Document")),
		attribute.Bool("app.request.payload.has_send", payloadFieldPresent(payload, "Send")),
		attribute.Bool("app.request.payload.has_source", payloadFieldPresent(payload, "Source")),
		attribute.Bool("app.request.payload.has_distribution", payloadFieldPresent(payload, "Distribution")),
		attribute.Bool("app.request.payload.has_account_rule", payloadFieldPresent(payload, "Account")),
		attribute.Bool("app.request.payload.has_valid_if", payloadFieldPresent(payload, "ValidIf")),
	}
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
		attribute.Bool("app.request.query.has_related_party_filters", query.RelatedPartyDocument != nil || query.RelatedPartyRole != nil),
		attribute.Bool("app.request.query.has_banking_details_filters", query.BankingDetailsBranch != nil || query.BankingDetailsAccount != nil || query.BankingDetailsIban != nil),
		attribute.Int("app.request.query.to_asset_codes_count", len(query.ToAssetCodes)),
	}
}

func payloadTypeName(payload any) string {
	typ := reflect.TypeOf(payload)
	if typ == nil {
		return "unknown"
	}

	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Name() != "" {
		return typ.Name()
	}

	return typ.String()
}

func payloadFieldPresent(payload any, fieldName string) bool {
	value := reflect.ValueOf(payload)
	if !value.IsValid() {
		return false
	}

	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return false
		}

		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return false
	}

	field := value.FieldByName(fieldName)
	if !field.IsValid() {
		return false
	}

	return !field.IsZero()
}
