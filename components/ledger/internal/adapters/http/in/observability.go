// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"reflect"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// payloadField maps a struct field name to its observability label.
type payloadField struct {
	name      string // Go struct field name
	attrKey   string // OTel attribute key
	logLabel  string // log summary label
	mergeWith string // if non-empty, this field is OR-merged with another (e.g. "AccountId" merges with "AccountID")
}

// payloadFields is the canonical, ordered list of fields inspected for every
// request payload.  Declared once at package level to avoid per-call allocation.
var payloadFields = []payloadField{
	// Common
	{name: "Metadata", attrKey: "app.request.payload.has_metadata", logLabel: "hasMetadata"},
	{name: "Alias", attrKey: "app.request.payload.has_alias", logLabel: "hasAlias"},
	// Onboarding entities
	{name: "ParentAccountID", attrKey: "app.request.payload.has_parent_account_id", logLabel: "hasParentAccountID"},
	{name: "ParentOrganizationID", attrKey: "app.request.payload.has_parent_organization_id", logLabel: "hasParentOrganizationID"},
	{name: "PortfolioID", attrKey: "app.request.payload.has_portfolio_id", logLabel: "hasPortfolioID"},
	{name: "SegmentID", attrKey: "app.request.payload.has_segment_id", logLabel: "hasSegmentID"},
	{name: "EntityID", attrKey: "app.request.payload.has_entity_id", logLabel: "hasEntityID"},
	{name: "LegalDocument", attrKey: "app.request.payload.has_legal_document", logLabel: "hasLegalDocument"},
	// Transaction entities
	{name: "Key", attrKey: "app.request.payload.has_key", logLabel: "hasKey"},
	{name: "AccountID", attrKey: "app.request.payload.has_account_id", logLabel: "hasAccountID"},
	{name: "AccountId", attrKey: "", logLabel: "", mergeWith: "AccountID"},
	{name: "LedgerID", attrKey: "app.request.payload.has_ledger_id", logLabel: "hasLedgerID"},
	{name: "LedgerId", attrKey: "", logLabel: "", mergeWith: "LedgerID"},
	{name: "OrganizationID", attrKey: "app.request.payload.has_organization_id", logLabel: "hasOrganizationID"},
	{name: "OrganizationId", attrKey: "", logLabel: "", mergeWith: "OrganizationID"},
	{name: "TransactionID", attrKey: "app.request.payload.has_transaction_id", logLabel: "hasTransactionID"},
	{name: "TransactionId", attrKey: "", logLabel: "", mergeWith: "TransactionID"},
	{name: "ParentTransactionID", attrKey: "app.request.payload.has_parent_transaction_id", logLabel: "hasParentTransactionID"},
	{name: "ParentTransactionId", attrKey: "", logLabel: "", mergeWith: "ParentTransactionID"},
	{name: "Document", attrKey: "app.request.payload.has_document", logLabel: "hasDocument"},
	{name: "Send", attrKey: "app.request.payload.has_send", logLabel: "hasSend"},
	{name: "Source", attrKey: "app.request.payload.has_source", logLabel: "hasSource"},
	{name: "Distribution", attrKey: "app.request.payload.has_distribution", logLabel: "hasDistribution"},
	{name: "Account", attrKey: "app.request.payload.has_account_rule", logLabel: "hasAccountRule"},
	{name: "ValidIf", attrKey: "app.request.payload.has_valid_if", logLabel: "hasValidIf"},
}

func logSafePayload(ctx context.Context, logger libLog.Logger, message string, payload any) {
	if logger == nil || !logger.Enabled(libLog.LevelInfo) {
		return
	}

	logger.Log(ctx, libLog.LevelInfo, message+" ("+safePayloadSummary(payload)+")")
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

// resolvePayloadValue dereferences the payload through any pointer chain and
// returns the underlying struct reflect.Value.  If the payload is nil, not a
// pointer-to-struct, or a nil pointer, the returned value is invalid
// (value.IsValid() == false).
func resolvePayloadValue(payload any) reflect.Value {
	v := reflect.ValueOf(payload)
	if !v.IsValid() {
		return v
	}

	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return reflect.Value{}
	}

	return v
}

// fieldPresent checks whether the named field exists on the already-resolved
// struct value and is non-zero.
func fieldPresent(resolved reflect.Value, fieldName string) bool {
	if !resolved.IsValid() {
		return false
	}

	f := resolved.FieldByName(fieldName)
	if !f.IsValid() {
		return false
	}

	return !f.IsZero()
}

func safePayloadSummary(payload any) string {
	resolved := resolvePayloadValue(payload)

	parts := []string{"type=" + payloadTypeName(resolved)}

	// presence tracks OR-merged fields so the log label appears at most once.
	presence := make(map[string]bool, len(payloadFields))

	for i := range payloadFields {
		f := &payloadFields[i]
		present := fieldPresent(resolved, f.name)

		if f.mergeWith != "" {
			presence[f.mergeWith] = presence[f.mergeWith] || present

			continue
		}

		presence[f.name] = presence[f.name] || present
	}

	for i := range payloadFields {
		f := &payloadFields[i]
		if f.logLabel != "" && presence[f.name] {
			parts = append(parts, f.logLabel+"=true")
		}
	}

	return strings.Join(parts, ", ")
}

func safePayloadAttributes(payload any) []attribute.KeyValue {
	resolved := resolvePayloadValue(payload)

	attrs := make([]attribute.KeyValue, 0, len(payloadFields)+1)
	attrs = append(attrs, attribute.String("app.request.payload.type", payloadTypeName(resolved)))

	// presence tracks OR-merged fields (e.g. AccountID || AccountId).
	presence := make(map[string]bool, len(payloadFields))

	for i := range payloadFields {
		f := &payloadFields[i]
		present := fieldPresent(resolved, f.name)

		if f.mergeWith != "" {
			presence[f.mergeWith] = presence[f.mergeWith] || present

			continue
		}

		presence[f.name] = presence[f.name] || present
	}

	for i := range payloadFields {
		f := &payloadFields[i]
		if f.attrKey != "" {
			attrs = append(attrs, attribute.Bool(f.attrKey, presence[f.name]))
		}
	}

	return attrs
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

// payloadTypeName returns the struct type name from an already-resolved
// reflect.Value.  Falls back to "unknown" for invalid values.
func payloadTypeName(resolved reflect.Value) string {
	if !resolved.IsValid() {
		return "unknown"
	}

	t := resolved.Type()
	if t.Name() != "" {
		return t.Name()
	}

	return t.String()
}
