// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumByTag(t *testing.T) {
	t.Parallel()
	tplStr := `{% sum_by data by "amount" %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"amount": 1000},
			{"amount": 2500},
			{"amount": 1500},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "5000", out)
}

func TestCountByTagWithFilter(t *testing.T) {
	t.Parallel()
	tplStr := `{% count_by data if amount > 1000 %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"amount": 1000},
			{"amount": 2500},
			{"amount": 1500},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2", out)
}

func TestAvgByTag(t *testing.T) {
	t.Parallel()
	tplStr := `{% avg_by data by "amount" %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"amount": 1000},
			{"amount": 2000},
			{"amount": 3000},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2000", out)
}

func TestMinByTag(t *testing.T) {
	t.Parallel()
	tplStr := `{% min_by data by "amount" %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"amount": 4000},
			{"amount": 1500},
			{"amount": 5000},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1500", out)
}

func TestMaxByTag(t *testing.T) {
	t.Parallel()
	tplStr := `{% max_by data by "amount" %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"amount": 1000},
			{"amount": 8000},
			{"amount": 3200},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "8000", out)
}

func TestCalcTag_BasicOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		expected string
	}{
		{"addition", `{% calc 10 + 5 %}`, "15"},
		{"subtraction", `{% calc 10 - 5 %}`, "5"},
		{"multiplication", `{% calc 10 * 5 %}`, "50"},
		{"division", `{% calc 10 / 5 %}`, "2"},
		{"power", `{% calc 2 ** 3 %}`, "8"},
		{"parentheses", `{% calc (10 + 5) * 2 %}`, "30"},
		{"decimal", `{% calc 10.5 + 5.5 %}`, "16"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(pongo2.Context{})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestCalcTag_NegativeNumbers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		expected string
	}{
		// Simple negative number
		{"single_negative", `{% calc -5 %}`, "-5"},

		// Subtraction resulting in negative
		{"subtraction_negative_result", `{% calc 3 - 10 %}`, "-7"},

		// Multiplication with negative
		{"multiply_negative", `{% calc -5 * 2 %}`, "-10"},
		{"multiply_two_negatives", `{% calc -5 * -2 %}`, "10"},

		// Division with negative
		{"divide_negative", `{% calc -10 / 2 %}`, "-5"},
		{"divide_by_negative", `{% calc 10 / -2 %}`, "-5"},

		// Parentheses with negative
		{"parentheses_negative", `{% calc (-5) + 10 %}`, "5"},
		{"parentheses_double_negative", `{% calc (-5) * (-2) %}`, "10"},

		// Addition with negative operand (5 + -3 = 2)
		{"add_negative_operand", `{% calc 5 + -3 %}`, "2"},

		// Subtraction of negative (5 - -3 = 8)
		{"subtract_negative_operand", `{% calc 5 - -3 %}`, "8"},

		// Complex with negatives
		{"complex_negative", `{% calc (-10 + 5) * 2 %}`, "-10"},
		{"complex_negative_result", `{% calc (5 - 15) / 2 %}`, "-5"},

		// Power with negative base
		{"power_negative_base", `{% calc -2 ** 2 %}`, "4"},
		{"power_negative_base_odd", `{% calc -2 ** 3 %}`, "-8"},

		// Decimal negatives
		{"decimal_negative", `{% calc -5.5 + 2.5 %}`, "-3"},
		{"decimal_negative_result", `{% calc 2.5 - 5.5 %}`, "-3"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err, "template parsing should not fail")

			out, err := tpl.Execute(pongo2.Context{})
			require.NoError(t, err, "template execution should not fail")
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestCalcTag_DivisionByZero(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% calc 10 / 0 %}`)
	require.NoError(t, err, "template parsing should not fail")

	_, err = tpl.Execute(pongo2.Context{})
	require.Error(t, err, "division by zero should return an error")
	assert.Contains(t, err.Error(), "division by zero")
}

func TestCalcTag_NegativeWithVariables(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		expected string
	}{
		{
			name:     "variable_with_negative_value",
			template: `{% calc value + 10 %}`,
			context:  pongo2.Context{"value": -5},
			expected: "5",
		},
		{
			name:     "subtract_from_negative_variable",
			template: `{% calc value - 3 %}`,
			context:  pongo2.Context{"value": -5},
			expected: "-8",
		},
		{
			name:     "multiply_negative_variables",
			template: `{% calc a * b %}`,
			context:  pongo2.Context{"a": -5, "b": -2},
			expected: "10",
		},
		{
			name:     "nested_negative_values",
			template: `{% calc data.amount * 2 %}`,
			context: pongo2.Context{
				"data": map[string]any{"amount": -100},
			},
			expected: "-200",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err, "template parsing should not fail")

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err, "template execution should not fail")
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestSumByTag_CompoundConditions(t *testing.T) {
	t.Parallel()
	// Test data matching the user's scenario
	data := []map[string]any{
		{
			"amount":                  1.50,
			"transfer_type":           "CASHIN",
			"destination_person_type": "NATURAL_PERSON",
			"status":                  "COMPLETED",
		},
		{
			"amount":                  1.50,
			"transfer_type":           "CASHIN",
			"destination_person_type": "NATURAL_PERSON",
			"status":                  "COMPLETED",
		},
		{
			"amount":                  5.00,
			"transfer_type":           "CASHOUT", // Different type - should be excluded
			"destination_person_type": "NATURAL_PERSON",
			"status":                  "COMPLETED",
		},
		{
			"amount":                  10.00,
			"transfer_type":           "CASHIN",
			"destination_person_type": "LEGAL_PERSON", // Different person type - should be excluded
			"status":                  "COMPLETED",
		},
		{
			"amount":                  20.00,
			"transfer_type":           "CASHIN",
			"destination_person_type": "NATURAL_PERSON",
			"status":                  "PENDING", // Different status - should be excluded
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "single_condition",
			template: `{% sum_by data by "amount" if transfer_type == "CASHIN" %}`,
			expected: "33", // 1.50 + 1.50 + 10.00 + 20.00 = 33
		},
		{
			name:     "two_conditions_with_and",
			template: `{% sum_by data by "amount" if transfer_type == "CASHIN" and status == "COMPLETED" %}`,
			expected: "13", // 1.50 + 1.50 + 10.00 = 13
		},
		{
			name:     "three_conditions_with_and",
			template: `{% sum_by data by "amount" if transfer_type == "CASHIN" and destination_person_type == "NATURAL_PERSON" and status == "COMPLETED" %}`,
			expected: "3", // 1.50 + 1.50 = 3
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err, "template parsing should not fail for: %s", tt.template)

			ctx := pongo2.Context{"data": data}
			out, err := tpl.Execute(ctx)
			require.NoError(t, err, "template execution should not fail")
			assert.Equal(t, tt.expected, out, "expected sum to match")
		})
	}
}

func TestConvertToGoDateLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full date YYYY-MM-dd",
			input:    "YYYY-MM-dd",
			expected: "2006-01-02",
		},
		{
			name:     "date with time dd/MM/YYYY HH:mm:ss",
			input:    "dd/MM/YYYY HH:mm:ss",
			expected: "02/01/2006 15:04:05",
		},
		{
			name:     "year only",
			input:    "YYYY",
			expected: "2006",
		},
		{
			name:     "time only HH:mm",
			input:    "HH:mm",
			expected: "15:04",
		},
		{
			name:     "ISO format YYYY-MM-ddTHH:mm:ss",
			input:    "YYYY-MM-ddTHH:mm:ss",
			expected: "2006-01-02T15:04:05",
		},
		{
			name:     "no recognized tokens returns input unchanged",
			input:    "hello-world",
			expected: "hello-world",
		},
		{
			name:     "empty string returns empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertToGoDateLayout(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldSkipMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		match    string
		expected bool
	}{
		// Operators and parentheses - should skip
		{name: "plus operator", match: "+", expected: true},
		{name: "minus operator", match: "-", expected: true},
		{name: "multiply operator", match: "*", expected: true},
		{name: "divide operator", match: "/", expected: true},
		{name: "power operator", match: "**", expected: true},
		{name: "open parenthesis", match: "(", expected: true},
		{name: "close parenthesis", match: ")", expected: true},
		{name: "open bracket", match: "[", expected: true},
		{name: "close bracket", match: "]", expected: true},
		{name: "open brace", match: "{", expected: true},
		{name: "close brace", match: "}", expected: true},
		// Numbers without dots - should skip
		{name: "integer 42", match: "42", expected: true},
		{name: "integer 0", match: "0", expected: true},
		{name: "negative-looking integer 100", match: "100", expected: true},
		// Variable paths with dots - should NOT skip (contain dots, ParseFloat is not called)
		{name: "variable path with dot", match: "variable.path", expected: false},
		{name: "nested variable path", match: "data.nested.field", expected: false},
		// Plain identifiers - should NOT skip (not operators, not numbers)
		{name: "plain identifier", match: "variable", expected: false},
		{name: "identifier with underscore", match: "my_var", expected: false},
		// Float-like strings with dots are NOT skipped because they contain dots
		{name: "float 3.14 not skipped due to dot", match: "3.14", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := shouldSkipMatch(tt.match)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveVariableFromContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		match     string
		context   pongo2.Context
		wantValue string
		wantOK    bool
	}{
		{
			name:      "resolves integer variable",
			match:     "amount",
			context:   pongo2.Context{"amount": 42},
			wantValue: "42",
			wantOK:    true,
		},
		{
			name:      "resolves float variable",
			match:     "price",
			context:   pongo2.Context{"price": 3.14},
			wantValue: "3.140000",
			wantOK:    true,
		},
		{
			name:      "resolves nested map variable",
			match:     "data.amount",
			context:   pongo2.Context{"data": map[string]any{"amount": 100}},
			wantValue: "100",
			wantOK:    true,
		},
		{
			name:    "missing variable returns false",
			match:   "nonexistent",
			context: pongo2.Context{},
			wantOK:  false,
		},
		{
			name:    "non-numeric value returns false",
			match:   "name",
			context: pongo2.Context{"name": "hello"},
			wantOK:  false,
		},
		{
			name:    "empty string value returns false",
			match:   "empty",
			context: pongo2.Context{"empty": ""},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, ok := resolveVariableFromContext(tt.match, tt.context)
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantOK {
				assert.Equal(t, tt.wantValue, value)
			}
		})
	}
}

func TestExtractDecimalValue_ThroughTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		expected string
	}{
		{
			name:     "sum with int values",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": 42},
					{"value": 58},
				},
			},
			expected: "100",
		},
		{
			name:     "sum with int64 values",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": int64(100)},
					{"value": int64(200)},
				},
			},
			expected: "300",
		},
		{
			name:     "sum with float64 values",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": 3.14},
					{"value": 6.86},
				},
			},
			expected: "10",
		},
		{
			name:     "sum with string numeric values",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": "99.5"},
					{"value": "0.5"},
				},
			},
			expected: "100",
		},
		{
			name:     "sum with non-numeric string values skips them",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": "not_a_number"},
					{"value": 50},
				},
			},
			expected: "50",
		},
		{
			name:     "sum with missing field skips item",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"other": 10},
					{"value": 50},
				},
			},
			expected: "50",
		},
		{
			name:     "sum with empty string value skips item",
			template: `{% sum_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": ""},
					{"value": 75},
				},
			},
			expected: "75",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestAggregatorResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func() *aggregator
		expected string
	}{
		{
			name: "count returns integer",
			setup: func() *aggregator {
				a := newAggregator("count")
				a.count = 5
				return a
			},
			expected: "5",
		},
		{
			name: "sum returns total",
			setup: func() *aggregator {
				a := newAggregator("sum")
				a.total = decimal.NewFromInt(100)
				return a
			},
			expected: "100",
		},
		{
			name: "avg with zero count returns zero",
			setup: func() *aggregator {
				return newAggregator("avg")
			},
			expected: "0",
		},
		{
			name: "min with nil returns zero",
			setup: func() *aggregator {
				return newAggregator("min")
			},
			expected: "0",
		},
		{
			name: "max with nil returns zero",
			setup: func() *aggregator {
				return newAggregator("max")
			},
			expected: "0",
		},
		{
			name: "unknown op returns NaN",
			setup: func() *aggregator {
				return newAggregator("unknown")
			},
			expected: "NaN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := tt.setup()
			result := a.result()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalcTag_VariableReplacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		expected string
	}{
		{
			name:     "simple variable replacement",
			template: `{% calc amount + 10 %}`,
			context:  pongo2.Context{"amount": 5},
			expected: "15",
		},
		{
			name:     "nested variable replacement",
			template: `{% calc data.price * 2 %}`,
			context:  pongo2.Context{"data": map[string]any{"price": 25}},
			expected: "50",
		},
		{
			name:     "missing variable replaced with zero",
			template: `{% calc missing + 10 %}`,
			context:  pongo2.Context{},
			expected: "10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestDateTimeTag(t *testing.T) {
	t.Parallel()

	// We can only test that the tag produces a valid formatted date string
	// since the actual date changes. We verify the format is correct.
	tests := []struct {
		name     string
		template string
		lenCheck int
	}{
		{
			name:     "YYYY-MM-dd format produces 10-char date",
			template: `{% date_time "YYYY-MM-dd" %}`,
			lenCheck: 10,
		},
		{
			name:     "YYYY format produces 4-char year",
			template: `{% date_time "YYYY" %}`,
			lenCheck: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(pongo2.Context{})
			require.NoError(t, err)
			assert.Len(t, out, tt.lenCheck)
		})
	}
}

func TestPassesFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		item     map[string]any
		filter   string
		expected bool
	}{
		{
			name:     "nil filter always passes",
			item:     map[string]any{"status": "active"},
			filter:   "",
			expected: true,
		},
		{
			name:     "matching filter passes",
			item:     map[string]any{"status": "active"},
			filter:   `status == "active"`,
			expected: true,
		},
		{
			name:     "non-matching filter fails",
			item:     map[string]any{"status": "inactive"},
			filter:   `status == "active"`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := pongo2.Context{}
			execCtx := pongo2.NewChildExecutionContext(&pongo2.ExecutionContext{
				Public:  ctx,
				Private: pongo2.Context{},
			})

			if tt.filter == "" {
				result := passesFilter(execCtx, tt.item, nil)
				assert.Equal(t, tt.expected, result)
				return
			}

			// Build a filter expression by parsing a template with the condition
			templateStr := `{% if ` + tt.filter + ` %}true{% endif %}`
			ts := pongo2.NewSet("test", pongo2.DefaultLoader)

			_, err := ts.FromString(templateStr)
			require.NoError(t, err)

			// Since passesFilter requires an IEvaluator, test through sum_by template
			tplStr := `{% count_by data if ` + tt.filter + ` %}`
			tpl, err := SafeFromString(tplStr)
			require.NoError(t, err)

			out, err := tpl.Execute(pongo2.Context{
				"data": []map[string]any{tt.item},
			})
			require.NoError(t, err)

			if tt.expected {
				assert.Equal(t, "1", out)
			} else {
				assert.Equal(t, "0", out)
			}
		})
	}
}

func TestAccumulate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		op       string
		values   []int64
		expected string
	}{
		{
			name:     "sum accumulates values",
			op:       "sum",
			values:   []int64{10, 20, 30},
			expected: "60",
		},
		{
			name:     "avg computes average",
			op:       "avg",
			values:   []int64{10, 20, 30},
			expected: "20",
		},
		{
			name:     "min finds minimum",
			op:       "min",
			values:   []int64{30, 10, 20},
			expected: "10",
		},
		{
			name:     "max finds maximum",
			op:       "max",
			values:   []int64{10, 30, 20},
			expected: "30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := newAggregator(tt.op)

			for _, v := range tt.values {
				err := a.accumulate(decimal.NewFromInt(v))
				assert.Nil(t, err)
			}

			assert.Equal(t, tt.expected, a.result())
		})
	}
}

func TestMakeAggregateTag_MissingByKeyword(t *testing.T) {
	t.Parallel()

	// Non-count aggregate tags require 'by' keyword. Without it, parsing should fail.
	tests := []struct {
		name     string
		template string
	}{
		{"sum_by missing by", `{% sum_by data "amount" %}`},
		{"avg_by missing by", `{% avg_by data "amount" %}`},
		{"min_by missing by", `{% min_by data "amount" %}`},
		{"max_by missing by", `{% max_by data "amount" %}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := SafeFromString(tt.template)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Expected 'by' keyword")
		})
	}
}

func TestAggregateTagNode_Execute_NonCollectionType(t *testing.T) {
	t.Parallel()

	// When the collection variable is not []map[string]any, Execute should return
	// an error from evaluateCollection's type assertion failure.
	tests := []struct {
		name    string
		tplStr  string
		context pongo2.Context
	}{
		{
			name:    "count_by with string collection",
			tplStr:  `{% count_by data %}`,
			context: pongo2.Context{"data": "not a collection"},
		},
		{
			name:    "sum_by with integer collection",
			tplStr:  `{% sum_by data by "amount" %}`,
			context: pongo2.Context{"data": 42},
		},
		{
			name:    "count_by with slice of strings",
			tplStr:  `{% count_by data %}`,
			context: pongo2.Context{"data": []string{"a", "b"}},
		},
		{
			name:    "sum_by with nil collection",
			tplStr:  `{% sum_by data by "amount" %}`,
			context: pongo2.Context{"data": nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.tplStr)
			require.NoError(t, err)

			_, err = tpl.Execute(tt.context)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Expected []map[string]any")
		})
	}
}

func TestAggregateTag_EmptyCollectionResults(t *testing.T) {
	t.Parallel()

	// Empty collections should return default values for each operation
	tests := []struct {
		name     string
		template string
		expected string
	}{
		{"sum_by empty list", `{% sum_by data by "amount" %}`, "0"},
		{"avg_by empty list", `{% avg_by data by "amount" %}`, "0"},
		{"min_by empty list", `{% min_by data by "amount" %}`, "0"},
		{"max_by empty list", `{% max_by data by "amount" %}`, "0"},
		{"count_by empty list", `{% count_by data %}`, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(pongo2.Context{"data": []map[string]any{}})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestCalcTag_InvalidExpression(t *testing.T) {
	t.Parallel()

	// An expression that evaluates to an invalid arithmetic operation
	tpl, err := SafeFromString(`{% calc 10 / 0 + 5 %}`)
	require.NoError(t, err)

	_, err = tpl.Execute(pongo2.Context{})
	require.Error(t, err)
}
