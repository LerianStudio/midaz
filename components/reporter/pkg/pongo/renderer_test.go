// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *zap.Logger {
	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		return &zap.Logger{}
	}

	return logger
}

func TestRenderFromBytes_Success(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte("Hello, {{ person._.0.name }}!")

	data := map[string]map[string][]map[string]any{
		"person": {
			"_": {
				{"name": "World"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", out)
}

func TestRenderFromBytes_SyntaxError(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte("Hello, {{ name !")
	data := map[string]map[string][]map[string]any{
		"name": {
			"_": {
				{"name": "World"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.Error(t, err)
	assert.Empty(t, out)
}

func TestRender_ArithmeticExpression(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`Initial Balance: {{ midaz_transaction.balance.0.initial_balance }}
Final Balance: {{ midaz_transaction.balance.0.final_balance }}
Calculation: {% calc (100 + 200) * 1.2 %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"initial_balance": 100.0, "final_balance": 200.0},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Initial Balance: 100")
	assert.Contains(t, out, "Final Balance: 200")
	assert.Contains(t, out, "Calculation: 360")
}

func TestRender_ArithmeticExpressionWithVariables(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`Initial Balance: {{ midaz_transaction.balance.0.initial_balance }}
Final Balance: {{ midaz_transaction.balance.0.final_balance }}
Calculation: {% calc (midaz_transaction.balance.0.initial_balance + midaz_transaction.balance.0.final_balance) * 1.2 %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"initial_balance": 100.0, "final_balance": 200.0},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Initial Balance: 100")
	assert.Contains(t, out, "Final Balance: 200")
	assert.Contains(t, out, "Calculation: 360")
}

func TestRender_CalcTag(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`{% calc midaz_transaction.balance.0.initial_balance + midaz_transaction.balance.0.final_balance %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"initial_balance": 100.0, "final_balance": 200.0},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "300")
}

func TestRender_CalcTagWithEmptyValue(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`{% for balance in midaz_transaction.balance %}
Alias: {{ balance.alias }}
Balance: {{ balance.available }}
Sum: {% calc balance.available + 1.2 %}
{% endfor %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"alias": "Account1", "available": ""},
				{"alias": "Account2", "available": "100.5"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Alias: Account1")
	assert.Contains(t, out, "Balance: ")
	assert.Contains(t, out, "Sum: 1.2")
	assert.Contains(t, out, "Alias: Account2")
	assert.Contains(t, out, "Balance: 100.5")
	assert.Contains(t, out, "Sum: 101.7")
}

func TestRender_CalcTagComplexExpression(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`{% for balance in midaz_transaction.balance %}
Alias: {{ balance.alias }}
Balance: {{ balance.available }}
Sum: {% calc balance.available + 1.2 %}
Sum Complex: {% calc (balance.available + 1.2) * balance.on_hold - balance.available / 2 %}
{% endfor %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"alias": "Account1", "available": "0", "on_hold": "3.50"},
				{"alias": "Account2", "available": "100.5", "on_hold": "2.0"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Alias: Account1")
	assert.Contains(t, out, "Balance: 0")
	assert.Contains(t, out, "Sum: 1.2")
	assert.Contains(t, out, "Sum Complex: 4.2")
	assert.Contains(t, out, "Alias: Account2")
	assert.Contains(t, out, "Balance: 100.5")
	assert.Contains(t, out, "Sum: 101.7")
	assert.Contains(t, out, "Sum Complex: 153.15")
}

func TestRender_CalcTagPowerOperation(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`{% for balance in midaz_transaction.balance %}
Alias: {{ balance.alias }}
Balance: {{ balance.available }}
Power: {% calc balance.available ** 2 %}
{% endfor %}

Sum: {% calc (midaz_transaction.balance.3.available + 1.2) ** 2 %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"alias": "Account1", "available": "3"},
				{"alias": "Account2", "available": "4"},
				{"alias": "Account3", "available": "5"},
				{"alias": "Account4", "available": "10"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Alias: Account1")
	assert.Contains(t, out, "Balance: 3")
	assert.Contains(t, out, "Power: 9")
	assert.Contains(t, out, "Alias: Account2")
	assert.Contains(t, out, "Balance: 4")
	assert.Contains(t, out, "Power: 16")
	assert.Contains(t, out, "Alias: Account3")
	assert.Contains(t, out, "Balance: 5")
	assert.Contains(t, out, "Power: 25")
	assert.True(t, strings.Contains(out, "Sum: 125.4") || strings.Contains(out, "Sum: 125.44") || strings.Contains(out, "Sum: 125.43999999999998"))
}

func TestRender_CalcTagIndexAccess(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`{% for balance in midaz_transaction.balance %}
Alias: {{ balance.alias }}
Balance: {{ balance.available }}
Sum: {% calc balance.available + 1.2 %}
Sum Complex: {% calc (balance.available + 1.2) * balance.on_hold - balance.available / 2 %}
{% endfor %}

Sum: {% calc (midaz_transaction.balance.3.available + 1.2) ** 2 %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"alias": "Account1", "available": "0", "on_hold": "3.50"}, // Only 3 elements, index 3 doesn't exist
				{"alias": "Account2", "available": "100.5", "on_hold": "2.0"},
				{"alias": "Account3", "available": "5", "on_hold": "1.0"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Alias: Account1")
	assert.Contains(t, out, "Alias: Account2")
	assert.Contains(t, out, "Alias: Account3")
	assert.Contains(t, out, "Sum: 1.44")
}

func TestRender_CalcTagScientificNotation(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()
	tpl := []byte(`Power Small: {% calc 0.1 ** 3 %}
Power Large: {% calc 1000 ** 2 %}
Power Fractional: {% calc 2.5 ** 0.5 %}`)

	data := map[string]map[string][]map[string]any{
		"midaz_transaction": {
			"balance": {
				{"alias": "Account1", "available": "0.1"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Power Small: 0.001")
	assert.Contains(t, out, "Power Large: 1000000")
	assert.Contains(t, out, "Power Fractional: 1.5811388301")
}

func TestPreprocessSchemaReferences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "converts schema syntax in for loop",
			input:    `{% for tx in external_db:sales.orders %}{{ tx.id }}{% endfor %}`,
			expected: `{% for tx in external_db.sales__orders %}{{ tx.id }}{% endfor %}`,
		},
		{
			name:     "converts multiple schema references",
			input:    `{% for tx in db1:schema1.table1 %}{% endfor %}{% for acc in db2:schema2.table2 %}{% endfor %}`,
			expected: `{% for tx in db1.schema1__table1 %}{% endfor %}{% for acc in db2.schema2__table2 %}{% endfor %}`,
		},
		{
			name:     "preserves legacy format",
			input:    `{% for tx in midaz_transaction.balance %}{{ tx.amount }}{% endfor %}`,
			expected: `{% for tx in midaz_transaction.balance %}{{ tx.amount }}{% endfor %}`,
		},
		{
			name:     "handles mixed formats",
			input:    `{% for tx in external_db:sales.orders %}{% endfor %}{% for acc in midaz.account %}{% endfor %}`,
			expected: `{% for tx in external_db.sales__orders %}{% endfor %}{% for acc in midaz.account %}{% endfor %}`,
		},
		{
			name:     "converts direct access with index",
			input:    `{{ external_db:sales.orders.0.id }}`,
			expected: `{{ external_db.sales__orders.0.id }}`,
		},
		{
			name:     "handles schema in calc expression",
			input:    `{% calc external_db:sales.orders.0.amount + external_db:sales.orders.1.amount %}`,
			expected: `{% calc external_db.sales__orders.0.amount + external_db.sales__orders.1.amount %}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := preprocessSchemaReferences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRender_ExplicitSchemaFormat(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	// Template uses explicit schema syntax that will be preprocessed to sales__orders
	tpl := []byte(`{% for order in external_db:sales.orders %}
ID: {{ order.id }}, Amount: {{ order.amount }}
{% endfor %}`)

	// Data is stored using double underscore key (schema__table) for Pongo2 compatibility
	data := map[string]map[string][]map[string]any{
		"external_db": {
			"sales__orders": {
				{"id": "TX001", "amount": 100.50},
				{"id": "TX002", "amount": 200.00},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "ID: TX001, Amount: 100.5")
	assert.Contains(t, out, "ID: TX002, Amount: 200")
}

func TestRender_ExplicitSchemaDirectAccess(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	// Direct access to schema-qualified data with index
	tpl := []byte(`First Order ID: {{ external_db:sales.orders.0.id }}
First Amount: {{ external_db:sales.orders.0.amount }}`)

	data := map[string]map[string][]map[string]any{
		"external_db": {
			"sales__orders": {
				{"id": "TX001", "amount": 100.50},
				{"id": "TX002", "amount": 200.00},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "First Order ID: TX001")
	assert.Contains(t, out, "First Amount: 100.5")
}

func TestRender_ExplicitSchemaIfTag(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% if external_db:sales.orders %}Has orders{% endif %}`)

	data := map[string]map[string][]map[string]any{
		"external_db": {
			"sales__orders": {
				{"id": "TX001", "amount": 100.50},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Has orders")
}

func TestRender_ExplicitSchemaCalcTag(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`Total: {% calc external_db:sales.orders.0.amount + external_db:sales.orders.1.amount %}`)

	data := map[string]map[string][]map[string]any{
		"external_db": {
			"sales__orders": {
				{"id": "TX001", "amount": 100.50},
				{"id": "TX002", "amount": 200.00},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Total: 300.5")
}

func TestRender_MixedLegacyAndSchemaFormats(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`Legacy: {{ midaz.account.0.alias }}
Schema: {{ external_db:sales.orders.0.id }}`)

	data := map[string]map[string][]map[string]any{
		"midaz": {
			"account": {
				{"alias": "ACCT001"},
			},
		},
		"external_db": {
			"sales__orders": {
				{"id": "TX001"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Legacy: ACCT001")
	assert.Contains(t, out, "Schema: TX001")
}

func TestRenderFromBytes_FilterFunction_MatchingItems(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% for item in filter(db._, "status", "active") %}{{ item.name }},{% endfor %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "Alice", "status": "active"},
				{"name": "Bob", "status": "inactive"},
				{"name": "Charlie", "status": "active"},
				{"name": "Diana", "status": "pending"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "Alice")
	assert.Contains(t, out, "Charlie")
	assert.NotContains(t, out, "Bob")
	assert.NotContains(t, out, "Diana")
}

func TestRenderFromBytes_FilterFunction_NoMatches(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% for item in filter(db._, "status", "deleted") %}{{ item.name }},{% endfor %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "Alice", "status": "active"},
				{"name": "Bob", "status": "inactive"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestRenderFromBytes_FilterFunction_InvalidType(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% for item in filter(db._.0.name, "x", "y") %}{{ item }}{% endfor %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "hello"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestRenderFromBytes_ContainsFunction(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% if contains(db._.0.name, "john") %}YES{% else %}NO{% endif %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "Johnny"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "YES")
}

func TestRenderFromBytes_ContainsFunction_NoMatch(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% if contains(db._.0.name, "john") %}YES{% else %}NO{% endif %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "Alice"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.NoError(t, err)
	assert.Contains(t, out, "NO")
}

func TestRenderFromBytes_ExecutionError(t *testing.T) {
	t.Parallel()
	r := NewTemplateRenderer()
	logger := newTestLogger()

	tpl := []byte(`{% include "nonexistent_file.html" %}`)

	data := map[string]map[string][]map[string]any{
		"db": {
			"_": {
				{"name": "test"},
			},
		},
	}

	out, err := r.RenderFromBytes(context.Background(), tpl, data, logger)
	require.Error(t, err)
	assert.Empty(t, out)
}

func TestCleanNumericOutput_WithXMLDeclaration(t *testing.T) {
	t.Parallel()

	input := `<?xml version="1.0" encoding="UTF-8"?><data>100.50</data>`
	result := cleanNumericOutput(input)
	assert.Equal(t, `<?xml version="1.0" encoding="UTF-8"?><data>100.5</data>`, result)
}

func TestCleanNumericOutput_MultipleXMLDeclarations(t *testing.T) {
	t.Parallel()

	input := `<?xml version="1.0" encoding="UTF-8"?><root>50.100</root><?xml version="1.1" encoding="UTF-8"?><other>200.300</other>`
	result := cleanNumericOutput(input)
	assert.Contains(t, result, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, result, `<?xml version="1.1" encoding="UTF-8"?>`)
	assert.Contains(t, result, "50.1")
	assert.Contains(t, result, "200.3")
	assert.NotContains(t, result, "50.100")
	assert.NotContains(t, result, "200.300")
}

func TestCleanNumericOutput_PreservesMultiDotTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "COSIF code with multiple dots is preserved",
			input:    "1.1.2.00.000",
			expected: "1.1.2.00.000",
		},
		{
			name:     "COSIF code with embedded 1.10 segment is preserved",
			input:    "1.4.1.10.005",
			expected: "1.4.1.10.005",
		},
		{
			name:     "COSIF code surrounded by CSV separators is preserved",
			input:    ";1.1.2.00.000;",
			expected: ";1.1.2.00.000;",
		},
		{
			name:     "version-like string with trailing zero is preserved",
			input:    "v1.10.0",
			expected: "v1.10.0",
		},
		{
			name:     "two adjacent legitimate decimals are both cleaned",
			input:    ";2.00;1.10;",
			expected: ";2;1.1;",
		},
		{
			name:     "legitimate decimal next to COSIF code: only decimal cleaned",
			input:    "VALOR=2.00;COD=1.1.2.00.000",
			expected: "VALOR=2;COD=1.1.2.00.000",
		},
		{
			name:     "decimal at start of input is cleaned",
			input:    "1.10 rest",
			expected: "1.1 rest",
		},
		{
			name:     "decimal at end of input is cleaned",
			input:    "rest 1.10",
			expected: "rest 1.1",
		},
		{
			name:     "integer with no decimals is unchanged",
			input:    "100",
			expected: "100",
		},
		{
			name:     "decimal without trailing zeros is unchanged",
			input:    "1.5",
			expected: "1.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := cleanNumericOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCleanNumericString_DecimalParseFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "invalid decimal with multiple dots falls through to TrimRight",
			input:    "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "valid decimal with trailing zeros parsed by decimal library",
			input:    "1.200",
			expected: "1.2",
		},
		{
			name:     "invalid decimal with trailing zeros exercises TrimRight path",
			input:    "1.2.300",
			expected: "1.2.3",
		},
		{
			name:     "invalid decimal that reduces to integer after TrimRight",
			input:    "1.0.0",
			expected: "1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cleanNumericString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
