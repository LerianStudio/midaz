// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package templateutils

import (
	"regexp"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
)

// The two reporter-specific filters that synthesize a field at render time from
// a real source column. ONLY these two filter names trigger the field-map
// reconciliation below — no other filter is affected.
const (
	derivedFilterAddDay    = "add_day"    // {{ coll|add_day:"created_at" }}    -> injects "dia"
	derivedFilterAddSigned = "add_signed" // {{ coll|add_signed:"amount" }}     -> injects "signed"

	derivedOutputDay    = "dia"
	derivedOutputSigned = "signed"
)

var (
	addDayArgRegex    = regexp.MustCompile(`add_day\s*:\s*"([^"]+)"`)
	addSignedArgRegex = regexp.MustCompile(`add_signed\s*:\s*"([^"]+)"`)
	withAssignRegex   = regexp.MustCompile(`{%-?\s*with\s+[A-Za-z_]\w*\s*=\s*(.+?)\s*-?%}`)
	inlineFieldRegex  = regexp.MustCompile(`{{\s*(.*?)\s*}}`)
)

// rewriteDerivedFilterFields reconciles the extracted datasource field map with
// the add_day / add_signed filters. These synthesize a field at render time, so
// the auto-extractor wrongly attributes the synthetic output (dia / signed) to
// the datasource table while missing the real source column the filter reads.
//
// For every collection a recognized filter is applied to, this:
//   - adds the source column (the filter's quoted argument, e.g. created_at /
//     amount) to that table's field list so the worker SELECTs it; and
//   - removes the synthetic output (dia / signed) so the worker does NOT try to
//     SELECT a column that does not exist in the datasource.
//
// It is a no-op for templates that use neither filter, and it never touches
// tables that no recognized filter was applied to.
func rewriteDerivedFilterFields(result map[string]map[string][]string, templateFile string) {
	if !strings.Contains(templateFile, derivedFilterAddDay+":") &&
		!strings.Contains(templateFile, derivedFilterAddSigned+":") {
		return
	}

	for _, expr := range collectFilterableExpressions(templateFile) {
		dayArgs := addDayArgRegex.FindAllStringSubmatch(expr, -1)
		signedArgs := addSignedArgRegex.FindAllStringSubmatch(expr, -1)

		if len(dayArgs) == 0 && len(signedArgs) == 0 {
			continue
		}

		// The collection the filters are applied to is the path before the first pipe.
		parts := CleanPath(strings.TrimSpace(strings.Split(expr, "|")[0]))
		if len(parts) < constant.MinPathParts {
			continue
		}

		database, table := parts[0], parts[1]

		tables, ok := result[database]
		if !ok {
			continue
		}

		if _, ok := tables[table]; !ok {
			continue
		}

		for _, m := range dayArgs {
			tables[table] = appendUniqueField(tables[table], m[1])
		}

		if len(dayArgs) > 0 {
			tables[table] = removeFieldValue(tables[table], derivedOutputDay)
		}

		for _, m := range signedArgs {
			tables[table] = appendUniqueField(tables[table], m[1])
		}

		if len(signedArgs) > 0 {
			tables[table] = removeFieldValue(tables[table], derivedOutputSigned)
		}
	}
}

// collectFilterableExpressions returns the candidate expressions where a filter
// pipeline may live: the right-hand side of `{% with x = ... %}` assignments and
// inline `{{ ... }}` expressions.
func collectFilterableExpressions(templateFile string) []string {
	withMatches := withAssignRegex.FindAllStringSubmatch(templateFile, -1)
	inlineMatches := inlineFieldRegex.FindAllStringSubmatch(templateFile, -1)

	exprs := make([]string, 0, len(withMatches)+len(inlineMatches))

	for _, m := range withMatches {
		exprs = append(exprs, m[1])
	}

	for _, m := range inlineMatches {
		exprs = append(exprs, m[1])
	}

	return exprs
}

// appendUniqueField appends field unless it is already present.
func appendUniqueField(fields []string, field string) []string {
	for _, f := range fields {
		if f == field {
			return fields
		}
	}

	return append(fields, field)
}

// removeFieldValue returns a copy of fields with every occurrence of field removed.
func removeFieldValue(fields []string, field string) []string {
	out := make([]string, 0, len(fields))

	for _, f := range fields {
		if f != field {
			out = append(out, f)
		}
	}

	return out
}
