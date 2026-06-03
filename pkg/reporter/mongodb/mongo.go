// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"sort"
	"strings"
)

// FilterNestedFields removes nested fields when any ancestor field is already in the list.
// MongoDB projections cannot have both a parent field and its nested fields simultaneously.
// For example, if "related_parties" is in the list, "related_parties.document" should be removed.
// This also handles deeply nested cases: if "contact.address" exists, "contact.address.city" is removed.
// This prevents MongoDB projection conflicts and Pongo2 path collision errors.
func FilterNestedFields(fields []string) []string {
	if len(fields) == 0 {
		return fields
	}

	// Build a set of all fields for ancestor lookup
	fieldSet := make(map[string]struct{})
	for _, field := range fields {
		fieldSet[field] = struct{}{}
	}

	// Filter out fields that have any ancestor already in the list
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if hasAncestor(field, fieldSet) {
			// Skip this field, an ancestor already includes it
			continue
		}

		result = append(result, field)
	}

	// Sort for consistent ordering
	sort.Strings(result)

	return result
}

// hasAncestor checks if any prefix path of field exists in fieldSet.
// For "contact.address.city", it checks if "contact" or "contact.address" exists.
func hasAncestor(field string, fieldSet map[string]struct{}) bool {
	parts := strings.Split(field, ".")
	// Check all prefixes except the full field itself
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], ".")
		if _, exists := fieldSet[ancestor]; exists {
			return true
		}
	}

	return false
}

// ValidateFieldsInSchemaMongo validate if all fields exist on mongo DB collection
// For nested fields (e.g., "related_parties.document"), it validates that the parent field exists
// since MongoDB is schemaless and nested structures cannot be fully validated at schema level
func ValidateFieldsInSchemaMongo(expectedFields []string, schema CollectionSchema, countIfTableExist *int32) (missing []string) {
	columnSet := make(map[string]struct{}, len(schema.Fields))
	for _, col := range schema.Fields {
		columnSet[strings.ToLower(col.Name)] = struct{}{}
	}

	for _, field := range expectedFields {
		*countIfTableExist++

		fieldLower := strings.ToLower(field)

		// Check if field exists directly
		if _, exists := columnSet[fieldLower]; exists {
			continue
		}

		// For nested fields (e.g., "related_parties.document"), check if parent field exists
		// MongoDB is schemaless, so if the parent field exists, we trust the nested structure
		if strings.Contains(field, ".") {
			parentField := strings.ToLower(strings.Split(field, ".")[0])
			if _, parentExists := columnSet[parentField]; parentExists {
				continue
			}
		}

		missing = append(missing, field)
	}

	return
}
