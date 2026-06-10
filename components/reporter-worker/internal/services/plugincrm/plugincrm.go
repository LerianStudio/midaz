// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package plugincrm reproduces the plugin_crm-specific report extraction
// behavior that the generic embedded engine path does not provide: the
// hash-based advanced-filter rewrite applied before extraction, the
// organization-scoped collection fan-out applied during extraction, and the
// field decryption applied after extraction.
//
// It exists as a self-contained stage so the generate-report handler can wire
// two seams around the engine:
//
//	(1) PRE-FILTER  — TransformFilters rewrites plugin_crm field-name filters to
//	    their hashed search.* equivalents before the filters enter the engine's
//	    ExtractionRequest.Filters.
//	(2) POST-EXTRACTION — FanOutOrgCollections + DecryptRecords reproduce the
//	    holders_* org fan-out with organization_id injection and the field
//	    decryption the legacy worker applied to plugin_crm result rows.
//
// The crypto keys are passed in by the caller exactly as the legacy worker
// sourced them (UseCase.CryptoHashSecretKeyPluginCRM /
// UseCase.CryptoEncryptSecretKeyPluginCRM); this package neither holds nor
// relocates secrets. It never logs decrypted values, secrets, hashes, or PII.
package plugincrm

import (
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/lib-observability/log"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// DatasourceName is the logical datasource name that selects the plugin_crm
	// extraction path. It mirrors the literal the legacy worker matched on
	// (databaseName == "plugin_crm"); plugin_crm-ness is determined exactly this
	// way and no new predicate is introduced.
	DatasourceName = "plugin_crm"

	// OrganizationCollection is the synthetic collection name that carries the
	// organization_id into template context. It is not a queryable collection;
	// the legacy worker skipped it, and IsQueryableCollection reproduces that.
	OrganizationCollection = "organization"
)

// advancedFilterFieldMappings maps a plugin_crm encrypted/logical field name to
// its hashed search.* counterpart. A filter on an encrypted field cannot match
// stored ciphertext directly; the plugin stores an HMAC of the plaintext under a
// search.* field, so a filter must be rewritten to that field with hashed
// values. This is a verbatim copy of the legacy transformPluginCRMAdvancedFilters
// mapping — the wire contract with the plugin's stored search index.
var advancedFilterFieldMappings = map[string]string{
	"document":                               "search.document",
	"name":                                   "search.name",
	"banking_details.account":                "search.banking_details_account",
	"banking_details.iban":                   "search.banking_details_iban",
	"contact.primary_email":                  "search.contact_primary_email",
	"contact.secondary_email":                "search.contact_secondary_email",
	"contact.mobile_phone":                   "search.contact_mobile_phone",
	"contact.other_phone":                    "search.contact_other_phone",
	"regulatory_fields.participant_document": "search.regulatory_fields_participant_document",
	"related_parties.document":               "search.related_party_documents",
}

// Is reports whether a datasource name selects the plugin_crm extraction path.
// It is the single predicate the handler uses to decide whether to apply this
// stage, mirroring the legacy databaseName == "plugin_crm" check.
func Is(datasourceName string) bool {
	return datasourceName == DatasourceName
}

// IsQueryableCollection reports whether a plugin_crm collection should be
// extracted. The "organization" collection is metadata (it carries the
// organization_id for template context), not a physical collection, so the
// legacy worker skipped it; this reproduces that decision.
func IsQueryableCollection(collection string) bool {
	return collection != OrganizationCollection
}

// TransformFilters rewrites a single collection's advanced filters for the
// plugin_crm search index. Mapped field names are replaced with their hashed
// search.* counterparts and their string values are HMAC-hashed with the
// configured hash key; unmapped fields are passed through unchanged. It
// reproduces the legacy transformPluginCRMAdvancedFilters exactly.
//
// A nil filter map yields nil with no error (the unfiltered case). An empty
// hashKey is a fail-closed precondition error: hashing with an empty HMAC key
// produces an insecure, non-matching hash, so a filtered plugin_crm extraction
// must not proceed.
//
// The hashKey is supplied by the caller (UseCase.CryptoHashSecretKeyPluginCRM);
// this function neither holds nor logs it.
func TransformFilters(filter map[string]model.FilterCondition, hashKey string, logger log.Logger) (map[string]model.FilterCondition, error) {
	if filter == nil {
		return nil, nil
	}

	if hashKey == "" {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeCRMHashKeyNotConfigured.Error(),
			Title:   "CRM Crypto Not Configured",
			Message: "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM not configured",
		}
	}

	crypto := &libCrypto.Crypto{
		HashSecretKey: hashKey,
		Logger:        logger,
	}

	transformed := make(map[string]model.FilterCondition, len(filter))

	for fieldName, condition := range filter {
		searchField, mapped := advancedFilterFieldMappings[fieldName]
		if !mapped {
			// Keep non-mapped fields as-is.
			transformed[fieldName] = condition
			continue
		}

		transformed[searchField] = hashCondition(condition, crypto)
	}

	return transformed, nil
}

// hashCondition rewrites every populated operator's values to their hashed
// equivalents, mirroring the per-operator hashing the legacy
// transformPluginCRMAdvancedFilters performed. An operator left empty in the
// source stays empty in the result.
func hashCondition(condition model.FilterCondition, crypto *libCrypto.Crypto) model.FilterCondition {
	transformed := model.FilterCondition{}

	if len(condition.Equals) > 0 {
		transformed.Equals = hashValues(condition.Equals, crypto)
	}

	if len(condition.GreaterThan) > 0 {
		transformed.GreaterThan = hashValues(condition.GreaterThan, crypto)
	}

	if len(condition.GreaterOrEqual) > 0 {
		transformed.GreaterOrEqual = hashValues(condition.GreaterOrEqual, crypto)
	}

	if len(condition.LessThan) > 0 {
		transformed.LessThan = hashValues(condition.LessThan, crypto)
	}

	if len(condition.LessOrEqual) > 0 {
		transformed.LessOrEqual = hashValues(condition.LessOrEqual, crypto)
	}

	if len(condition.Between) > 0 {
		transformed.Between = hashValues(condition.Between, crypto)
	}

	if len(condition.In) > 0 {
		transformed.In = hashValues(condition.In, crypto)
	}

	if len(condition.NotIn) > 0 {
		transformed.NotIn = hashValues(condition.NotIn, crypto)
	}

	return transformed
}

// hashValues hashes the non-empty string values in a filter operator's value
// slice, leaving non-string and empty-string values untouched. It mirrors the
// legacy hashFilterValues.
func hashValues(values []any, crypto *libCrypto.Crypto) []any {
	hashed := make([]any, len(values))

	for i, value := range values {
		if strValue, ok := value.(string); ok && strValue != "" {
			hashed[i] = crypto.GenerateHash(&strValue)
			continue
		}

		hashed[i] = value
	}

	return hashed
}
