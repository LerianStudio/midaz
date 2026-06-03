// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import "strings"

const (
	defaultDatasource = "default"
	rootTable         = "_root"
)

// ExtractMappedFields walks the block tree and extracts all referenced data fields.
// Returns a map: datasource -> table -> [field1, field2, ...].
// Uses "default" as the datasource key and the collection as the table key.
// Variables in "collection.field" format are split into table and field.
func ExtractMappedFields(blocks []TemplateBlock) map[string]map[string][]string {
	result := make(map[string]map[string][]string)

	if len(blocks) == 0 {
		return result
	}

	seen := make(map[string]map[string]map[string]bool)

	extractFromBlocks(blocks, 0, result, seen)

	return result
}

// extractFromBlocks recursively processes blocks to extract mapped fields.
func extractFromBlocks(blocks []TemplateBlock, depth int, result map[string]map[string][]string, seen map[string]map[string]map[string]bool) {
	if depth > maxBlockDepth {
		return
	}

	for _, block := range blocks {
		extractFromBlock(block, depth, result, seen)
	}
}

// extractFromBlock extracts mapped fields from a single block.
func extractFromBlock(block TemplateBlock, depth int, result map[string]map[string][]string, seen map[string]map[string]map[string]bool) {
	switch block.Type {
	case "variable":
		addVariable(block.Variable, result, seen)
	case "loop":
		addField(block.Collection, "", result, seen)
		extractFromBlocks(block.Children, depth+1, result, seen)
	case "aggregation":
		addField(block.Collection, block.Field, result, seen)
	case "calculation":
		addVariable(block.Variable, result, seen)
	case "date_time":
		addVariable(block.Variable, result, seen)
	case "conditional":
		extractFromBlocks(block.Children, depth+1, result, seen)

		for _, elif := range block.ElifBranches {
			extractFromBlocks(elif.Children, depth+1, result, seen)
		}

		extractFromBlocks(block.ElseChildren, depth+1, result, seen)
	case "section":
		extractFromBlocks(block.Children, depth+1, result, seen)
	case "with":
		extractFromBlocks(block.Children, depth+1, result, seen)
	case "expression":
		// Expression blocks contain inline expressions; variables are not easily parseable.
	case "custom_tag":
		// Custom tag arguments may reference collections but are not easily parseable.
	}
}

// addVariable parses a variable name (potentially "table.field" format) and adds it.
func addVariable(variable string, result map[string]map[string][]string, seen map[string]map[string]map[string]bool) {
	if variable == "" {
		return
	}

	parts := strings.SplitN(variable, ".", 2)

	if len(parts) == 2 {
		addField(parts[0], parts[1], result, seen)
	} else {
		addField(rootTable, variable, result, seen)
	}
}

// addField adds a field to the result map with deduplication.
// If field is empty, only the table entry is ensured to exist.
func addField(table, field string, result map[string]map[string][]string, seen map[string]map[string]map[string]bool) {
	if table == "" {
		return
	}

	// Ensure datasource map exists
	if result[defaultDatasource] == nil {
		result[defaultDatasource] = make(map[string][]string)
		seen[defaultDatasource] = make(map[string]map[string]bool)
	}

	// Ensure table entry exists
	if seen[defaultDatasource][table] == nil {
		seen[defaultDatasource][table] = make(map[string]bool)
	}

	if result[defaultDatasource][table] == nil {
		result[defaultDatasource][table] = []string{}
	}

	// Add field if non-empty and not already seen
	if field != "" && !seen[defaultDatasource][table][field] {
		seen[defaultDatasource][table][field] = true
		result[defaultDatasource][table] = append(result[defaultDatasource][table], field)
	}
}
