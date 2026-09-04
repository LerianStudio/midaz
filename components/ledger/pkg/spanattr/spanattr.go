// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package spanattr holds the span helpers shared by the HTTP handler layer and
// the transaction use cases: error-class-aware span recording and the safe
// (presence-only) payload attributes.
package spanattr

import (
	"reflect"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// payloadField maps a struct field name to its span attribute key.
type payloadField struct {
	name      string // Go struct field name
	attrKey   string // OTel attribute key
	mergeWith string // if non-empty, this field is OR-merged with another (e.g. "AccountId" merges with "AccountID")
}

// payloadFields is the canonical, ordered list of fields inspected for every
// request payload.  Declared once at package level to avoid per-call allocation.
var payloadFields = []payloadField{
	// Common
	{name: "Metadata", attrKey: "app.request.payload.has_metadata"},
	{name: "Alias", attrKey: "app.request.payload.has_alias"},
	// Onboarding entities
	{name: "ParentAccountID", attrKey: "app.request.payload.has_parent_account_id"},
	{name: "ParentOrganizationID", attrKey: "app.request.payload.has_parent_organization_id"},
	{name: "PortfolioID", attrKey: "app.request.payload.has_portfolio_id"},
	{name: "SegmentID", attrKey: "app.request.payload.has_segment_id"},
	{name: "EntityID", attrKey: "app.request.payload.has_entity_id"},
	{name: "LegalDocument", attrKey: "app.request.payload.has_legal_document"},
	{name: "Settings", attrKey: "app.request.payload.has_settings"},
	// Transaction entities
	{name: "Key", attrKey: "app.request.payload.has_key"},
	{name: "AccountID", attrKey: "app.request.payload.has_account_id"},
	{name: "AccountId", attrKey: "", mergeWith: "AccountID"},
	{name: "LedgerID", attrKey: "app.request.payload.has_ledger_id"},
	{name: "LedgerId", attrKey: "", mergeWith: "LedgerID"},
	{name: "OrganizationID", attrKey: "app.request.payload.has_organization_id"},
	{name: "OrganizationId", attrKey: "", mergeWith: "OrganizationID"},
	{name: "TransactionID", attrKey: "app.request.payload.has_transaction_id"},
	{name: "TransactionId", attrKey: "", mergeWith: "TransactionID"},
	{name: "ParentTransactionID", attrKey: "app.request.payload.has_parent_transaction_id"},
	{name: "ParentTransactionId", attrKey: "", mergeWith: "ParentTransactionID"},
	{name: "Document", attrKey: "app.request.payload.has_document"},
	{name: "Send", attrKey: "app.request.payload.has_send"},
	{name: "Source", attrKey: "app.request.payload.has_source"},
	{name: "Distribution", attrKey: "app.request.payload.has_distribution"},
	{name: "Account", attrKey: "app.request.payload.has_account_rule"},
	{name: "ValidIf", attrKey: "app.request.payload.has_valid_if"},
}

// HandleSpanByErrorClass records err onto span using the helper appropriate to
// the error's class: business/4xx errors keep the span status green via
// HandleSpanBusinessErrorEvent; technical/5xx errors flip it red via
// HandleSpanError. Use it at the handler boundary for errors returned from
// use cases, where the class is not known statically.
func HandleSpanByErrorClass(span trace.Span, message string, err error) {
	if pkg.IsBusinessError(err) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)

		return
	}

	libOpentelemetry.HandleSpanError(span, message, err)
}

// RecordSafePayloadAttributes records the presence-only shape of a request
// payload on span: which known fields are set, never their values.
func RecordSafePayloadAttributes(span trace.Span, payload any) {
	if span == nil {
		return
	}

	span.SetAttributes(SafePayloadAttributes(payload)...)
}

// SafePayloadAttributes builds the presence-only attributes for a payload: the
// resolved struct type name plus one boolean per known field.
func SafePayloadAttributes(payload any) []attribute.KeyValue {
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

// resolvePayloadValue dereferences the payload through any pointer chain and
// returns the underlying struct reflect.Value.  If the payload is nil, not a
// pointer-to-struct, or a nil pointer, the returned value is invalid
// (value.IsValid() == false).
func resolvePayloadValue(payload any) reflect.Value {
	v := reflect.ValueOf(payload)
	if !v.IsValid() {
		return v
	}

	for v.Kind() == reflect.Pointer {
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
