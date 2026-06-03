// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"strings"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
)

// replaceFilter substitutes all occurrences of a search string with a replacement string.
// Syntax: {{ value|replace:"search:replacement" }}
// The parameter uses colon as separator between search and replacement.
// Examples:
//   - {{ "01310-100"|replace:"-:" }} → "01310100" (remove hyphen)
//   - {{ "1234.56"|replace:".:," }} → "1234,56" (dot to comma)
//   - {{ "12.345.678/0001-99"|replace:".:" }} → "12345678/0001-99" (remove dots)
func replaceFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.String()
	paramStr := param.String()

	// Parse parameter: "search:replacement"
	// Find the first colon as separator
	colonIndex := strings.Index(paramStr, ":")
	if colonIndex == -1 {
		return nil, &pongo2.Error{
			Sender:    "replace",
			OrigError: fmt.Errorf("invalid format, expected 'search:replacement', got '%s'", paramStr),
		}
	}

	search := paramStr[:colonIndex]
	replacement := paramStr[colonIndex+1:]

	result := strings.ReplaceAll(input, search, replacement)

	return pongo2.AsValue(result), nil
}

// whereFilter filters an array of maps by a field condition.
// Syntax: {{ array|where:"field:value" }}
// Supports nested fields using dot notation: {{ array|where:"address.state:SP" }}
// Examples:
//   - {{ holders|where:"state:SP" }} → holders where state == "SP"
//   - {{ holders|where:"address.uf:SP" }} → holders where address.uf == "SP"
func whereFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	// Try to get as []map[string]any first
	list, ok := toMapSlice(in.Interface())
	if !ok {
		return nil, &pongo2.Error{
			Sender:    "where",
			OrigError: fmt.Errorf("expected array of maps, got %T", in.Interface()),
		}
	}

	// Parse parameter: "field:value"
	paramStr := param.String()

	colonIndex := strings.Index(paramStr, ":")
	if colonIndex == -1 {
		return nil, &pongo2.Error{
			Sender:    "where",
			OrigError: fmt.Errorf("invalid format, expected 'field:value', got '%s'", paramStr),
		}
	}

	field := paramStr[:colonIndex]
	expectedValue := paramStr[colonIndex+1:]

	// Filter
	var result []map[string]any

	for _, item := range list {
		if val, ok := getNestedField(item, field); ok {
			if fmt.Sprintf("%v", val) == expectedValue {
				result = append(result, item)
			}
		}
	}

	// Return empty slice instead of nil for consistency
	if result == nil {
		result = []map[string]any{}
	}

	return pongo2.AsValue(result), nil
}

// toMapSlice attempts to convert various slice types to []map[string]any
func toMapSlice(v any) ([]map[string]any, bool) {
	switch t := v.(type) {
	case []map[string]any:
		return t, true
	case []any:
		result := make([]map[string]any, 0, len(t))

		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			} else {
				return nil, false
			}
		}

		return result, true
	default:
		return nil, false
	}
}

// sumFilter sums numeric values of a field in an array.
// Syntax: {{ array|sum:"field" }}
// Supports nested fields using dot notation.
// Uses decimal.Decimal for precision (avoids float rounding errors).
// Examples:
//   - {{ operations|sum:"amount" }} → "5000.50"
//   - {{ items|sum:"price.value" }} → "1500.75"
func sumFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	list, ok := toMapSlice(in.Interface())
	if !ok {
		return nil, &pongo2.Error{
			Sender:    "sum",
			OrigError: fmt.Errorf("expected array of maps, got %T", in.Interface()),
		}
	}

	// Get field name (remove quotes if present)
	field := strings.Trim(param.String(), "\"")

	// Sum values using decimal for precision
	total := decimal.Zero

	for _, item := range list {
		if val, ok := getNestedField(item, field); ok {
			if dec, ok := toDecimal(val); ok {
				total = total.Add(dec)
			}
		}
	}

	return pongo2.AsValue(total.String()), nil
}

// toDecimal converts various numeric types to decimal.Decimal
func toDecimal(v any) (decimal.Decimal, bool) {
	switch t := v.(type) {
	case int:
		return decimal.NewFromInt(int64(t)), true
	case int64:
		return decimal.NewFromInt(t), true
	case float64:
		return decimal.NewFromFloat(t), true
	case string:
		d, err := decimal.NewFromString(t)
		return d, err == nil
	case decimal.Decimal:
		return t, true
	default:
		return decimal.Zero, false
	}
}

// countFilter counts elements in an array where a field matches a value.
// Syntax: {{ array|count:"field:value" }}
// Supports nested fields using dot notation.
// Examples:
//   - {{ operations|count:"nat_oper:6" }} → 3
//   - {{ holders|count:"address.state:SP" }} → 2
func countFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	list, ok := toMapSlice(in.Interface())
	if !ok {
		return nil, &pongo2.Error{
			Sender:    "count",
			OrigError: fmt.Errorf("expected array of maps, got %T", in.Interface()),
		}
	}

	// Parse parameter: "field:value"
	paramStr := param.String()

	colonIndex := strings.Index(paramStr, ":")
	if colonIndex == -1 {
		return nil, &pongo2.Error{
			Sender:    "count",
			OrigError: fmt.Errorf("invalid format, expected 'field:value', got '%s'", paramStr),
		}
	}

	field := paramStr[:colonIndex]
	expectedValue := paramStr[colonIndex+1:]

	// Count matches
	count := 0

	for _, item := range list {
		if val, ok := getNestedField(item, field); ok {
			if fmt.Sprintf("%v", val) == expectedValue {
				count++
			}
		}
	}

	return pongo2.AsValue(count), nil
}
