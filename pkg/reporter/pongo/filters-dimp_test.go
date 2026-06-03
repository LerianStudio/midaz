// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"testing"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T1: Replace Filter Tests
// =============================================================================

func TestReplaceFilter_RemoveHyphen(t *testing.T) {
	t.Parallel()
	// {{ "01310-100"|replace:"-:" }} → "01310100"
	val, err := replaceFilter(pongo2.AsValue("01310-100"), pongo2.AsValue("-:"))
	assert.Nil(t, err)
	assert.Equal(t, "01310100", val.String())
}

func TestReplaceFilter_DotToComma(t *testing.T) {
	t.Parallel()
	// {{ "1234.56"|replace:".:," }} → "1234,56"
	val, err := replaceFilter(pongo2.AsValue("1234.56"), pongo2.AsValue(".:,"))
	assert.Nil(t, err)
	assert.Equal(t, "1234,56", val.String())
}

func TestReplaceFilter_MultipleOccurrences(t *testing.T) {
	t.Parallel()
	// {{ "a-b-c-d"|replace:"-:_" }} → "a_b_c_d"
	val, err := replaceFilter(pongo2.AsValue("a-b-c-d"), pongo2.AsValue("-:_"))
	assert.Nil(t, err)
	assert.Equal(t, "a_b_c_d", val.String())
}

func TestReplaceFilter_NotFound(t *testing.T) {
	t.Parallel()
	// {{ "abc"|replace:"x:y" }} → "abc"
	val, err := replaceFilter(pongo2.AsValue("abc"), pongo2.AsValue("x:y"))
	assert.Nil(t, err)
	assert.Equal(t, "abc", val.String())
}

func TestReplaceFilter_EmptyReplacement(t *testing.T) {
	t.Parallel()
	// {{ "12.345.678"|replace:".:" }} → "12345678"
	val, err := replaceFilter(pongo2.AsValue("12.345.678"), pongo2.AsValue(".:"))
	assert.Nil(t, err)
	assert.Equal(t, "12345678", val.String())
}

func TestReplaceFilter_EmptyInput(t *testing.T) {
	t.Parallel()
	// {{ ""|replace:"-:" }} → ""
	val, err := replaceFilter(pongo2.AsValue(""), pongo2.AsValue("-:"))
	assert.Nil(t, err)
	assert.Equal(t, "", val.String())
}

func TestReplaceFilter_InvalidFormat(t *testing.T) {
	t.Parallel()
	// Missing separator
	_, err := replaceFilter(pongo2.AsValue("test"), pongo2.AsValue("invalid"))
	assert.NotNil(t, err)
}

func TestReplaceFilter_CNPJ(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: remove all punctuation from CNPJ
	// Step 1: remove dots
	val1, err := replaceFilter(pongo2.AsValue("12.345.678/0001-99"), pongo2.AsValue(".:"))
	assert.Nil(t, err)
	assert.Equal(t, "12345678/0001-99", val1.String())

	// Step 2: remove slash
	val2, err := replaceFilter(pongo2.AsValue(val1.String()), pongo2.AsValue("/:"))
	assert.Nil(t, err)
	assert.Equal(t, "123456780001-99", val2.String())

	// Step 3: remove hyphen
	val3, err := replaceFilter(pongo2.AsValue(val2.String()), pongo2.AsValue("-:"))
	assert.Nil(t, err)
	assert.Equal(t, "12345678000199", val3.String())
}

func TestReplaceFilter_Integration(t *testing.T) {
	t.Parallel()
	// Test with pongo2 template
	tplStr := `{{ cnpj|replace:".:"|replace:"/:"|replace:"-:" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"cnpj": "12.345.678/0001-99",
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "12345678000199", out)
}

func TestReplaceFilter_CEP(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: remove hyphen from CEP
	tplStr := `{{ cep|replace:"-:" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"cep": "01310-100",
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "01310100", out)
}

func TestReplaceFilter_DecimalComma(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: convert decimal point to comma for Brazilian format
	tplStr := `{{ valor|replace:".:," }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"valor": "1234.56",
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1234,56", out)
}

// =============================================================================
// T2: Where Filter Tests
// =============================================================================

func TestWhereFilter_SimpleField(t *testing.T) {
	t.Parallel()
	// {{ holders|where:"state:SP" }}
	input := []map[string]any{
		{"name": "Alice", "state": "SP"},
		{"name": "Bob", "state": "RJ"},
		{"name": "Carol", "state": "SP"},
		{"name": "Dave", "state": "MG"},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("state:SP"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0]["name"])
	assert.Equal(t, "Carol", result[1]["name"])
}

func TestWhereFilter_NestedField(t *testing.T) {
	t.Parallel()
	// {{ holders|where:"address.state:SP" }}
	input := []map[string]any{
		{"name": "Alice", "address": map[string]any{"state": "SP", "city": "Sao Paulo"}},
		{"name": "Bob", "address": map[string]any{"state": "RJ", "city": "Rio"}},
		{"name": "Carol", "address": map[string]any{"state": "SP", "city": "Campinas"}},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("address.state:SP"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0]["name"])
	assert.Equal(t, "Carol", result[1]["name"])
}

func TestWhereFilter_EmptyArray(t *testing.T) {
	t.Parallel()
	// {{ []|where:"state:SP" }} → []
	input := []map[string]any{}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("state:SP"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 0)
}

func TestWhereFilter_NoMatch(t *testing.T) {
	t.Parallel()
	// {{ holders|where:"state:XX" }} → []
	input := []map[string]any{
		{"name": "Alice", "state": "SP"},
		{"name": "Bob", "state": "RJ"},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("state:XX"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 0)
}

func TestWhereFilter_PreservesOrder(t *testing.T) {
	t.Parallel()
	// Verify order is preserved
	input := []map[string]any{
		{"id": 1, "type": "A"},
		{"id": 2, "type": "B"},
		{"id": 3, "type": "A"},
		{"id": 4, "type": "A"},
		{"id": 5, "type": "B"},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("type:A"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 3)
	assert.Equal(t, 1, result[0]["id"])
	assert.Equal(t, 3, result[1]["id"])
	assert.Equal(t, 4, result[2]["id"])
}

func TestWhereFilter_NumericValue(t *testing.T) {
	t.Parallel()
	// Filter by numeric value (as string comparison)
	input := []map[string]any{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
		{"name": "Carol", "age": 30},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("age:30"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 2)
}

func TestWhereFilter_InvalidFormat(t *testing.T) {
	t.Parallel()
	input := []map[string]any{
		{"name": "Alice"},
	}

	_, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("invalid"))
	assert.NotNil(t, err)
}

func TestWhereFilter_InvalidInput(t *testing.T) {
	t.Parallel()
	// Not an array
	_, err := whereFilter(pongo2.AsValue("not an array"), pongo2.AsValue("field:value"))
	assert.NotNil(t, err)
}

func TestWhereFilter_Integration(t *testing.T) {
	t.Parallel()
	// Test with pongo2 template - filter by UF (DIMP use case)
	tplStr := `{% for h in holders|where:"uf:SP" %}{{ h.name }};{% endfor %}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"holders": []map[string]any{
			{"name": "Alice", "uf": "SP"},
			{"name": "Bob", "uf": "RJ"},
			{"name": "Carol", "uf": "SP"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Alice;Carol;", out)
}

func TestWhereFilter_WithAnySlice(t *testing.T) {
	t.Parallel()
	// Test with []any input (common when data comes from JSON)
	input := []any{
		map[string]any{"name": "Alice", "state": "SP"},
		map[string]any{"name": "Bob", "state": "RJ"},
	}

	val, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("state:SP"))
	assert.Nil(t, err)

	result, ok := val.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Len(t, result, 1)
	assert.Equal(t, "Alice", result[0]["name"])
}

func TestWhereFilter_WithInvalidAnySlice(t *testing.T) {
	t.Parallel()
	// Test with []any containing non-map items
	input := []any{
		"not a map",
		123,
	}

	_, err := whereFilter(pongo2.AsValue(input), pongo2.AsValue("field:value"))
	assert.NotNil(t, err)
}

// =============================================================================
// T3: Sum Filter Tests
// =============================================================================

func TestSumFilter_SimpleField(t *testing.T) {
	t.Parallel()
	// {{ operations|sum:"amount" }} → "5000.5"
	input := []map[string]any{
		{"name": "Op1", "amount": 1000.50},
		{"name": "Op2", "amount": 2000.00},
		{"name": "Op3", "amount": 1500.00},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "4500.5", val.String())
}

func TestSumFilter_IntegerValues(t *testing.T) {
	t.Parallel()
	input := []map[string]any{
		{"value": 100},
		{"value": 200},
		{"value": 300},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("value"))
	assert.Nil(t, err)
	assert.Equal(t, "600", val.String())
}

func TestSumFilter_StringNumericValues(t *testing.T) {
	t.Parallel()
	input := []map[string]any{
		{"amount": "1000.50"},
		{"amount": "2000.25"},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "3000.75", val.String())
}

func TestSumFilter_EmptyArray(t *testing.T) {
	t.Parallel()
	// {{ []|sum:"amount" }} → "0"
	input := []map[string]any{}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "0", val.String())
}

func TestSumFilter_MissingField(t *testing.T) {
	t.Parallel()
	// Items without field are skipped
	input := []map[string]any{
		{"amount": 100},
		{"other": 200}, // no "amount" field
		{"amount": 300},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "400", val.String())
}

func TestSumFilter_MixedTypes(t *testing.T) {
	t.Parallel()
	// int, int64, float64, string numeric values
	input := []map[string]any{
		{"value": 100},        // int
		{"value": int64(200)}, // int64
		{"value": 300.50},     // float64
		{"value": "400.25"},   // string
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("value"))
	assert.Nil(t, err)
	assert.Equal(t, "1000.75", val.String())
}

func TestSumFilter_NestedField(t *testing.T) {
	t.Parallel()
	input := []map[string]any{
		{"transaction": map[string]any{"amount": 100.0}},
		{"transaction": map[string]any{"amount": 200.0}},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("transaction.amount"))
	assert.Nil(t, err)
	assert.Equal(t, "300", val.String())
}

func TestSumFilter_InvalidInput(t *testing.T) {
	t.Parallel()
	_, err := sumFilter(pongo2.AsValue("not an array"), pongo2.AsValue("field"))
	assert.NotNil(t, err)
}

func TestSumFilter_DecimalPrecision(t *testing.T) {
	t.Parallel()
	// Test that decimal precision is maintained (no float artifacts)
	input := []map[string]any{
		{"amount": "0.1"},
		{"amount": "0.2"},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	// Should be exactly 0.3, not 0.30000000000000004
	assert.Equal(t, "0.3", val.String())
}

func TestSumFilter_Integration(t *testing.T) {
	t.Parallel()
	// Test with pongo2 template - DIMP total calculation
	tplStr := `{{ operations|sum:"amount" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"type": "credit", "amount": 1000.00},
			{"type": "debit", "amount": 500.00},
			{"type": "credit", "amount": 750.50},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2250.5", out)
}

func TestSumFilter_WithWhereChain(t *testing.T) {
	t.Parallel()
	// Test chaining where + sum (DIMP use case: sum by UF)
	tplStr := `{{ operations|where:"uf:SP"|sum:"amount" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"uf": "SP", "amount": 1000.00},
			{"uf": "RJ", "amount": 500.00},
			{"uf": "SP", "amount": 750.00},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1750", out)
}

func TestSumFilter_DecimalType(t *testing.T) {
	t.Parallel()
	// Test with decimal.Decimal type directly
	dec1, _ := decimal.NewFromString("100.50")
	dec2, _ := decimal.NewFromString("200.25")

	input := []map[string]any{
		{"amount": dec1},
		{"amount": dec2},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "300.75", val.String())
}

func TestSumFilter_InvalidStringSkipped(t *testing.T) {
	t.Parallel()
	// Invalid string values are skipped
	input := []map[string]any{
		{"amount": "100"},
		{"amount": "not-a-number"},
		{"amount": "200"},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "300", val.String())
}

func TestSumFilter_UnsupportedTypeSkipped(t *testing.T) {
	t.Parallel()
	// Unsupported types (like bool) are skipped
	input := []map[string]any{
		{"amount": 100},
		{"amount": true}, // unsupported
		{"amount": 200},
	}

	val, err := sumFilter(pongo2.AsValue(input), pongo2.AsValue("amount"))
	assert.Nil(t, err)
	assert.Equal(t, "300", val.String())
}

// =============================================================================
// T4: Count Filter Tests
// =============================================================================

func TestCountFilter_SimpleField(t *testing.T) {
	t.Parallel()
	// {{ operations|count:"nat_oper:6" }} → 3
	input := []map[string]any{
		{"name": "Op1", "nat_oper": "6"},
		{"name": "Op2", "nat_oper": "7"},
		{"name": "Op3", "nat_oper": "6"},
		{"name": "Op4", "nat_oper": "6"},
	}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("nat_oper:6"))
	assert.Nil(t, err)
	assert.Equal(t, 3, val.Integer())
}

func TestCountFilter_NestedField(t *testing.T) {
	t.Parallel()
	// {{ holders|count:"address.state:SP" }} → 2
	input := []map[string]any{
		{"name": "Alice", "address": map[string]any{"state": "SP"}},
		{"name": "Bob", "address": map[string]any{"state": "RJ"}},
		{"name": "Carol", "address": map[string]any{"state": "SP"}},
	}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("address.state:SP"))
	assert.Nil(t, err)
	assert.Equal(t, 2, val.Integer())
}

func TestCountFilter_EmptyArray(t *testing.T) {
	t.Parallel()
	// {{ []|count:"state:SP" }} → 0
	input := []map[string]any{}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("state:SP"))
	assert.Nil(t, err)
	assert.Equal(t, 0, val.Integer())
}

func TestCountFilter_NoMatch(t *testing.T) {
	t.Parallel()
	// {{ holders|count:"state:XX" }} → 0
	input := []map[string]any{
		{"name": "Alice", "state": "SP"},
		{"name": "Bob", "state": "RJ"},
	}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("state:XX"))
	assert.Nil(t, err)
	assert.Equal(t, 0, val.Integer())
}

func TestCountFilter_NumericValue(t *testing.T) {
	t.Parallel()
	// Count by numeric value (compared as string)
	input := []map[string]any{
		{"type": 1},
		{"type": 2},
		{"type": 1},
		{"type": 1},
	}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("type:1"))
	assert.Nil(t, err)
	assert.Equal(t, 3, val.Integer())
}

func TestCountFilter_InvalidFormat(t *testing.T) {
	t.Parallel()
	input := []map[string]any{
		{"name": "Alice"},
	}

	_, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("invalid"))
	assert.NotNil(t, err)
}

func TestCountFilter_InvalidInput(t *testing.T) {
	t.Parallel()
	_, err := countFilter(pongo2.AsValue("not an array"), pongo2.AsValue("field:value"))
	assert.NotNil(t, err)
}

func TestCountFilter_Integration(t *testing.T) {
	t.Parallel()
	// Test with pongo2 template - DIMP 9900 record count
	tplStr := `{{ records|count:"tipo_reg:0000" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"records": []map[string]any{
			{"tipo_reg": "0000"},
			{"tipo_reg": "1100"},
			{"tipo_reg": "1100"},
			{"tipo_reg": "9900"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1", out)
}

func TestCountFilter_WithWhereChain(t *testing.T) {
	t.Parallel()
	// Test chaining where + count (DIMP use case)
	tplStr := `{{ operations|where:"uf:SP"|count:"tipo:credit" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"uf": "SP", "tipo": "credit"},
			{"uf": "RJ", "tipo": "credit"},
			{"uf": "SP", "tipo": "debit"},
			{"uf": "SP", "tipo": "credit"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2", out)
}

func TestCountFilter_AllMatch(t *testing.T) {
	t.Parallel()
	// All items match
	input := []map[string]any{
		{"status": "active"},
		{"status": "active"},
		{"status": "active"},
	}

	val, err := countFilter(pongo2.AsValue(input), pongo2.AsValue("status:active"))
	assert.Nil(t, err)
	assert.Equal(t, 3, val.Integer())
}

// =============================================================================
// T5: Integration Tests - Complete DIMP Workflow
// =============================================================================

func TestDIMPFilters_AllRegistered(t *testing.T) {
	t.Parallel()
	// Verify all DIMP filters are registered and available
	filters := []string{"replace", "where", "sum", "count"}

	for _, filterName := range filters {
		tplStr := fmt.Sprintf(`{{ "test"|%s:"a:b" }}`, filterName)
		_, err := SafeFromString(tplStr)
		// Should not get "filter does not exist" error
		if err != nil {
			assert.NotContains(t, err.Error(), "does not exist",
				"Filter '%s' should be registered", filterName)
		}
	}
}

func TestDIMPFilters_CNPJFormatting(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: format CNPJ without punctuation
	tplStr := `{{ cnpj|replace:".:"|replace:"/:"|replace:"-:" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	testCases := []struct {
		input    string
		expected string
	}{
		{"12.345.678/0001-99", "12345678000199"},
		{"00.000.000/0001-00", "00000000000100"},
		{"98.765.432/0001-10", "98765432000110"},
	}

	for _, tc := range testCases {
		ctx := pongo2.Context{"cnpj": tc.input}
		out, err := tpl.Execute(ctx)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, out, "CNPJ %s should format to %s", tc.input, tc.expected)
	}
}

func TestDIMPFilters_CPFFormatting(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: format CPF without punctuation
	tplStr := `{{ cpf|replace:".:"|replace:"-:" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{"cpf": "123.456.789-00"}
	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "12345678900", out)
}

func TestDIMPFilters_CEPFormatting(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: format CEP without hyphen
	tplStr := `{{ cep|replace:"-:" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{"cep": "01310-100"}
	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "01310100", out)
}

func TestDIMPFilters_DecimalBrazilianFormat(t *testing.T) {
	t.Parallel()
	// Real DIMP use case: convert decimal point to comma
	tplStr := `{{ valor|replace:".:," }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	testCases := []struct {
		input    string
		expected string
	}{
		{"1234.56", "1234,56"},
		{"1000000.00", "1000000,00"},
		{"0.99", "0,99"},
	}

	for _, tc := range testCases {
		ctx := pongo2.Context{"valor": tc.input}
		out, err := tpl.Execute(ctx)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, out)
	}
}

func TestDIMPFilters_FilterAndSum(t *testing.T) {
	t.Parallel()
	// DIMP use case: sum amounts by UF (for records 1100/1110)
	tplStr := `{{ operations|where:"uf:SP"|sum:"amount" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"uf": "SP", "amount": 1000.00},
			{"uf": "RJ", "amount": 500.00},
			{"uf": "SP", "amount": 750.50},
			{"uf": "MG", "amount": 300.00},
			{"uf": "SP", "amount": 249.50},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2000", out) // 1000 + 750.50 + 249.50 = 2000
}

func TestDIMPFilters_FilterAndCount(t *testing.T) {
	t.Parallel()
	// DIMP use case: count records by type (for record 9900)
	tplStr := `{{ records|count:"tipo_reg:1100" }}`
	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"records": []map[string]any{
			{"tipo_reg": "0000"},
			{"tipo_reg": "1100"},
			{"tipo_reg": "1100"},
			{"tipo_reg": "1100"},
			{"tipo_reg": "1110"},
			{"tipo_reg": "9900"},
			{"tipo_reg": "9999"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "3", out)
}

func TestDIMPFilters_CompleteWorkflow(t *testing.T) {
	t.Parallel()
	// Complete DIMP template simulation
	tplStr := `|0000|{{ empresa.cnpj|replace:".:"|replace:"/:"|replace:"-:" }}|{{ empresa.nome }}|
{% for op in operations|where:"uf:SP" %}|1100|SP|{{ op.amount }}|
{% endfor %}|9900|0000|1|
|9900|1100|{{ operations|where:"uf:SP"|count:"tipo_reg:1100" }}|
|9999|{{ total }}|`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"empresa": map[string]any{
			"cnpj": "12.345.678/0001-99",
			"nome": "EMPRESA TESTE LTDA",
		},
		"operations": []map[string]any{
			{"uf": "SP", "tipo_reg": "1100", "amount": "1000.00"},
			{"uf": "RJ", "tipo_reg": "1100", "amount": "500.00"},
			{"uf": "SP", "tipo_reg": "1100", "amount": "750.00"},
		},
		"total": 5,
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)

	// Verify key parts of the output
	assert.Contains(t, out, "|0000|12345678000199|EMPRESA TESTE LTDA|")
	assert.Contains(t, out, "|1100|SP|1000.00|")
	assert.Contains(t, out, "|1100|SP|750.00|")
	assert.Contains(t, out, "|9900|0000|1|")
	assert.Contains(t, out, "|9900|1100|2|") // 2 SP operations
	assert.Contains(t, out, "|9999|5|")
}

func TestDIMPFilters_Record9900Generation(t *testing.T) {
	t.Parallel()
	// DIMP 9900 record: count each record type
	tplStr := `|9900|0000|{{ records|count:"tipo:0000" }}|
|9900|1100|{{ records|count:"tipo:1100" }}|
|9900|1110|{{ records|count:"tipo:1110" }}|
|9900|9900|{{ records|count:"tipo:9900" }}|
|9900|9999|{{ records|count:"tipo:9999" }}|`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"records": []map[string]any{
			{"tipo": "0000"},
			{"tipo": "1100"},
			{"tipo": "1100"},
			{"tipo": "1100"},
			{"tipo": "1110"},
			{"tipo": "1110"},
			{"tipo": "9900"},
			{"tipo": "9900"},
			{"tipo": "9900"},
			{"tipo": "9900"},
			{"tipo": "9900"},
			{"tipo": "9999"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "|9900|0000|1|")
	assert.Contains(t, out, "|9900|1100|3|")
	assert.Contains(t, out, "|9900|1110|2|")
	assert.Contains(t, out, "|9900|9900|5|")
	assert.Contains(t, out, "|9900|9999|1|")
}

func TestDIMPFilters_SumByMultipleUFs(t *testing.T) {
	t.Parallel()
	// DIMP use case: sum by each UF
	tplSP := `{{ ops|where:"uf:SP"|sum:"valor" }}`
	tplRJ := `{{ ops|where:"uf:RJ"|sum:"valor" }}`
	tplMG := `{{ ops|where:"uf:MG"|sum:"valor" }}`

	tplSPCompiled, _ := SafeFromString(tplSP)
	tplRJCompiled, _ := SafeFromString(tplRJ)
	tplMGCompiled, _ := SafeFromString(tplMG)

	ctx := pongo2.Context{
		"ops": []map[string]any{
			{"uf": "SP", "valor": 100.50},
			{"uf": "RJ", "valor": 200.25},
			{"uf": "SP", "valor": 300.75},
			{"uf": "MG", "valor": 150.00},
			{"uf": "RJ", "valor": 50.00},
		},
	}

	outSP, _ := tplSPCompiled.Execute(ctx)
	outRJ, _ := tplRJCompiled.Execute(ctx)
	outMG, _ := tplMGCompiled.Execute(ctx)

	assert.Equal(t, "401.25", outSP) // 100.50 + 300.75
	assert.Equal(t, "250.25", outRJ) // 200.25 + 50.00
	assert.Equal(t, "150", outMG)    // 150.00
}

func TestDIMPFilters_ChainAllFilters(t *testing.T) {
	t.Parallel()
	// Chain all 4 filters in one template
	tplStr := `CNPJ:{{ data.cnpj|replace:".:"|replace:"/:"|replace:"-:" }}|COUNT:{{ data.ops|where:"active:true"|count:"type:A" }}|SUM:{{ data.ops|where:"active:true"|sum:"value" }}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": map[string]any{
			"cnpj": "11.222.333/0001-44",
			"ops": []map[string]any{
				{"active": "true", "type": "A", "value": 100},
				{"active": "false", "type": "A", "value": 200},
				{"active": "true", "type": "B", "value": 300},
				{"active": "true", "type": "A", "value": 400},
			},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)

	assert.Equal(t, "CNPJ:11222333000144|COUNT:2|SUM:800", out)
}
