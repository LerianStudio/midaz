// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"strings"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// datasourceFilters is the concrete shape the host stamps onto
// ExtractionRequest.Filters[configName]: per qualified table (dot-notation
// "schema.table" for postgres, bare "collection" for mongo) a map of field name
// to its FilterCondition. The engine contract types Filters as map[string]any
// so the adapter owns interpretation; this is that interpretation.
type datasourceFilters map[string]map[string]model.FilterCondition

// filtersForDatasource extracts the per-datasource filter tree for this
// connector's config name. A missing or nil entry yields nil filters and no
// error (the unfiltered case). A present-but-wrong-typed entry is a loud
// CategoryValidation error rather than a silent full-table read: a financial
// reporter must never widen scope because a filter payload was mis-shaped.
//
// It accepts two concrete shapes, because the engine reaches a connector by two
// paths that carry filters differently:
//
//   - the DIRECT path (the adapter's own QueryStream tests) stamps the named
//     datasourceFilters value verbatim;
//   - the PLAN path (PlanExtraction -> ExecuteExtraction) round-trips filters
//     through the planner and runner, which widen them to a nested
//     map[string]any (table -> field -> model.FilterCondition value). The named
//     type is erased on that trip, so the connector must structurally decode it
//     back.
//
// Both decode to the same datasourceFilters the cursors consume.
func filtersForDatasource(configName string, raw map[string]any) (datasourceFilters, error) {
	entry, ok := raw[configName]
	if !ok || entry == nil {
		return nil, nil
	}

	if filters, ok := entry.(datasourceFilters); ok {
		return filters, nil
	}

	tables, ok := entry.(map[string]any)
	if !ok {
		return nil, NewEngineValidationError("extraction filters for datasource " + configName + " have an unexpected shape")
	}

	return decodeDatasourceFilters(configName, tables)
}

// decodeDatasourceFilters rebuilds the named datasourceFilters from the nested
// map[string]any shape the planner/runner produce. Each table value must be a
// map[string]any of field -> model.FilterCondition; any other shape is a loud
// validation error so a mis-shaped payload never silently widens the result.
func decodeDatasourceFilters(configName string, tables map[string]any) (datasourceFilters, error) {
	out := make(datasourceFilters, len(tables))

	for table, tableRaw := range tables {
		fields, ok := tableRaw.(map[string]any)
		if !ok {
			return nil, NewEngineValidationError("extraction filters for datasource " + configName + " table " + table + " have an unexpected shape")
		}

		conditions := make(map[string]model.FilterCondition, len(fields))

		for field, valueRaw := range fields {
			condition, ok := valueRaw.(model.FilterCondition)
			if !ok {
				return nil, NewEngineValidationError("extraction filter for datasource " + configName + " field " + field + " has an unexpected shape")
			}

			conditions[field] = condition
		}

		out[table] = conditions
	}

	return out, nil
}

// tableFilters returns the field->condition map for a qualified table, matching
// the table key in the same multi-format way the legacy worker's getTableFilters
// did: it tries the exact qualified key ("schema.table" / "database.collection")
// first, then the bare table/collection name. A table with no filters yields nil.
func (f datasourceFilters) tableFilters(qualified string) map[string]model.FilterCondition {
	if f == nil {
		return nil
	}

	if conditions, ok := f[qualified]; ok {
		return conditions
	}

	if _, table := splitQualified(qualified); table != qualified {
		if conditions, ok := f[table]; ok {
			return conditions
		}
	}

	return nil
}

// validColumnSet returns the set of column/field names known for a qualified
// table in the snapshot, used to reject filter field references that are not in
// the discovered schema (the resolveValidColumns equivalent). An unknown table
// yields an empty set, so every filter field for it is rejected.
func validColumnSet(snapshot fetcher.SchemaSnapshot, qualified string) map[string]struct{} {
	cols := make(map[string]struct{})

	for _, table := range snapshot.Tables {
		if table.Name != qualified {
			continue
		}

		for _, field := range table.Fields {
			cols[field] = struct{}{}
		}

		break
	}

	return cols
}

// collectionInSnapshot reports whether a qualified table/collection is present
// in the discovered schema at all, independent of how many fields it has. It
// distinguishes an existing-but-empty mongo collection (present, zero fields)
// from a genuinely missing one (absent), so the empty-collection short-circuit
// only relaxes field validation for collections that really exist.
func collectionInSnapshot(snapshot fetcher.SchemaSnapshot, qualified string) bool {
	for _, table := range snapshot.Tables {
		if table.Name == qualified {
			return true
		}
	}

	return false
}

// rootField reduces a dotted field reference to its root column, mirroring the
// legacy validation: a nested JSONB/document path is validated against its root
// column name.
func rootField(field string) string {
	if dot := strings.Index(field, "."); dot != -1 {
		return field[:dot]
	}

	return field
}

// validateFilterField rejects a filter field whose full string is not a safe
// dotted identifier. The connector validation gates check only the ROOT column
// against the discovered schema, but the FULL field string is used verbatim as a
// squirrel map key (emitted unquoted) — so a dotted path carrying SQL escapes
// would pass the root check yet inject at the sink. It delegates the charset
// whitelist to model.ValidateFieldName (the single source of truth shared with
// the mongo connector) and re-wraps any rejection as the boundary's
// CategoryValidation engine error so the contract stays unchanged.
func validateFilterField(field string) error {
	if err := model.ValidateFieldName(field); err != nil {
		return NewEngineValidationError(err.Error())
	}

	return nil
}

// applyDateRangeUpperBound expands a date-only upper bound to end-of-day
// (T23:59:59.999Z) so a "between" on a date field is inclusive of the whole end
// day, matching pkg/reporter/postgres applyAdvancedFilter. Expansion requires
// BOTH range ends to be date strings — the legacy builder gated on
// isDateString(start) && isDateString(end), so a non-date start (e.g. a number
// or timestamp) must leave the upper bound untouched. It returns the end value
// unchanged for non-date fields, non-date-string ends, or non-date starts.
func applyDateRangeUpperBound(field string, start, end any) any {
	if !isDateField(field) || !isDateString(start) || !isDateString(end) {
		return end
	}

	str, ok := end.(string)
	if !ok || len(str) != constant.DateOnlyStringLength {
		return end
	}

	return str + "T23:59:59.999Z"
}

// validateFilterCondition rejects malformed FilterCondition arities the same way
// the legacy builders (pkg/reporter/postgres and pkg/reporter/mongodb
// validateFilterCondition) did, returning a CategoryValidation engine error
// instead of silently applying a narrower-or-wider filter. Without this guard a
// Between with a length other than 2 is silently dropped (the field goes
// unfiltered, widening the row set) and a single-value operator carrying extra
// values silently uses only the first. A financial reporter must fail closed on
// a mis-shaped condition, not return a different row set.
func validateFilterCondition(field string, condition model.FilterCondition) error {
	if len(condition.Between) > 0 && len(condition.Between) != constant.BetweenOperatorValues {
		return NewEngineValidationError("between operator for field " + field + " must have exactly 2 values")
	}

	singleValueOps := [][]any{
		condition.GreaterThan,
		condition.GreaterOrEqual,
		condition.LessThan,
		condition.LessOrEqual,
	}
	for _, values := range singleValueOps {
		if len(values) > 1 {
			return NewEngineValidationError("single-value operator for field " + field + " must have exactly 1 value")
		}
	}

	return nil
}

// dateFormats are the layouts attempted by isDateString, mirroring
// pkg/reporter/postgres datasource_field_patterns.go. They are mirrored here
// (not imported) because that package pulls in the pgx driver, which the engine
// adapter must not depend on.
var dateFormats = []string{
	"2006-01-02",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05Z07:00",
	time.RFC3339,
	time.RFC3339Nano,
}

// isDateField reports whether a field name denotes a temporal column, mirroring
// the legacy reporter heuristic used to decide date-range upper-bound expansion.
func isDateField(fieldName string) bool {
	fieldLower := strings.ToLower(fieldName)

	for _, suffix := range []string{"_at", "_date", "_time"} {
		if strings.HasSuffix(fieldLower, suffix) {
			return true
		}
	}

	for _, name := range []string{"date", "time", "timestamp", "created", "updated", "deleted"} {
		if fieldLower == name {
			return true
		}
	}

	return false
}

// isDateString reports whether a value is a string parseable as one of the
// supported date layouts, mirroring the legacy reporter heuristic.
func isDateString(value any) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	for _, layout := range dateFormats {
		if _, err := time.Parse(layout, str); err == nil {
			return true
		}
	}

	return false
}
