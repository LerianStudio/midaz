// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package templateutils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/LerianStudio/reporter/pkg/constant"
)

// GetMimeType return a MIME type correctly based with outputFormat
func GetMimeType(outputFormat string) string {
	switch strings.ToLower(outputFormat) {
	case "xml":
		return "application/xml"
	case "html":
		return "text/html"
	case "csv":
		return "text/csv"
	case "txt":
		return "text/plain"
	case "pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// MappedFieldsOfTemplate analyzes a template file and returns a nested map of variable paths and their associated fields.
func MappedFieldsOfTemplate(templateFile string) map[string]map[string][]string {
	variableMap := regexBlockForOnPlaceholder(templateFile)
	resultRegex := regexBlockWithOnPlaceholder(variableMap, templateFile)
	regexBlockForWithFilterOnPlaceholder(resultRegex, variableMap, templateFile)

	// Re-resolve nested variables after with/filter blocks registered their variables.
	// For loops parsed first may reference with-variables that didn't exist yet
	// (e.g., "for operation in operations" where "operations" comes from a with block).
	resolveNestedVariables(variableMap)

	// Regex for fields {{ ... }}
	fieldRegex := regexp.MustCompile(`{{\s*(.*?)\s*}}`)

	fieldMatches := fieldRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range fieldMatches {
		expr := match[1]

		// Skip expressions that contain DIMP filters - they will be processed by regexBlockDIMPFiltersOnPlaceholder
		if strings.Contains(expr, "|where:") || strings.Contains(expr, "|sum:") || strings.Contains(expr, "|count:") {
			// For DIMP filter expressions, only extract the base path fields (before the pipe)
			// The filter fields will be extracted by regexBlockDIMPFiltersOnPlaceholder
			basePart := strings.Split(expr, "|")[0]
			basePart = strings.TrimSpace(basePart)

			// Don't process if it's just a collection path (will be handled by DIMP function)
			basePathParts := CleanPath(basePart)
			if len(basePathParts) == constant.MinPathParts {
				// This is just datasource.collection, skip it - DIMP function will handle
				continue
			}
		}

		// For expressions with arithmetic operators (e.g., "6 + plugin_crm.holders|length"),
		// use regex-based extraction that correctly isolates dotted field paths
		// instead of the pipe-splitting logic that misinterprets "6 + plugin_crm.holders" as a path.
		if containsArithmeticOperator(expr) {
			registerArithmeticFieldPaths(expr, resultRegex, variableMap)
			continue
		}

		fieldPaths := extractFieldsFromExpression(expr)

		for _, fieldExpr := range fieldPaths {
			parts := CleanPath(fieldExpr)
			if len(parts) < constant.MinPathParts {
				continue
			}

			if loopPath, ok := variableMap[parts[0]]; ok {
				fullPath := append([]string{}, loopPath...)
				insertField(resultRegex, fullPath, parts[1])
			} else {
				insertField(resultRegex, parts[:len(parts)-1], parts[len(parts)-1])
			}
		}
	}

	regexBlockIfOnPlaceholder(templateFile, resultRegex, variableMap)
	regexBlockSetOnPlaceholder(templateFile, resultRegex, variableMap)
	regexBlockLastItemByGroupOnPlaceholder(templateFile, resultRegex, variableMap)
	regexBlockAggregationBlocksOnPlaceholder(templateFile, resultRegex, variableMap)
	regexBlockCalcOnPlaceholder(templateFile, resultRegex, variableMap)
	regexBlockDIMPFiltersOnPlaceholder(templateFile, resultRegex, variableMap)

	result := normalizeStructure(resultRegex)

	// Reconcile the two derived-field filters (add_day / add_signed) with the
	// datasource projection. Scoped strictly to those two filters by name.
	rewriteDerivedFilterFields(result, templateFile)

	// Drop phantom datasources: template-local variables (e.g. the `as` result
	// var of last_item_by_group) can surface as a top-level key with no tables.
	// They are not real datasources, carry no fields, and break downstream
	// consumers such as the fetcher (FET-0401).
	pruneEmptyDatasources(result)

	return result
}

// pruneEmptyDatasources removes top-level entries that contain no tables. Such
// entries originate from template-local aliases mistaken for datasources and
// never correspond to data that can be fetched.
func pruneEmptyDatasources(result map[string]map[string][]string) {
	for datasource, tables := range result {
		if len(tables) == 0 {
			delete(result, datasource)
		}
	}
}

// regexBlockIfOnPlaceholder parses a template file to process "if" blocks and updates a nested map with extracted field mappings.
// It identifies fields used in conditional statements, cleans their paths, and inserts them into the resultRegex map structure.
func regexBlockIfOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	ifRegex := regexp.MustCompile(`{%-?\s*if\s+(.*?)\s*-?%}`)

	ifMatches := ifRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range ifMatches {
		expr := match[1]
		fieldPaths := extractIfFromExpression(expr)

		for _, fieldExpr := range fieldPaths {
			parts := CleanPath(fieldExpr)
			if len(parts) < constant.MinPathParts {
				continue
			}

			if loopPath, ok := variableMap[parts[0]]; ok {
				insertField(resultRegex, loopPath, parts[1])
			} else {
				// Skip if the field already exists as a table (map) in the result.
				// e.g., {% if midaz_onboarding.account %} should not overwrite
				// the account table that already has fields from {{ a.id }}.
				if isExistingTable(resultRegex, parts[:len(parts)-1], parts[len(parts)-1]) {
					continue
				}

				insertField(resultRegex, parts[:len(parts)-1], parts[len(parts)-1])
			}
		}
	}
}

// regexBlockIfOnPlaceholder parses a template file to process "if" blocks and updates a nested map with extracted field mappings.
// It identifies fields used in conditional statements, cleans their paths, and inserts them into the resultRegex map structure.
func regexBlockSetOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	setRegex := regexp.MustCompile(`{%-?\s*set\s+(.*?)\s*-?%}`)

	setMatches := setRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range setMatches {
		expr := match[1]
		fieldPaths := extractIfFromExpression(expr)

		for _, fieldExpr := range fieldPaths {
			parts := CleanPath(fieldExpr)
			if len(parts) < constant.MinPathParts {
				continue
			}

			if loopPath, ok := variableMap[parts[0]]; ok {
				insertField(resultRegex, loopPath, parts[1])
			} else {
				insertField(resultRegex, parts[:len(parts)-1], parts[len(parts)-1])
			}
		}
	}
}

// regexBlockLastItemByGroupOnPlaceholder parses a template file to process "last_item_by_group" blocks.
// It extracts the collection path, group_by field, order_by field, optional if-condition fields,
// and registers the "as" variable in variableMap so subsequent tags can reference the result.
func regexBlockLastItemByGroupOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	tagRegex := regexp.MustCompile(`{%-?\s*last_item_by_group\s+(.*?)\s*-?%}`)

	tagMatches := tagRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range tagMatches {
		expr := match[1]

		// Parse: <collection> group_by "<field>" order_by "<field>" [if <condition>] as <var>
		partsRegex := regexp.MustCompile(`^(\S+)\s+group_by\s+"([^"]+)"\s+order_by\s+"([^"]+)"(?:\s+if\s+(.+?))?\s+as\s+(\w+)$`)
		parts := partsRegex.FindStringSubmatch(strings.TrimSpace(expr))

		if len(parts) == 0 {
			continue
		}

		collectionPath := parts[1] // e.g., "midaz_transaction.operation"
		groupByField := parts[2]   // e.g., "account_id"
		orderByField := parts[3]   // e.g., "created_at"
		ifCondition := parts[4]    // e.g., "route" or "type == \"CREDIT\"" (may be empty)
		asVarName := parts[5]      // e.g., "listaOperations"

		// Clean the collection path
		mainPath := CleanPath(collectionPath)
		if len(mainPath) < constant.MinPathParts {
			continue
		}

		// Register the "as" variable in variableMap pointing to the collection path
		variableMap[asVarName] = mainPath[:constant.MinPathParts]

		// Insert the group_by field(s) - supports comma-separated composite keys
		for _, gf := range strings.Split(groupByField, ",") {
			insertField(resultRegex, mainPath[:constant.MinPathParts], strings.TrimSpace(gf))
		}

		// Insert the order_by field
		insertField(resultRegex, mainPath[:constant.MinPathParts], orderByField)

		// Process if-condition fields (if present)
		if ifCondition != "" {
			insertConditionFields(ifCondition, resultRegex, mainPath[:constant.MinPathParts], variableMap)
		}
	}
}

// insertConditionFields extracts field references from an if-condition expression
// and inserts them into the result regex map.
func insertConditionFields(ifCondition string, resultRegex map[string]any, basePath []string, variableMap map[string][]string) {
	conditionFields := extractIfFromExpression(ifCondition)
	if len(conditionFields) > 0 {
		for _, fieldExpr := range conditionFields {
			condParts := CleanPath(fieldExpr)
			if len(condParts) < constant.MinPathParts {
				insertField(resultRegex, basePath, fieldExpr)
			} else if loopPath, ok := variableMap[condParts[0]]; ok {
				insertField(resultRegex, loopPath, condParts[1])
			} else {
				insertField(resultRegex, condParts[:len(condParts)-1], condParts[len(condParts)-1])
			}
		}

		return
	}

	// extractIfFromExpression only returns dotted paths (a.b).
	// For simple field names (e.g., "route"), extract identifiers directly.
	simpleFieldRegex := regexp.MustCompile(`\b([a-zA-Z_]\w*)\b`)

	simpleMatches := simpleFieldRegex.FindAllString(ifCondition, -1)
	for _, field := range simpleMatches {
		if isConditionKeyword(field) {
			continue
		}

		insertField(resultRegex, basePath, field)
	}
}

// regexBlockAggregationBlocksOnPlaceholder parses a template file to process aggregation blocks
// (count_by, sum_by, avg_by, min_by, max_by) and updates a nested map with extracted field mappings.
func regexBlockAggregationBlocksOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	aggrRegexes := []*regexp.Regexp{
		regexp.MustCompile(`{%-?\s*count_by\s+(.*?)\s*-?%}`),
		regexp.MustCompile(`{%-?\s*sum_by\s+(.*?)\s*-?%}`),
		regexp.MustCompile(`{%-?\s*avg_by\s+(.*?)\s*-?%}`),
		regexp.MustCompile(`{%-?\s*min_by\s+(.*?)\s*-?%}`),
		regexp.MustCompile(`{%-?\s*max_by\s+(.*?)\s*-?%}`),
	}

	matches := make([][]string, 0, len(aggrRegexes))
	for _, re := range aggrRegexes {
		matches = append(matches, re.FindAllStringSubmatch(templateFile, -1)...)
	}

	for _, match := range matches {
		expr := match[1]

		args := extractFieldsFromExpressionOfAggregation(expr)
		if len(args) == 0 {
			continue
		}

		rawMain := strings.TrimSpace(args[0])
		mainPath := CleanPath(rawMain)

		// If the collection reference is a single identifier, resolve it from variableMap
		// (e.g., "listaOperations" registered by last_item_by_group)
		if len(mainPath) < constant.MinPathParts {
			if resolved, ok := variableMap[rawMain]; ok && len(resolved) >= constant.MinPathParts {
				mainPath = resolved
			} else {
				continue
			}
		}

		variableMap[mainPath[1]] = mainPath

		// Detect if this is a "by" expression using parity:
		// - With "by": mainPath(1) + byField(1) + conditionPairs(2n) = even number
		// - Without "by": mainPath(1) + conditionPairs(2n) = odd number
		// In "by" expressions, args[1] is a nested JSON field path (e.g., "fee_charge.totalAmount"),
		// NOT a datasource reference. It should be preserved as-is.
		hasByClause := len(args) >= constant.MinByClauseArgs && len(args)%2 == 0

		for i, arg := range args[1:] {
			// Skip quoted string literals (values like "cacc", 'value', etc.)
			trimmedArg := strings.TrimSpace(arg)
			if isQuotedString(trimmedArg) {
				continue
			}

			// If this is the "by" field (first arg after mainPath in a "by" expression),
			// it's a nested JSON field path within the collection, not a datasource reference.
			// Insert it directly without CleanPath processing.
			if hasByClause && i == 0 {
				// This is the "by" field (e.g., "fee_charge.totalAmount")
				// It's a nested field path within the main collection
				insertField(resultRegex, mainPath, trimmedArg)
				continue
			}

			argPath := CleanPath(arg)

			switch {
			case len(argPath) < constant.MinPathParts:
				insertField(resultRegex, mainPath, arg)
			case variableMap[argPath[0]] != nil:
				insertField(resultRegex, variableMap[argPath[0]], argPath[1])
			default:
				insertField(resultRegex, argPath[:len(argPath)-1], argPath[len(argPath)-1])
			}
		}
	}
}

// regexBlockDIMPFiltersOnPlaceholder parses a template file to process DIMP filters (where, sum, count)
// and updates the nested map with extracted field mappings.
// It identifies fields used in filter expressions like |where:"field:value", |sum:"field", |count:"field:value"
func regexBlockDIMPFiltersOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	processDIMPExpressions(templateFile, resultRegex, variableMap)
	processDIMPForLoops(templateFile, resultRegex)
}

// processDIMPExpressions processes {{ }} expressions containing DIMP filters
func processDIMPExpressions(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	exprRegex := regexp.MustCompile(`\{\{\s*([^}]+)\s*\}\}`)
	exprMatches := exprRegex.FindAllStringSubmatch(templateFile, -1)

	for _, exprMatch := range exprMatches {
		expr := exprMatch[1]

		if !containsDIMPFilter(expr) {
			continue
		}

		basePath := extractDIMPBasePath(expr, variableMap)
		if basePath == nil {
			continue
		}

		ensureMapStructure(resultRegex, basePath)
		extractFieldsFromDIMPFilters(expr, resultRegex, basePath)
	}
}

// processDIMPForLoops processes for loops containing DIMP filters
// Supports both legacy format (database.table) and explicit schema format (database:schema.table)
func processDIMPForLoops(templateFile string, resultRegex map[string]any) {
	forFilterRegex := regexp.MustCompile(`{%-?\s*for\s+\w+\s+in\s+([a-zA-Z_][\w.:]*)\s*\|\s*(where|sum|count)\s*:\s*"([^"]+)"`)
	forMatches := forFilterRegex.FindAllStringSubmatch(templateFile, -1)

	for _, match := range forMatches {
		collection := match[1]
		filterType := match[2]
		filterArg := match[3]

		collectionParts := CleanPath(collection)
		if len(collectionParts) < constant.MinPathParts {
			continue
		}

		basePath := collectionParts[:2]
		ensureMapStructure(resultRegex, basePath)
		extractFieldFromFilterArg(resultRegex, basePath, filterType, filterArg)
	}
}

// containsDIMPFilter checks if an expression contains DIMP filters
func containsDIMPFilter(expr string) bool {
	return strings.Contains(expr, "|where:") || strings.Contains(expr, "|sum:") || strings.Contains(expr, "|count:")
}

func extractDIMPBasePath(expr string, variableMap map[string][]string) []string {
	parts := strings.Split(expr, "|")
	if len(parts) < constant.MinPathParts {
		return nil
	}

	baseCollection := strings.TrimSpace(parts[0])
	collectionParts := CleanPath(baseCollection)

	if len(collectionParts) == 0 {
		return nil
	}

	if loopPath, ok := variableMap[collectionParts[0]]; ok {
		return loopPath
	}

	if len(collectionParts) >= constant.MinPathParts {
		return collectionParts[:constant.MinPathParts]
	}

	return nil
}

// extractFieldsFromDIMPFilters extracts fields from all DIMP filters in an expression
func extractFieldsFromDIMPFilters(expr string, resultRegex map[string]any, basePath []string) {
	parts := strings.Split(expr, "|")
	filterArgRegex := regexp.MustCompile(`^(where|sum|count)\s*:\s*"([^"]+)"`)

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		filterMatch := filterArgRegex.FindStringSubmatch(part)

		if filterMatch == nil {
			continue
		}

		extractFieldFromFilterArg(resultRegex, basePath, filterMatch[1], filterMatch[2])
	}
}

// extractFieldFromFilterArg extracts a field from a filter argument and inserts it into the result
func extractFieldFromFilterArg(resultRegex map[string]any, basePath []string, filterType, filterArg string) {
	switch filterType {
	case "where", "count":
		colonIdx := strings.Index(filterArg, ":")
		if colonIdx > 0 {
			field := filterArg[:colonIdx]
			insertFieldToPath(resultRegex, basePath, field)
		}
	case "sum":
		field := strings.Trim(filterArg, `"' `)
		if field != "" {
			insertFieldToPath(resultRegex, basePath, field)
		}
	}
}

// ensureMapStructure ensures that the nested map structure exists for the given path
// For path ["datasource", "collection"], it creates: resultRegex["datasource"]["collection"] = []any{}
func ensureMapStructure(m map[string]any, path []string) {
	if len(path) < constant.MinPathParts {
		return
	}

	datasource := path[0]
	collection := path[1]

	// Ensure datasource map exists
	if _, ok := m[datasource]; !ok {
		m[datasource] = map[string]any{}
	}

	// Get or create the datasource map
	dsMap, ok := m[datasource].(map[string]any)
	if !ok {
		dsMap = map[string]any{}
		m[datasource] = dsMap
	}

	// Ensure collection array exists
	if _, ok := dsMap[collection]; !ok {
		dsMap[collection] = []any{}
	}
}

// insertFieldToPath inserts a field into the nested structure at the given path
// For path ["datasource", "collection"] and field "status", it adds "status" to the collection's field list
func insertFieldToPath(m map[string]any, path []string, field string) {
	if len(path) < constant.MinPathParts {
		return
	}

	datasource := path[0]
	collection := path[1]

	// Get the datasource map
	dsMap, ok := m[datasource].(map[string]any)
	if !ok {
		return
	}

	// Get the collection's field list and append
	switch v := dsMap[collection].(type) {
	case []any:
		dsMap[collection] = appendIfMissingAny(v, field)
	case nil:
		dsMap[collection] = []any{field}
	}
}

// regexBlockCalcOnPlaceholder parses a template file to process "calc" blocks and updates a nested map with extracted field mappings.
// It identifies fields used in calculation expressions, cleans their paths, and inserts them into the resultRegex map structure.
func regexBlockCalcOnPlaceholder(templateFile string, resultRegex map[string]any, variableMap map[string][]string) {
	calcRegex := regexp.MustCompile(`{%-?\s*calc\s+(.*?)\s*-?%}`)

	calcMatches := calcRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range calcMatches {
		expr := match[1]
		fieldPaths := extractCalcFromExpression(expr)

		for _, fieldExpr := range fieldPaths {
			parts := CleanPath(fieldExpr)
			if len(parts) < constant.MinPathParts {
				continue
			}

			if loopPath, ok := variableMap[parts[0]]; ok {
				insertField(resultRegex, loopPath, parts[1])
			} else {
				insertField(resultRegex, parts[:len(parts)-1], parts[len(parts)-1])
			}
		}
	}
}

// regexBlockForOnPlaceholder parses a template file to extract variable mappings defined in for-loop blocks.
// It returns a map where keys are variables from the for loop, and values are their corresponding path segments.
// It also handles for loops with filters like: {% for acc in collection|where:"field:value" %}
func regexBlockForOnPlaceholder(templateFile string) map[string][]string {
	variableMap := map[string][]string{}

	// Regex for block for - updated to capture collection with optional filters
	// Matches: {% for var in collection %} or {% for var in collection|filter:"arg" %}
	// Also supports explicit schema syntax: {% for var in database:schema.table %}
	forRegex := regexp.MustCompile(`{%-?\s*for\s+(\w+)\s+in\s+([a-zA-Z_][\w.:]*(?:\s*\|[^%]+)?)\s*-?%}`)

	forMatches := forRegex.FindAllStringSubmatch(templateFile, -1)
	for _, match := range forMatches {
		variable := match[1]
		fullExpr := match[2]

		// Extract base collection path (before any filter)
		// e.g., "midaz_onboarding.account|where:\"type:cacc\"" -> "midaz_onboarding.account"
		basePath := extractBasePathFromFilterExpr(fullExpr)
		path := CleanPath(basePath)

		if len(path) == constant.MinPathParts {
			variableMap[variable] = []string{path[0], path[1]}
		} else if len(path) > constant.MinPathParts {
			variableMap[variable] = []string{path[0], path[1], path[2]}
		} else {
			variableMap[variable] = path
		}
	}

	// Resolve nested variable references (e.g., when inner loop iterates over parent loop variable's field)
	resolveNestedVariables(variableMap)

	return variableMap
}

// resolveNestedVariables resolves nested loop variable references in variableMap.
// When a variable's path starts with another loop variable, it expands the full path.
// Example: if variableMap["alias"] = ["plugin_crm", "aliases"] and
// variableMap["related_party"] = ["alias", "related_parties"], this function
// resolves it to variableMap["related_party"] = ["plugin_crm", "aliases", "related_parties"]
func resolveNestedVariables(variableMap map[string][]string) {
	maxIterations := len(variableMap) // Prevent infinite loops

	for i := 0; i < maxIterations; i++ {
		resolved := true

		for varName, path := range variableMap {
			if len(path) == 0 {
				continue
			}

			// Check if first element of path is another loop variable
			if parentPath, exists := variableMap[path[0]]; exists && path[0] != varName {
				// Expand: replace path[0] with parentPath, keep rest
				newPath := make([]string, 0, len(parentPath)+len(path)-1)
				newPath = append(newPath, parentPath...)
				newPath = append(newPath, path[1:]...)
				variableMap[varName] = newPath
				resolved = false
			}
		}

		if resolved {
			break
		}
	}
}

// extractBasePathFromFilterExpr extracts the base collection path from an expression that may contain filters.
// e.g., "midaz_onboarding.account|where:\"type:cacc\"" -> "midaz_onboarding.account"
func extractBasePathFromFilterExpr(expr string) string {
	// Find the first pipe character (filter separator)
	pipeIdx := strings.Index(expr, "|")
	if pipeIdx > 0 {
		return strings.TrimSpace(expr[:pipeIdx])
	}

	return strings.TrimSpace(expr)
}

// regexBlockForWithFilterOnPlaceholder processes "for" loops with the filter function in a template file, updating nested data structures.
// It extracts variable mappings, assigns paths, and inserts filtered parameter data into the result map.
func regexBlockForWithFilterOnPlaceholder(result map[string]any, variableMap map[string][]string, templateFile string) {
	withRegex := regexp.MustCompile(`{%-?\s*for\s+(\w+)\s*in\s*filter\(\s*([^)]+)\s*\)[^\%]+`)

	withMatches := withRegex.FindAllStringSubmatch(templateFile, -1)

	for _, match := range withMatches {
		assignedVar := match[1]
		args := match[2]
		argParts := strings.Split(args, ",")

		if len(argParts) > 0 {
			filterTarget := strings.TrimSpace(argParts[0])
			path := CleanPath(filterTarget)

			if len(path) >= constant.MinPathParts {
				variableMap[assignedVar] = []string{path[0], path[1]}

				for _, param := range argParts[1:] {
					param = strings.TrimSpace(param)
					cleanParam := strings.Trim(param, `"' `)

					if cleanParam == "" {
						continue
					}

					paramPath := CleanPath(cleanParam)

					if len(paramPath) < constant.MinPathParts {
						insertField(result, path, cleanParam)
						continue
					}

					if loopPath, ok := variableMap[paramPath[0]]; ok {
						insertField(result, loopPath, paramPath[1])
					} else {
						insertField(result, paramPath[:len(paramPath)-1], paramPath[len(paramPath)-1])
					}
				}
			}
		}
	}
}

// regexBlockWithOnPlaceholder parses a template file to process "with" statements and updates `variableMap` with mapped variables.
// The function extracts filters, processes their arguments, and organizes nested data into a structured map for use.
// It cleans paths, maps targets to their corresponding variables, and inserts additional parameters where applicable.
func regexBlockWithOnPlaceholder(variableMap map[string][]string, templateFile string) map[string]any {
	result := map[string]any{}
	withRegex1 := regexp.MustCompile(`{%-?\s*with\s+(\w+)\s*=\s*filter\(\s*([^)]+)\s*\)[^\%]+`)
	withRegex2 := regexp.MustCompile(`{%-?\s*with\s+(\w+)\s*=\s*([^\s%]+)\s*-?%}`)

	withMatches := withRegex1.FindAllStringSubmatch(templateFile, -1)
	withMatches2 := withRegex2.FindAllStringSubmatch(templateFile, -1)

	// Aggregate both sets of matches
	if withMatches2 != nil {
		withMatches = append(withMatches, withMatches2...)
	}

	for _, match := range withMatches {
		assignedVar := match[1]
		args := match[2]
		argParts := strings.Split(args, ",")

		if len(argParts) > 0 {
			filterTarget := strings.TrimSpace(argParts[0])
			path := CleanPath(filterTarget)

			if len(path) >= constant.MinPathParts {
				variableMap[assignedVar] = []string{path[0], path[1]}

				for _, param := range argParts[1:] {
					param = strings.TrimSpace(param)
					cleanParam := strings.Trim(param, `"' `)

					if cleanParam == "" {
						continue
					}

					paramPath := CleanPath(cleanParam)

					if len(paramPath) < constant.MinPathParts {
						insertField(result, path, cleanParam)
						continue
					}

					if loopPath, ok := variableMap[paramPath[0]]; ok {
						insertField(result, loopPath, paramPath[1])
					} else {
						insertField(result, paramPath[:len(paramPath)-1], paramPath[len(paramPath)-1])
					}
				}
			}
		}
	}

	return result
}

// extractFieldsFromExpressionOfAggregation parses an aggregation expression and extracts key fields as a slice of strings.
// Supports compound conditions with "and" operator.
// Examples:
//   - "collection if field == value" -> [collection, field, value]
//   - "collection by "byField" if field == value" -> [collection, byField, field, value]
//   - "collection by "byField" if f1 == v1 and f2 == v2" -> [collection, byField, f1, v1, f2, v2]
func extractFieldsFromExpressionOfAggregation(expr string) []string {
	result := make([]string, 0)

	// Try to match expression with "by" clause
	reWithBy := regexp.MustCompile(`^\s*(\S+)\s+by\s+"([^"]+)"\s+if\s+(.+)$`)
	matchesWithBy := reWithBy.FindStringSubmatch(expr)

	if len(matchesWithBy) == constant.MatchGroupsWithByClause {
		// Has "by" clause: collection, byField, conditions
		result = append(result, matchesWithBy[1], matchesWithBy[2])
		// Extract all fields from conditions (supports "and" compound conditions)
		conditionFields := extractFieldsFromConditions(matchesWithBy[3])
		result = append(result, conditionFields...)

		return result
	}

	// Try to match simple expression without "by" clause
	reSimple := regexp.MustCompile(`^\s*(\S+)\s+if\s+(.+)$`)
	matchesSimple := reSimple.FindStringSubmatch(expr)

	if len(matchesSimple) == constant.MatchGroupsSimple {
		// No "by" clause: collection, conditions
		result = append(result, matchesSimple[1])
		// Extract all fields from conditions
		conditionFields := extractFieldsFromConditions(matchesSimple[2])
		result = append(result, conditionFields...)

		return result
	}

	return result
}

// extractFieldsFromConditions extracts field names and values from condition expressions.
// Supports compound conditions with "and" operator.
// Example: "transfer_type == "CASHIN" and status == "COMPLETED"" -> [transfer_type, "CASHIN", status, "COMPLETED"]
func extractFieldsFromConditions(conditions string) []string {
	result := make([]string, 0)

	// Split by "and" (case insensitive)
	andRegex := regexp.MustCompile(`\s+and\s+`)
	parts := andRegex.Split(conditions, -1)

	// Extract field and value from each condition
	conditionRegex := regexp.MustCompile(`^\s*(\S+)\s*==\s*(\S+)\s*$`)

	for _, part := range parts {
		matches := conditionRegex.FindStringSubmatch(strings.TrimSpace(part))
		if len(matches) == constant.MatchGroupsSimple {
			result = append(result, matches[1], matches[2])
		}
	}

	return result
}

// extractIfFromExpression extracts object.field patterns from a string expression,
// skipping numerical indices like `.0` in midaz_transaction.transaction.0.id.
// Supports both legacy format (database.table.field) and explicit schema format (database:schema.table.field).
func extractIfFromExpression(expr string) []string {
	// Regex: matches paths like:
	// - `foo.bar.baz` (legacy format)
	// - `foo:bar.baz.qux` (explicit schema format - database:schema.table.field)
	// Optionally with `.0` etc., but filters them after
	identifierRegex := regexp.MustCompile(`\b(?:[a-zA-Z_]\w*)(?::[a-zA-Z_]\w*)?(?:\.(?:[a-zA-Z_]\w*|\d+))+\b`)
	matches := identifierRegex.FindAllString(expr, -1)

	var results []string

	for _, match := range matches {
		// For explicit schema syntax, return the full match to be processed by CleanPath
		if strings.Contains(match, ":") {
			results = append(results, match)
			continue
		}

		// Legacy format: split by dot and filter numeric indices
		parts := strings.Split(match, ".")

		var cleaned []string

		for _, part := range parts {
			// Skip purely numeric parts like "0"
			if _, err := strconv.Atoi(part); err == nil {
				continue
			}

			cleaned = append(cleaned, part)
		}

		if len(cleaned) > 1 {
			results = append(results, strings.Join(cleaned, "."))
		}
	}

	return results
}

// extractCalcFromExpression extracts object.field patterns from a calculation expression
func extractCalcFromExpression(expr string) []string {
	return extractIfFromExpression(expr)
}

// normalizeStructure convert input to a type pattern of mapped fields map[string]map[string][]string
func normalizeStructure(input map[string]any) map[string]map[string][]string {
	result := make(map[string]map[string][]string)

	for topKey, topVal := range input {
		section := make(map[string][]string)

		if m, ok := topVal.(map[string]any); ok {
			for subKey, subVal := range m {
				switch v := subVal.(type) {
				case []any:
					for _, item := range v {
						switch itemVal := item.(type) {
						case string:
							section[subKey] = append(section[subKey], itemVal)
						case map[string]any:
							// Recursively flatten nested fields with prefix
							nestedFields := flattenNestedFields(itemVal, "")
							section[subKey] = append(section[subKey], nestedFields...)
						}
					}
				case map[string]any: // Caso especial como em "transaction": { "metadata": [...] }
					section[subKey] = append(section[subKey], getMapKeys(v)...)
				}
			}
		}

		result[topKey] = section
	}

	return result
}

// flattenNestedFields recursively extracts all fields from a nested map structure,
// prefixing nested field names with their parent keys (e.g., "related_parties.role").
func flattenNestedFields(m map[string]any, prefix string) []string {
	var fields []string

	for key, val := range m {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}

		switch v := val.(type) {
		case []any:
			// Add the key itself as a field
			fields = append(fields, fieldName)
			// Also extract nested string fields
			for _, item := range v {
				switch itemVal := item.(type) {
				case string:
					fields = append(fields, fieldName+"."+itemVal)
				case map[string]any:
					// Recursively flatten deeper nested structures
					nested := flattenNestedFields(itemVal, fieldName)
					fields = append(fields, nested...)
				}
			}
		case map[string]any:
			// Recursively process nested maps
			nested := flattenNestedFields(v, fieldName)
			fields = append(fields, nested...)
		case string:
			fields = append(fields, fieldName)
		}
	}

	return fields
}

// getMapKeys retrieves all keys from a given map and returns them as a slice of strings.
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// extractFieldsFromExpression Get all valid object.property fields from expression
// Supports both legacy format (database.table.field) and explicit schema format (database:schema.table.field)
func extractFieldsFromExpression(expr string) []string {
	fields := []string{}

	// Split by pipe to separate base expression from filters
	parts := strings.Split(expr, "|")
	for i, part := range parts {
		part = strings.TrimSpace(part)

		// First part (before any pipe) is the main expression
		if i == 0 {
			// Check for explicit schema syntax (database:schema.table.field)
			if strings.Contains(part, ":") && strings.Contains(part, ".") {
				// Don't split by colon for explicit schema syntax
				fields = append(fields, part)
				continue
			}

			// Legacy format: just add if it has a dot
			if strings.Contains(part, ".") {
				fields = append(fields, part)
			}

			continue
		}

		// For filter parts, split by colon to handle filter arguments
		subParts := strings.Split(part, ":")
		for _, sub := range subParts {
			sub = strings.TrimSpace(sub)
			// Skip if it looks like a filter argument (contains quotes) or is too short
			if strings.Contains(sub, `"`) || strings.Contains(sub, `'`) {
				continue
			}

			if strings.Contains(sub, ".") {
				fields = append(fields, sub)
			}
		}
	}

	return fields
}

// insertField inserts a field into a nested map structure at a specified path, creating intermediate maps as needed.
// isExistingTable checks if a field already exists as a table (map) in the result structure.
// This prevents {% if datasource.table %} from overwriting a table that already has fields.
func isExistingTable(m map[string]any, path []string, field string) bool {
	current := m

	for _, p := range path {
		next, ok := current[p]
		if !ok {
			return false
		}

		switch val := next.(type) {
		case map[string]any:
			current = val
		default:
			return false
		}
	}

	// Check if field already exists with content (map with sub-fields or array of field names).
	// Either form means it's already been registered as a table, not a simple field.
	val, exists := current[field]
	if !exists {
		return false
	}

	switch v := val.(type) {
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	default:
		return false
	}
}

func insertField(m map[string]any, path []string, field string) {
	if len(path) == 0 {
		return
	}

	// Pass throw struct normally
	current := m

	for i, p := range path {
		if i == len(path)-1 {
			val := current[p]
			switch cast := val.(type) {
			case nil:
				current[p] = []any{field}
			case []any:
				current[p] = appendIfMissingAny(cast, field)
			case map[string]any:
				// Preserve existing table structure. A 1-element insertion path
				// (e.g. {% if datasource.field %}) at a position that already
				// holds a table map (populated by an earlier {% for %} + {{ }})
				// would otherwise clobber the map with []any{field}, then be
				// silently dropped by normalizeStructure — causing the schema
				// validator to skip the datasource entirely (see
				// schema-validation.go: `if len(tables) == 0 { continue }`).
				// We intentionally drop the orphan field reference here: the
				// tables already populated on this datasource are what allow
				// the schema validator to run and report ErrMissingDataSource
				// (when the datasource itself does not exist) or the right
				// missing-field/missing-table error.
				_ = cast
			default:
				current[p] = []any{field}
			}
		} else {
			next := current[p]
			switch val := next.(type) {
			case map[string]any:
				current = val
			case []any:
				found := false

				for _, item := range val {
					if m2, ok := item.(map[string]any); ok {
						current = m2
						found = true

						break
					}
				}

				if !found {
					newMap := map[string]any{}
					current[p] = append(val, newMap)
					current = newMap
				}
			case nil:
				newMap := map[string]any{}
				current[p] = newMap
				current = newMap
			default:
				newMap := map[string]any{}
				current[p] = newMap
				current = newMap
			}
		}
	}
}

// appendIfMissingAny add field only if does not exist yet
func appendIfMissingAny(slice []any, val any) []any {
	switch v := val.(type) {
	case string:
		for _, item := range slice {
			if str, ok := item.(string); ok && str == v {
				return slice
			}
		}
	case map[string]any:
		for _, item := range slice {
			if m, ok := item.(map[string]any); ok {
				for key := range m {
					if _, exists := v[key]; exists {
						return slice
					}
				}
			}
		}
	}

	return append(slice, val)
}

// CleanPath removes indexes and brackets from paths like foo[0].bar or foo.0.bar.
// It also handles the explicit schema syntax (database:schema.table.field) where
// the colon indicates an explicit schema reference. In this case, it returns:
// [database, schema__table, field1, field2, ...] using double underscore to create
// a Pongo2-compatible identifier that can be accessed via dot notation.
func CleanPath(path string) []string {
	// Check for explicit schema syntax (database:schema.table.field)
	if colonIdx := strings.Index(path, ":"); colonIdx != -1 {
		database := path[:colonIdx]
		rest := path[colonIdx+1:]

		// Split the rest by dots
		restParts := strings.Split(rest, ".")
		if len(restParts) < constant.MinPathParts {
			// Invalid format, fall back to legacy behavior
			return cleanPathLegacy(path)
		}

		// First two parts form schema__table (double underscore for Pongo2 compatibility)
		schema := stripPathDecorators(restParts[0])
		table := stripPathDecorators(restParts[1]) // Remove array bracket / filter pipeline if present

		clean := []string{database, schema + "__" + table}

		// Add remaining fields (skip numeric indices)
		for i := 2; i < len(restParts); i++ {
			base := stripPathDecorators(restParts[i])
			if _, err := strconv.Atoi(base); err == nil {
				continue
			}

			clean = append(clean, base)
		}

		return clean
	}

	// Legacy format: database.table.field
	return cleanPathLegacy(path)
}

// stripPathDecorators removes the array-index bracket suffix (e.g. "foo[0]")
// and any Pongo2 filter pipeline (e.g. `operation|add_day:"created_at"`) from a
// single path segment, returning the bare identifier. Schema references can be
// followed by filters (e.g. `db:schema.table|add_day:...`), and the filter must
// not become part of the extracted table/field name.
func stripPathDecorators(segment string) string {
	base := strings.Split(segment, "[")[0]
	base = strings.Split(base, "|")[0]

	return strings.TrimSpace(base)
}

// cleanPathLegacy handles the original path format without explicit schema.
func cleanPathLegacy(path string) []string {
	parts := strings.Split(path, ".")

	clean := make([]string, 0, len(parts))

	for _, p := range parts {
		base := stripPathDecorators(p)
		if _, err := strconv.Atoi(base); err == nil {
			continue
		}

		clean = append(clean, base)
	}

	return clean
}

// registerArithmeticFieldPaths extracts dotted field paths from arithmetic expressions
// (e.g., "6 + plugin_crm.holders|length") and ensures the referenced collections exist
// in the result map. For 2-part paths (datasource.collection), it only ensures the
// map structure exists without overwriting existing field lists.
func registerArithmeticFieldPaths(expr string, resultRegex map[string]any, variableMap map[string][]string) {
	fieldPaths := extractIfFromExpression(expr)

	for _, fieldExpr := range fieldPaths {
		parts := CleanPath(fieldExpr)
		if len(parts) < constant.MinPathParts {
			continue
		}

		if loopPath, ok := variableMap[parts[0]]; ok {
			if len(parts) > constant.MinPathParts {
				fullPath := append([]string{}, loopPath...)
				insertField(resultRegex, fullPath, parts[1])
			} else {
				// Collection reference (e.g., |length) - just ensure structure exists
				ensureMapStructure(resultRegex, loopPath)
			}
		} else if len(parts) > constant.MinPathParts {
			// Has specific field (datasource.collection.field)
			insertField(resultRegex, parts[:len(parts)-1], parts[len(parts)-1])
		} else {
			// Only datasource.collection (used with |length) - ensure structure exists
			ensureMapStructure(resultRegex, parts)
		}
	}
}

// containsArithmeticOperator checks if an expression contains arithmetic operators
// surrounded by spaces (e.g., "6 + plugin_crm.holders|length").
var arithmeticOperatorRegex = regexp.MustCompile(`\s[+\-*/]\s`)

func containsArithmeticOperator(expr string) bool {
	return arithmeticOperatorRegex.MatchString(expr)
}

// isConditionKeyword returns true if the given string is a comparison operator or keyword
// that should not be treated as a field name when extracting fields from if-conditions.
func isConditionKeyword(s string) bool {
	switch s {
	case "and", "or", "not", "true", "false", "nil", "is", "in", "eq", "ne", "lt", "gt", "le", "ge":
		return true
	default:
		return false
	}
}

// isQuotedString checks if a string is a quoted literal (starts and ends with quotes)
func isQuotedString(s string) bool {
	if len(s) < constant.MinQuotedStringLength {
		return false
	}

	return (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')
}

// eventHandlerPattern matches inline event handler attributes (onerror=, onload=, onclick=, etc.)
// The \b word boundary prevents false positives like "organization" containing "on"
var eventHandlerPattern = regexp.MustCompile(`(?i)\bon\w+\s*=`)

// ValidateNoScriptTag checks if the template contains <script> tags (case-insensitive) and returns an error if found.
// It also detects common XSS vectors like inline event handlers (onerror=, onload=, etc.)
func ValidateNoScriptTag(templateFile string) error {
	lower := strings.ToLower(templateFile)

	// Check for script tags (with or without attributes)
	// This catches: <script>, <script src="evil.js">, <script type="text/javascript">, etc.
	if strings.Contains(lower, "<script") || strings.Contains(lower, "</script") {
		return constant.ErrScriptTagDetected
	}

	// Check for tags that can load external resources (iframe, object, embed).
	// These enable LFI via file:// URLs in Chrome headless PDF rendering.
	if strings.Contains(lower, "<iframe") || strings.Contains(lower, "<object") || strings.Contains(lower, "<embed") {
		return constant.ErrScriptTagDetected
	}

	// Check for inline event handler attributes (onerror=, onload=, onclick=, etc.)
	// This catches: <img onerror="alert(1)">, <svg onload="alert(1)">, etc.
	if eventHandlerPattern.MatchString(templateFile) {
		return constant.ErrScriptTagDetected
	}

	return nil
}

// ParseDatabaseReference parses a database reference string and returns the database, schema, and table components.
// It supports two formats:
//   - Legacy format: "database.table" -> returns (database, "", table, nil)
//   - New format: "database:schema.table" -> returns (database, schema, table, nil)
//
// Returns an error if the format is invalid.
func ParseDatabaseReference(ref string) (database, schema, table string, err error) {
	// Check for new format with colon (database:schema.table)
	if strings.Contains(ref, ":") {
		colonIdx := strings.Index(ref, ":")
		database = ref[:colonIdx]
		rest := ref[colonIdx+1:]

		dotIdx := strings.Index(rest, ".")
		if dotIdx == -1 {
			return "", "", "", fmt.Errorf("invalid format: expected schema.table after ':', got %q", rest)
		}

		schema = rest[:dotIdx]
		table = rest[dotIdx+1:]

		return database, schema, table, nil
	}

	// Legacy format: database.table
	parts := strings.SplitN(ref, ".", constant.SplitKeyValueParts)
	if len(parts) != constant.SplitKeyValueParts {
		return "", "", "", fmt.Errorf("invalid format: expected database.table, got %q", ref)
	}

	return parts[0], "", parts[1], nil
}
