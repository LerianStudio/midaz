// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"

	"github.com/flosch/pongo2/v6"
)

// lastItemByGroupNode represents the last_item_by_group tag.
// It groups items by a field, picks the last item per group ordered by a date field DESC,
// optionally filters first, and stores the resulting list in a template variable.
type lastItemByGroupNode struct {
	collectionExpr pongo2.IEvaluator // Expression for the collection to process
	groupByExpr    pongo2.IEvaluator // Field to group by (e.g., account_id)
	orderByExpr    pongo2.IEvaluator // Field to order by for selecting last item (e.g., created_at)
	filterExpr     pongo2.IEvaluator // Optional filter condition
	resultVarName  string            // Variable name to store the result
}

// makeLastItemByGroupTag creates the tag parser for last_item_by_group.
// Syntax: {% last_item_by_group <collection> group_by "<group_field>" order_by "<date_field>" [if <condition>] as <result_var> %}
func makeLastItemByGroupTag() pongo2.TagParser {
	return func(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
		// 1. Parse collection expression
		collectionExpr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}

		// 2. Expect 'group_by' keyword + group field
		if args.Match(pongo2.TokenIdentifier, "group_by") == nil {
			return nil, args.Error("Expected 'group_by' keyword", nil)
		}

		groupByExpr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}

		// 3. Expect 'order_by' keyword + date field
		if args.Match(pongo2.TokenIdentifier, "order_by") == nil {
			return nil, args.Error("Expected 'order_by' keyword", nil)
		}

		orderByExpr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}

		// 4. Optional 'if' condition
		var filterExpr pongo2.IEvaluator
		if args.Match(pongo2.TokenIdentifier, "if") != nil {
			filterExpr, err = args.ParseExpression()
			if err != nil {
				return nil, err
			}
		}

		// 5. Expect 'as' keyword + result variable name
		// Note: 'as' is a reserved keyword in pongo2, so we use TokenKeyword
		if args.Match(pongo2.TokenKeyword, "as") == nil {
			return nil, args.Error("Expected 'as' keyword", nil)
		}

		resultVarToken := args.MatchType(pongo2.TokenIdentifier)
		if resultVarToken == nil {
			return nil, args.Error("Expected variable name after 'as'", nil)
		}

		return &lastItemByGroupNode{
			collectionExpr: collectionExpr,
			groupByExpr:    groupByExpr,
			orderByExpr:    orderByExpr,
			filterExpr:     filterExpr,
			resultVarName:  resultVarToken.Val,
		}, nil
	}
}

// Execute processes the last_item_by_group tag.
func (node *lastItemByGroupNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	// 1. Evaluate collection
	list, err := evaluateCollection(ctx, node.collectionExpr)
	if err != nil {
		return err
	}

	// 2. Check collection size to prevent resource exhaustion
	if len(list) > constant.MaxTagCollectionSize {
		return ctx.Error(fmt.Sprintf("collection size %d exceeds maximum allowed %d", len(list), constant.MaxTagCollectionSize), nil)
	}

	// 3. Get field names
	groupByField := node.getFieldName(ctx, node.groupByExpr)
	orderByField := node.getFieldName(ctx, node.orderByExpr)

	// 4. Filter items (if condition present)
	filtered := node.filterItems(ctx, list)

	// 5. Group by the specified field
	groups := node.groupByFieldValue(filtered, groupByField)

	// 6. For each group, get the last item by date
	results := node.collectLastItems(groups, orderByField, groupByField)

	// 7. Set result variable in context
	ctx.Private[node.resultVarName] = results

	return nil
}

// getFieldName evaluates a field expression and returns its string value.
func (node *lastItemByGroupNode) getFieldName(ctx *pongo2.ExecutionContext, expr pongo2.IEvaluator) string {
	if expr == nil {
		return ""
	}

	val, err := expr.Evaluate(ctx)
	if err != nil {
		return ""
	}

	return val.String()
}

// filterItems applies the filter expression to each item and returns matching items.
func (node *lastItemByGroupNode) filterItems(ctx *pongo2.ExecutionContext, items []map[string]any) []map[string]any {
	if node.filterExpr == nil {
		return items
	}

	var filtered []map[string]any

	for _, item := range items {
		if passesFilter(ctx, item, node.filterExpr) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

// groupByFieldValue groups items by the specified field (supports comma-separated composite keys).
func (node *lastItemByGroupNode) groupByFieldValue(items []map[string]any, field string) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)
	fields := strings.Split(field, ",")

	for _, item := range items {
		key, ok := buildCompositeKey(item, fields)
		if !ok {
			continue
		}

		groups[key] = append(groups[key], item)
	}

	return groups
}

// buildCompositeKey builds a composite grouping key from multiple fields.
// All fields must be present in the item for a valid key.
func buildCompositeKey(item map[string]any, fields []string) (string, bool) {
	if len(fields) == 1 {
		val, ok := getNestedField(item, strings.TrimSpace(fields[0]))
		if !ok {
			return "", false
		}

		return fmt.Sprintf("%v", val), true
	}

	parts := make([]string, 0, len(fields))

	for _, f := range fields {
		val, ok := getNestedField(item, strings.TrimSpace(f))
		if !ok {
			return "", false
		}

		parts = append(parts, fmt.Sprintf("%v", val))
	}

	return strings.Join(parts, "|"), true
}

// collectLastItems iterates over each group, picks the last item by date, and returns a sorted slice.
func (node *lastItemByGroupNode) collectLastItems(groups map[string][]map[string]any, orderByField, groupByField string) []map[string]any {
	results := make([]map[string]any, 0, len(groups))

	for _, items := range groups {
		lastItem := node.getLastByDate(items, orderByField)
		if lastItem != nil {
			results = append(results, lastItem)
		}
	}

	// Sort results by composite group key for deterministic output
	fields := strings.Split(groupByField, ",")

	sort.Slice(results, func(i, j int) bool {
		ki, okI := buildCompositeKey(results[i], fields)
		kj, okJ := buildCompositeKey(results[j], fields)

		if !okI || !okJ {
			return false
		}

		return ki < kj
	})

	return results
}

// getLastByDate returns the item with the latest date.
// If multiple items have the same date, uses the last one in iteration order.
// Items without a date field are included with zero time (lowest priority).
func (node *lastItemByGroupNode) getLastByDate(items []map[string]any, dateField string) map[string]any {
	if len(items) == 0 {
		return nil
	}

	var latest map[string]any

	var latestTime time.Time

	for _, item := range items {
		var itemTime time.Time

		if val, ok := getNestedField(item, dateField); ok {
			itemTime = parseTime(val)
		}
		// Items without date field get zero time (will be overwritten by any dated item)

		// Use >= to ensure deterministic behavior: later items win ties
		if latest == nil || itemTime.After(latestTime) || itemTime.Equal(latestTime) {
			latest = item
			latestTime = itemTime
		}
	}

	return latest
}

// parseTime attempts to parse a value as time.Time.
// This is a package-level function shared across tags.
func parseTime(val any) time.Time {
	switch v := val.(type) {
	case time.Time:
		return v
	case string:
		// Try RFC3339 first
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		// Try date only
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return t
		}
		// Try RFC3339Nano
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t
		}
	}

	return time.Time{}
}
