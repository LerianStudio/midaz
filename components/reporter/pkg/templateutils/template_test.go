// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package templateutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
)

func TestGetMimeType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		outputFormat string
		expected     string
	}{
		{
			name:         "xml format",
			outputFormat: "xml",
			expected:     "application/xml",
		},
		{
			name:         "html format",
			outputFormat: "html",
			expected:     "text/html",
		},
		{
			name:         "csv format",
			outputFormat: "csv",
			expected:     "text/csv",
		},
		{
			name:         "txt format",
			outputFormat: "txt",
			expected:     "text/plain",
		},
		{
			name:         "pdf format",
			outputFormat: "pdf",
			expected:     "application/pdf",
		},
		{
			name:         "unknown format falls back to octet-stream",
			outputFormat: "unknown",
			expected:     "application/octet-stream",
		},
		{
			name:         "empty string falls back to octet-stream",
			outputFormat: "",
			expected:     "application/octet-stream",
		},
		{
			name:         "uppercase XML is case-insensitive",
			outputFormat: "XML",
			expected:     "application/xml",
		},
		{
			name:         "mixed case Html is case-insensitive",
			outputFormat: "Html",
			expected:     "text/html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetMimeType(tt.outputFormat)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegexBlockForWithFilterOnPlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		templateFile   string
		wantVariable   string
		wantPath       []string
		wantFieldInMap bool
		wantField      string
	}{
		{
			name:           "for loop with filter function and single param",
			templateFile:   `{% for acc in filter(midaz_onboarding.account, "type") %}{{ acc.id }}{% endfor %}`,
			wantVariable:   "acc",
			wantPath:       []string{"midaz_onboarding", "account"},
			wantFieldInMap: true,
			wantField:      "type",
		},
		{
			name:           "for loop with filter function and multiple params",
			templateFile:   `{% for item in filter(datasource.collection, "field1", "field2") %}{{ item.name }}{% endfor %}`,
			wantVariable:   "item",
			wantPath:       []string{"datasource", "collection"},
			wantFieldInMap: true,
			wantField:      "field1",
		},
		{
			name:           "no filter function does not match",
			templateFile:   `{% for item in datasource.collection %}{{ item.name }}{% endfor %}`,
			wantVariable:   "",
			wantPath:       nil,
			wantFieldInMap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			variableMap := map[string][]string{}
			result := map[string]any{}

			regexBlockForWithFilterOnPlaceholder(result, variableMap, tt.templateFile)

			if tt.wantVariable == "" {
				assert.Empty(t, variableMap)
				return
			}

			assert.Contains(t, variableMap, tt.wantVariable)
			assert.Equal(t, tt.wantPath, variableMap[tt.wantVariable])

			if tt.wantFieldInMap {
				dsMap, ok := result[tt.wantPath[0]].(map[string]any)
				require.True(t, ok, "expected datasource map to exist")

				fields, ok := dsMap[tt.wantPath[1]].([]any)
				require.True(t, ok, "expected collection fields to exist")

				assert.Contains(t, fields, tt.wantField)
			}
		})
	}
}

func TestRegexBlockWithOnPlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		templateFile string
		wantVariable string
		wantPath     []string
	}{
		{
			name:         "simple with assignment",
			templateFile: `{% with x = datasource.table %}{{ x }}{% endwith %}`,
			wantVariable: "x",
			wantPath:     []string{"datasource", "table"},
		},
		{
			name:         "with filter function",
			templateFile: `{% with x = filter(datasource.table, "param") %}{{ x }}{% endwith %}`,
			wantVariable: "x",
			wantPath:     []string{"datasource", "table"},
		},
		{
			name:         "with nested path",
			templateFile: `{% with data = external_db.orders %}{{ data.id }}{% endwith %}`,
			wantVariable: "data",
			wantPath:     []string{"external_db", "orders"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			variableMap := map[string][]string{}
			result := regexBlockWithOnPlaceholder(variableMap, tt.templateFile)

			assert.NotNil(t, result)
			assert.Contains(t, variableMap, tt.wantVariable)
			assert.Equal(t, tt.wantPath, variableMap[tt.wantVariable])
		})
	}
}

func TestProcessDIMPForLoops(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		templateFile    string
		wantDatasource  string
		wantCollection  string
		wantFields      []string
		expectNoResults bool
	}{
		{
			name:           "where filter in for loop",
			templateFile:   `{% for item in midaz_onboarding.account|where:"type:cacc" %}{{ item.id }}{% endfor %}`,
			wantDatasource: "midaz_onboarding",
			wantCollection: "account",
			wantFields:     []string{"type"},
		},
		{
			name:           "sum filter in for loop",
			templateFile:   `{% for item in midaz_transaction.entries|sum:"amount" %}{{ item.id }}{% endfor %}`,
			wantDatasource: "midaz_transaction",
			wantCollection: "entries",
			wantFields:     []string{"amount"},
		},
		{
			name:           "count filter in for loop",
			templateFile:   `{% for item in datasource.records|count:"status:active" %}{{ item.id }}{% endfor %}`,
			wantDatasource: "datasource",
			wantCollection: "records",
			wantFields:     []string{"status"},
		},
		{
			name:            "no filter does not match",
			templateFile:    `{% for item in datasource.records %}{{ item.id }}{% endfor %}`,
			expectNoResults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resultRegex := map[string]any{}
			processDIMPForLoops(tt.templateFile, resultRegex)

			if tt.expectNoResults {
				assert.Empty(t, resultRegex)
				return
			}

			assert.Contains(t, resultRegex, tt.wantDatasource)

			dsMap, ok := resultRegex[tt.wantDatasource].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, dsMap, tt.wantCollection)

			fields, ok := dsMap[tt.wantCollection].([]any)
			require.True(t, ok)

			for _, wantField := range tt.wantFields {
				assert.Contains(t, fields, wantField)
			}
		})
	}
}

func TestExtractDIMPBasePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expr        string
		variableMap map[string][]string
		expected    []string
	}{
		{
			name:        "direct datasource.collection path",
			expr:        `midaz_data.records|where:"active:true"`,
			variableMap: map[string][]string{},
			expected:    []string{"midaz_data", "records"},
		},
		{
			name: "loop variable resolves to path",
			expr: `item|where:"status:active"`,
			variableMap: map[string][]string{
				"item": {"datasource", "collection"},
			},
			expected: []string{"datasource", "collection"},
		},
		{
			name:        "no pipe returns nil",
			expr:        `justanexpression`,
			variableMap: map[string][]string{},
			expected:    nil,
		},
		{
			name:        "single part before pipe returns nil",
			expr:        `single|where:"f:v"`,
			variableMap: map[string][]string{},
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractDIMPBasePath(tt.expr, tt.variableMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFieldsFromDIMPFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expr       string
		basePath   []string
		wantFields []string
	}{
		{
			name:       "where filter extracts field",
			expr:       `midaz.records|where:"status:active"`,
			basePath:   []string{"midaz", "records"},
			wantFields: []string{"status"},
		},
		{
			name:       "sum filter extracts field",
			expr:       `midaz.records|sum:"amount"`,
			basePath:   []string{"midaz", "records"},
			wantFields: []string{"amount"},
		},
		{
			name:       "count filter extracts field",
			expr:       `midaz.records|count:"type:A"`,
			basePath:   []string{"midaz", "records"},
			wantFields: []string{"type"},
		},
		{
			name:       "chained filters extract multiple fields",
			expr:       `midaz.records|where:"status:active"|sum:"total"`,
			basePath:   []string{"midaz", "records"},
			wantFields: []string{"status", "total"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resultRegex := map[string]any{}
			ensureMapStructure(resultRegex, tt.basePath)
			extractFieldsFromDIMPFilters(tt.expr, resultRegex, tt.basePath)

			dsMap, ok := resultRegex[tt.basePath[0]].(map[string]any)
			require.True(t, ok)

			fields, ok := dsMap[tt.basePath[1]].([]any)
			require.True(t, ok)

			for _, wantField := range tt.wantFields {
				assert.Contains(t, fields, wantField)
			}
		})
	}
}

func TestProcessDIMPExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		templateFile    string
		variableMap     map[string][]string
		wantDatasource  string
		wantCollection  string
		wantFields      []string
		expectNoResults bool
	}{
		{
			name:           "expression with where filter",
			templateFile:   `{{ midaz_data.records|where:"field:value" }}`,
			variableMap:    map[string][]string{},
			wantDatasource: "midaz_data",
			wantCollection: "records",
			wantFields:     []string{"field"},
		},
		{
			name:           "expression with sum filter",
			templateFile:   `{{ midaz_data.entries|sum:"amount" }}`,
			variableMap:    map[string][]string{},
			wantDatasource: "midaz_data",
			wantCollection: "entries",
			wantFields:     []string{"amount"},
		},
		{
			name:           "expression with count filter",
			templateFile:   `{{ midaz_data.entries|count:"type:A" }}`,
			variableMap:    map[string][]string{},
			wantDatasource: "midaz_data",
			wantCollection: "entries",
			wantFields:     []string{"type"},
		},
		{
			name:            "expression without DIMP filter is ignored",
			templateFile:    `{{ midaz_data.entries }}`,
			variableMap:     map[string][]string{},
			expectNoResults: true,
		},
		{
			name:         "expression referencing loop variable",
			templateFile: `{{ item|where:"status:active" }}`,
			variableMap: map[string][]string{
				"item": {"datasource", "collection"},
			},
			wantDatasource: "datasource",
			wantCollection: "collection",
			wantFields:     []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resultRegex := map[string]any{}
			processDIMPExpressions(tt.templateFile, resultRegex, tt.variableMap)

			if tt.expectNoResults {
				assert.Empty(t, resultRegex)
				return
			}

			assert.Contains(t, resultRegex, tt.wantDatasource)

			dsMap, ok := resultRegex[tt.wantDatasource].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, dsMap, tt.wantCollection)

			fields, ok := dsMap[tt.wantCollection].([]any)
			require.True(t, ok)

			for _, wantField := range tt.wantFields {
				assert.Contains(t, fields, wantField)
			}
		})
	}
}

func TestExtractBasePathFromFilterExpr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "expression with where filter",
			expr:     `midaz_onboarding.account|where:"type:cacc"`,
			expected: "midaz_onboarding.account",
		},
		{
			name:     "expression with sum filter",
			expr:     `midaz_transaction.entries|sum:"amount"`,
			expected: "midaz_transaction.entries",
		},
		{
			name:     "expression without filter",
			expr:     "midaz_onboarding.account",
			expected: "midaz_onboarding.account",
		},
		{
			name:     "expression with spaces around pipe",
			expr:     ` midaz_data.records |where:"active:true" `,
			expected: "midaz_data.records",
		},
		{
			name:     "single word without pipe",
			expr:     "variable",
			expected: "variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractBasePathFromFilterExpr(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnsureMapStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		path           []string
		expectCreated  bool
		wantDatasource string
		wantCollection string
	}{
		{
			name:           "creates nested structure for valid path",
			path:           []string{"datasource", "collection"},
			expectCreated:  true,
			wantDatasource: "datasource",
			wantCollection: "collection",
		},
		{
			name:          "path too short does nothing",
			path:          []string{"single"},
			expectCreated: false,
		},
		{
			name:          "empty path does nothing",
			path:          []string{},
			expectCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := map[string]any{}
			ensureMapStructure(m, tt.path)

			if !tt.expectCreated {
				assert.Empty(t, m)
				return
			}

			assert.Contains(t, m, tt.wantDatasource)

			dsMap, ok := m[tt.wantDatasource].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, dsMap, tt.wantCollection)
		})
	}
}

func TestInsertFieldToPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       []string
		field      string
		wantField  bool
		setupExist bool
	}{
		{
			name:       "inserts field to existing structure",
			path:       []string{"datasource", "collection"},
			field:      "status",
			wantField:  true,
			setupExist: true,
		},
		{
			name:      "path too short does nothing",
			path:      []string{"single"},
			field:     "field",
			wantField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := map[string]any{}

			if tt.setupExist {
				ensureMapStructure(m, tt.path)
			}

			insertFieldToPath(m, tt.path, tt.field)

			if !tt.wantField {
				assert.Empty(t, m)
				return
			}

			dsMap := m[tt.path[0]].(map[string]any)
			fields := dsMap[tt.path[1]].([]any)
			assert.Contains(t, fields, tt.field)
		})
	}
}

func TestContainsDIMPFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "contains where filter",
			expr:     `collection|where:"field:value"`,
			expected: true,
		},
		{
			name:     "contains sum filter",
			expr:     `collection|sum:"amount"`,
			expected: true,
		},
		{
			name:     "contains count filter",
			expr:     `collection|count:"type:A"`,
			expected: true,
		},
		{
			name:     "no filter",
			expr:     "collection.field",
			expected: false,
		},
		{
			name:     "empty string",
			expr:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := containsDIMPFilter(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsQuotedString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "double quoted string",
			input:    `"hello"`,
			expected: true,
		},
		{
			name:     "single quoted string",
			input:    `'hello'`,
			expected: true,
		},
		{
			name:     "unquoted string",
			input:    "hello",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single char",
			input:    `"`,
			expected: false,
		},
		{
			name:     "mismatched quotes",
			input:    `"hello'`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isQuotedString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFieldFromFilterArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filterType string
		filterArg  string
		wantField  string
		wantInsert bool
	}{
		{
			name:       "where filter extracts field before colon",
			filterType: "where",
			filterArg:  "status:active",
			wantField:  "status",
			wantInsert: true,
		},
		{
			name:       "count filter extracts field before colon",
			filterType: "count",
			filterArg:  "type:A",
			wantField:  "type",
			wantInsert: true,
		},
		{
			name:       "sum filter extracts entire arg as field",
			filterType: "sum",
			filterArg:  "amount",
			wantField:  "amount",
			wantInsert: true,
		},
		{
			name:       "where filter with no colon does not insert",
			filterType: "where",
			filterArg:  "nocolon",
			wantInsert: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			basePath := []string{"ds", "coll"}
			resultRegex := map[string]any{}
			ensureMapStructure(resultRegex, basePath)

			extractFieldFromFilterArg(resultRegex, basePath, tt.filterType, tt.filterArg)

			dsMap, ok := resultRegex["ds"].(map[string]any)
			require.True(t, ok)

			fields, ok := dsMap["coll"].([]any)
			require.True(t, ok)

			if tt.wantInsert {
				assert.Contains(t, fields, tt.wantField)
			} else {
				assert.Empty(t, fields)
			}
		})
	}
}

func TestNormalizeStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]map[string][]string
	}{
		{
			name: "simple structure with string fields",
			input: map[string]any{
				"datasource": map[string]any{
					"collection": []any{"field1", "field2"},
				},
			},
			expected: map[string]map[string][]string{
				"datasource": {
					"collection": {"field1", "field2"},
				},
			},
		},
		{
			name:     "empty input returns empty result",
			input:    map[string]any{},
			expected: map[string]map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := normalizeStructure(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlattenNestedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]any
		prefix   string
		expected []string
	}{
		{
			name: "flat fields without prefix",
			input: map[string]any{
				"field1": "value1",
			},
			prefix:   "",
			expected: []string{"field1"},
		},
		{
			name: "nested map with prefix",
			input: map[string]any{
				"nested": map[string]any{
					"deep": "value",
				},
			},
			prefix:   "parent",
			expected: []string{"parent.nested.deep"},
		},
		{
			name: "array with string items",
			input: map[string]any{
				"items": []any{"sub1", "sub2"},
			},
			prefix:   "",
			expected: []string{"items", "items.sub1", "items.sub2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := flattenNestedFields(tt.input, tt.prefix)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestGetMapKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name:     "multiple keys",
			input:    map[string]any{"a": 1, "b": 2, "c": 3},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name:     "single key",
			input:    map[string]any{"only": true},
			expected: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getMapKeys(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestMappedFieldsOfTemplate_CalcTag(t *testing.T) {
	t.Parallel()

	template := `{% for balance in midaz_transaction.balance %}
Alias: {{ balance.alias }}
Balance: {{ balance.available }}
Sum: {% calc balance.available + 1.2 %}
Sum Complex: {% calc (balance.available + 1.2) * balance.on_hold - balance.available / 2 %}
{% endfor %}

Sum: {% calc midaz_transaction.balance.3.available + 1.2 %}`

	result := MappedFieldsOfTemplate(template)

	// Verify that all required fields are mapped
	assert.NotNil(t, result)
	assert.Contains(t, result, "midaz_transaction")

	midazTransaction := result["midaz_transaction"]
	assert.Contains(t, midazTransaction, "balance")

	balanceFields := midazTransaction["balance"]

	// Check that all fields used in the template are mapped
	expectedFields := []string{"alias", "available", "on_hold"}
	for _, field := range expectedFields {
		assert.Contains(t, balanceFields, field, "Field %s should be mapped", field)
	}
}

func TestMappedFieldsOfTemplate_CalcTagComplex(t *testing.T) {
	t.Parallel()

	template := `{% calc (midaz_transaction.balance.0.initial_balance + midaz_transaction.balance.0.final_balance) * 1.2 %}`

	result := MappedFieldsOfTemplate(template)

	assert.NotNil(t, result)
	assert.Contains(t, result, "midaz_transaction")

	midazTransaction := result["midaz_transaction"]
	assert.Contains(t, midazTransaction, "balance")

	balanceFields := midazTransaction["balance"]
	expectedFields := []string{"initial_balance", "final_balance"}

	for _, field := range expectedFields {
		assert.Contains(t, balanceFields, field, "Field %s should be mapped", field)
	}
}

func TestMappedFieldsOfTemplate_DIMPFilters(t *testing.T) {
	t.Parallel()

	template := `|0000|{{ midaz_onboarding.onboarding.0.legal_document|replace:".:"|replace:"/:"|replace:"-:" }}|{{ midaz_onboarding.onboarding.0.legal_name }}|
{% for acc in midaz_onboarding.account|where:"type:cacc" %}|1100|SP|{{ acc.id }}|{{ acc.alias|replace:"@:"|replace:"_:/" }}|
{% endfor %}|TOTAL_SP|{{ midaz_transaction.transaction|where:"status:APPROVED"|sum:"amount" }}|
|9900|1100|{{ midaz_transaction.transaction|count:"tipo:1100" }}|`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	assert.NotNil(t, result)

	// Check midaz_onboarding
	assert.Contains(t, result, "midaz_onboarding")
	midazOnboarding := result["midaz_onboarding"]
	t.Logf("midaz_onboarding: %+v", midazOnboarding)

	// Check onboarding fields
	assert.Contains(t, midazOnboarding, "onboarding")
	onboardingFields := midazOnboarding["onboarding"]
	assert.Contains(t, onboardingFields, "legal_document", "legal_document should be mapped")
	assert.Contains(t, onboardingFields, "legal_name", "legal_name should be mapped")

	// Check account fields (from for loop with where filter)
	assert.Contains(t, midazOnboarding, "account")
	accountFields := midazOnboarding["account"]
	t.Logf("account fields: %+v", accountFields)
	assert.Contains(t, accountFields, "type", "type should be mapped from where filter")
	assert.Contains(t, accountFields, "id", "id should be mapped")
	assert.Contains(t, accountFields, "alias", "alias should be mapped")

	// Check midaz_transaction
	assert.Contains(t, result, "midaz_transaction")
	midazTransaction := result["midaz_transaction"]
	t.Logf("midaz_transaction: %+v", midazTransaction)

	assert.Contains(t, midazTransaction, "transaction")
	transactionFields := midazTransaction["transaction"]
	t.Logf("transaction fields: %+v", transactionFields)
	assert.Contains(t, transactionFields, "status", "status should be mapped from where filter")
	assert.Contains(t, transactionFields, "amount", "amount should be mapped from sum filter")
	assert.Contains(t, transactionFields, "tipo", "tipo should be mapped from count filter")
}

func TestMappedFieldsOfTemplate_WhereFilter(t *testing.T) {
	t.Parallel()

	template := `{{ operations|where:"uf:SP"|sum:"value" }}`

	result := MappedFieldsOfTemplate(template)

	assert.NotNil(t, result)
	// Note: "operations" alone isn't a valid datasource.collection path,
	// but the filter fields should still be captured if we had a proper path
}

func TestMappedFieldsOfTemplate_ChainedFilters(t *testing.T) {
	t.Parallel()

	template := `{{ midaz_data.records|where:"active:true"|where:"type:A"|sum:"amount"|count:"status:done" }}`

	result := MappedFieldsOfTemplate(template)

	assert.NotNil(t, result)
	assert.Contains(t, result, "midaz_data")

	midazData := result["midaz_data"]
	assert.Contains(t, midazData, "records")

	recordsFields := midazData["records"]
	assert.Contains(t, recordsFields, "active", "active should be mapped from first where filter")
	assert.Contains(t, recordsFields, "type", "type should be mapped from second where filter")
	assert.Contains(t, recordsFields, "amount", "amount should be mapped from sum filter")
	assert.Contains(t, recordsFields, "status", "status should be mapped from count filter")
}

func TestMappedFieldsOfTemplate_ForLoopWithWhereFilter(t *testing.T) {
	t.Parallel()

	template := `{% for item in midaz_source.items|where:"category:electronics" %}
{{ item.name }} - {{ item.price }}
{% endfor %}`

	result := MappedFieldsOfTemplate(template)

	assert.NotNil(t, result)
	assert.Contains(t, result, "midaz_source")

	midazSource := result["midaz_source"]
	assert.Contains(t, midazSource, "items")

	itemsFields := midazSource["items"]
	assert.Contains(t, itemsFields, "category", "category should be mapped from where filter in for loop")
	assert.Contains(t, itemsFields, "name", "name should be mapped from loop variable usage")
	assert.Contains(t, itemsFields, "price", "price should be mapped from loop variable usage")
}

func TestMappedFieldsOfTemplate_CountByTagQuotedValues(t *testing.T) {
	t.Parallel()

	template := `{% count_by midaz_onboarding.account if type == "cacc" %}`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	assert.NotNil(t, result)
	assert.Contains(t, result, "midaz_onboarding")

	midazOnboarding := result["midaz_onboarding"]
	assert.Contains(t, midazOnboarding, "account")

	accountFields := midazOnboarding["account"]
	t.Logf("account fields: %+v", accountFields)

	// Should contain "type" (the field being compared)
	assert.Contains(t, accountFields, "type", "type should be mapped from count_by condition")

	// Should NOT contain "cacc" (the quoted string value)
	assert.NotContains(t, accountFields, "cacc", "cacc should NOT be mapped - it's a value, not a field")
	assert.NotContains(t, accountFields, `"cacc"`, `"cacc" should NOT be mapped - it's a quoted value`)
}

func TestMappedFieldsOfTemplate_NestedLoopWithParentVariable(t *testing.T) {
	t.Parallel()

	template := `
{% for alias in plugin_crm.aliases %}
  Alias ID: {{ alias.account_id }}
  {% for related_party in alias.related_parties %}
    Role: {{ related_party.role }}
    Start: {{ related_party.start_date }}
  {% endfor %}
{% endfor %}`

	result := MappedFieldsOfTemplate(template)

	// Should map to plugin_crm
	assert.NotNil(t, result)
	assert.Contains(t, result, "plugin_crm", "plugin_crm should exist in result")

	pluginCRM := result["plugin_crm"]
	assert.Contains(t, pluginCRM, "aliases", "aliases should exist under plugin_crm")

	// Get aliases fields
	aliasesFields := pluginCRM["aliases"]

	// Should contain account_id (direct field from alias)
	assert.Contains(t, aliasesFields, "account_id", "account_id should be in aliases fields")

	// Should contain related_parties (nested loop resolved to same table)
	assert.Contains(t, aliasesFields, "related_parties", "related_parties should be captured under aliases")

	// Should contain nested fields with prefix (related_parties.field)
	assert.Contains(t, aliasesFields, "related_parties.role", "related_parties.role should be in aliases fields (from nested loop)")
	assert.Contains(t, aliasesFields, "related_parties.start_date", "related_parties.start_date should be in aliases fields (from nested loop)")
}

func TestParseDatabaseReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ref            string
		wantDatabase   string
		wantSchema     string
		wantTable      string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:         "legacy format - database.table",
			ref:          "midaz_onboarding.account",
			wantDatabase: "midaz_onboarding",
			wantSchema:   "",
			wantTable:    "account",
			wantErr:      false,
		},
		{
			name:         "new format - database:schema.table",
			ref:          "external_db:sales.orders",
			wantDatabase: "external_db",
			wantSchema:   "sales",
			wantTable:    "orders",
			wantErr:      false,
		},
		{
			name:         "new format with public schema",
			ref:          "midaz:public.users",
			wantDatabase: "midaz",
			wantSchema:   "public",
			wantTable:    "users",
			wantErr:      false,
		},
		{
			name:           "invalid format - no separator",
			ref:            "invalidformat",
			wantErr:        true,
			wantErrContain: "invalid format",
		},
		{
			name:           "invalid format - colon but no dot after",
			ref:            "database:schematable",
			wantErr:        true,
			wantErrContain: "expected schema.table",
		},
		{
			name:         "legacy format with underscore in names",
			ref:          "midaz_transaction.balance_entry",
			wantDatabase: "midaz_transaction",
			wantSchema:   "",
			wantTable:    "balance_entry",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			database, schema, table, err := ParseDatabaseReference(tt.ref)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantDatabase, database, "database mismatch")
			assert.Equal(t, tt.wantSchema, schema, "schema mismatch")
			assert.Equal(t, tt.wantTable, table, "table mismatch")
		})
	}
}

func TestResolveNestedVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string][]string
		expected map[string][]string
	}{
		{
			name: "single level - no resolution needed",
			input: map[string][]string{
				"account": {"midaz_onboarding", "account"},
			},
			expected: map[string][]string{
				"account": {"midaz_onboarding", "account"},
			},
		},
		{
			name: "nested loop - resolve parent variable",
			input: map[string][]string{
				"alias":         {"plugin_crm", "aliases"},
				"related_party": {"alias", "related_parties"},
			},
			expected: map[string][]string{
				"alias":         {"plugin_crm", "aliases"},
				"related_party": {"plugin_crm", "aliases", "related_parties"},
			},
		},
		{
			name: "triple nested - resolve chain",
			input: map[string][]string{
				"account":     {"midaz", "accounts"},
				"transaction": {"account", "transactions"},
				"entry":       {"transaction", "entries"},
			},
			expected: map[string][]string{
				"account":     {"midaz", "accounts"},
				"transaction": {"midaz", "accounts", "transactions"},
				"entry":       {"midaz", "accounts", "transactions", "entries"},
			},
		},
		{
			name: "independent loops - no resolution",
			input: map[string][]string{
				"account": {"midaz_onboarding", "account"},
				"balance": {"midaz_transaction", "balance"},
			},
			expected: map[string][]string{
				"account": {"midaz_onboarding", "account"},
				"balance": {"midaz_transaction", "balance"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Make a copy to avoid modifying test data
			input := make(map[string][]string)
			for k, v := range tt.input {
				input[k] = append([]string{}, v...)
			}

			resolveNestedVariables(input)

			for varName, expectedPath := range tt.expected {
				assert.Equal(t, expectedPath, input[varName], "Variable %s should have correct resolved path", varName)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "legacy format - database.table.field",
			path:     "midaz_transaction.balance.amount",
			expected: []string{"midaz_transaction", "balance", "amount"},
		},
		{
			name:     "legacy format with array index",
			path:     "midaz_transaction.balance.0.amount",
			expected: []string{"midaz_transaction", "balance", "amount"},
		},
		{
			name:     "schema format - database:schema.table.field",
			path:     "external_db:sales.orders.amount",
			expected: []string{"external_db", "sales__orders", "amount"},
		},
		{
			name:     "schema format with array index",
			path:     "external_db:sales.orders.0.amount",
			expected: []string{"external_db", "sales__orders", "amount"},
		},
		{
			name:     "schema format - public schema",
			path:     "midaz:public.users.name",
			expected: []string{"midaz", "public__users", "name"},
		},
		{
			name:     "simple two parts",
			path:     "database.table",
			expected: []string{"database", "table"},
		},
		{
			name:     "schema format - two parts (database:schema.table)",
			path:     "database:schema.table",
			expected: []string{"database", "schema__table"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := CleanPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMappedFieldsOfTemplate_SchemaFormat(t *testing.T) {
	t.Parallel()

	template := `{% for tx in external_db:sales.orders %}
Transaction ID: {{ tx.id }}
Amount: {{ tx.amount }}
Status: {{ tx.status }}
{% endfor %}`

	result := MappedFieldsOfTemplate(template)

	assert.NotNil(t, result)
	assert.Contains(t, result, "external_db")

	externalDB := result["external_db"]
	assert.Contains(t, externalDB, "sales__orders")

	txFields := externalDB["sales__orders"]
	assert.Contains(t, txFields, "id")
	assert.Contains(t, txFields, "amount")
	assert.Contains(t, txFields, "status")
}

func TestMappedFieldsOfTemplate_MixedFormats(t *testing.T) {
	t.Parallel()

	// Template uses original syntax with dot (database:schema.table)
	template := `{% for acc in midaz_onboarding.account %}
Account: {{ acc.alias }}
{% endfor %}

{% for tx in external_db:sales.orders %}
Amount: {{ tx.amount }}
{% endfor %}`

	result := MappedFieldsOfTemplate(template)

	// Check legacy format
	assert.Contains(t, result, "midaz_onboarding")
	assert.Contains(t, result["midaz_onboarding"], "account")
	assert.Contains(t, result["midaz_onboarding"]["account"], "alias")

	// Check schema format (schema.table becomes schema__table internally)
	assert.Contains(t, result, "external_db")
	assert.Contains(t, result["external_db"], "sales__orders")
	assert.Contains(t, result["external_db"]["sales__orders"], "amount")
}

func TestMappedFieldsOfTemplate_ExplicitSchemaCalcTag(t *testing.T) {
	t.Parallel()

	template := `{% for tx in external_db:sales.orders %}
Amount: {{ tx.amount }}
{% endfor %}
Total: {% calc external_db:sales.orders.0.amount + external_db:sales.orders.1.amount %}`

	result := MappedFieldsOfTemplate(template)

	assert.Contains(t, result, "external_db")
	assert.Contains(t, result["external_db"], "sales__orders")
	assert.Contains(t, result["external_db"]["sales__orders"], "amount")
}

func TestMappedFieldsOfTemplate_ExplicitSchemaIfTag(t *testing.T) {
	t.Parallel()

	template := `{% if external_db:sales.orders.0.status == "completed" %}
Completed: {{ external_db:sales.orders.0.amount }}
{% endif %}`

	result := MappedFieldsOfTemplate(template)

	assert.Contains(t, result, "external_db")
	assert.Contains(t, result["external_db"], "sales__orders")
	assert.Contains(t, result["external_db"]["sales__orders"], "status")
	assert.Contains(t, result["external_db"]["sales__orders"], "amount")
}

func TestExtractIfFromExpression_ExplicitSchemaSyntax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "explicit schema path",
			expr:     "external_db:sales.orders.0.amount",
			expected: []string{"external_db:sales.orders.0.amount"},
		},
		{
			name:     "explicit schema in comparison",
			expr:     "external_db:sales.orders.0.status == 'completed'",
			expected: []string{"external_db:sales.orders.0.status"},
		},
		{
			name:     "mixed formats",
			expr:     "external_db:sales.orders.0.amount + midaz.balance.0.value",
			expected: []string{"external_db:sales.orders.0.amount", "midaz.balance.value"},
		},
		{
			name:     "legacy format preserved",
			expr:     "midaz_transaction.balance.0.amount",
			expected: []string{"midaz_transaction.balance.amount"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractIfFromExpression(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFieldsFromExpression_ExplicitSchemaSyntax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "explicit schema path",
			expr:     "external_db:sales.orders.0.amount",
			expected: []string{"external_db:sales.orders.0.amount"},
		},
		{
			name:     "explicit schema with filter",
			expr:     `external_db:sales.orders|where:"status:completed"`,
			expected: []string{"external_db:sales.orders"},
		},
		{
			name:     "legacy format preserved",
			expr:     "midaz_transaction.balance.0.amount",
			expected: []string{"midaz_transaction.balance.0.amount"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractFieldsFromExpression(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMappedFieldsOfTemplate_AllTagsWithExplicitSchema(t *testing.T) {
	t.Parallel()

	// Test all template tags with explicit schema syntax
	template := `
{# For loop with schema #}
{% for tx in external_db:sales.orders %}
	ID: {{ tx.id }}
	Amount: {{ tx.amount }}
{% endfor %}

{# Variable expression with schema #}
First Order: {{ external_db:sales.orders.0.reference_id }}

{# Calc tag with schema #}
Total: {% calc external_db:sales.orders.0.amount + external_db:sales.orders.1.amount %}

{# If tag with schema #}
{% if external_db:sales.orders.0.status == "completed" %}
Completed!
{% endif %}

{# Set tag with schema #}
{% set total = external_db:sales.orders.0.fee %}

{# For loop with DIMP filter and schema #}
{% for tx in external_db:sales.orders|where:"status:completed" %}
	{{ tx.description }}
{% endfor %}

{# Sum_by aggregation with schema #}
{% sum_by external_db:sales.orders.amount if tx.status == "completed" %}

{# With tag and filter function #}
{% with filtered = filter(external_db:sales.orders, "status", "completed") %}
	{{ filtered }}
{% endwith %}
`

	result := MappedFieldsOfTemplate(template)

	// Verify all fields are extracted for the explicit schema datasource
	// Note: schema.table becomes schema__table for Pongo2 compatibility
	assert.Contains(t, result, "external_db", "Should contain external_db datasource")
	assert.Contains(t, result["external_db"], "sales__orders", "Should contain sales__orders table")

	orderFields := result["external_db"]["sales__orders"]

	expectedFields := []string{"id", "amount", "reference_id", "status", "fee", "description"}
	for _, field := range expectedFields {
		assert.Contains(t, orderFields, field, "Field %s should be mapped", field)
	}
}

func TestMappedFieldsOfTemplate_MixedLegacyAndSchemaFormats(t *testing.T) {
	t.Parallel()

	// Template uses original syntax with dot (database:schema.table)
	template := `
{# Legacy format #}
{% for acc in midaz_onboarding.account %}
	{{ acc.alias }}
{% endfor %}

{# Explicit schema format - user writes schema.table with dot #}
{% for tx in external_db:sales.orders %}
	{{ tx.amount }}
{% endfor %}

{# Both in same calc expression #}
{% calc midaz_onboarding.account.0.balance + external_db:sales.orders.0.amount %}
`

	result := MappedFieldsOfTemplate(template)

	// Check legacy format
	assert.Contains(t, result, "midaz_onboarding")
	assert.Contains(t, result["midaz_onboarding"], "account")
	assert.Contains(t, result["midaz_onboarding"]["account"], "alias")
	assert.Contains(t, result["midaz_onboarding"]["account"], "balance")

	// Check schema format (schema.table becomes schema__table internally)
	assert.Contains(t, result, "external_db")
	assert.Contains(t, result["external_db"], "sales__orders")
	assert.Contains(t, result["external_db"]["sales__orders"], "amount")
}

func TestMappedFieldsOfTemplate_AggregationNestedFields(t *testing.T) {
	t.Parallel()

	// Test that nested JSONB field paths in aggregation "by" clauses
	// are correctly treated as field paths, not datasource references
	template := `
{# Aggregation with nested JSONB field in "by" clause #}
<ValorReceita>{% sum_by external_db:analytics.transfers by "fee_charge.totalAmount" if status == "COMPLETED" %}</ValorReceita>

{# Another aggregation with deeply nested field #}
<TotalFees>{% sum_by external_db:analytics.transactions by "metadata.fees.amount" if type == "PAYMENT" %}</TotalFees>

{# Count with nested field #}
<Count>{% count_by datasource:public.orders by "customer.address.city" if active == "true" %}</Count>

{# Avg with nested field #}
<Average>{% avg_by datasource:public.products by "pricing.discount.percentage" if available == "true" %}</Average>

{# Simple aggregation without nested field (should still work) #}
<SimpleSum>{% sum_by external_db:analytics.sales by "amount" if region == "SOUTH" %}</SimpleSum>
`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	// Check external_db datasource
	assert.Contains(t, result, "external_db")

	// analytics__transfers should contain the nested field path as-is
	assert.Contains(t, result["external_db"], "analytics__transfers")
	transfersFields := result["external_db"]["analytics__transfers"]
	assert.Contains(t, transfersFields, "fee_charge.totalAmount", "Nested field 'fee_charge.totalAmount' should be preserved as-is")
	assert.Contains(t, transfersFields, "status", "Condition field 'status' should be mapped")

	// analytics__transactions should contain deeply nested field path
	assert.Contains(t, result["external_db"], "analytics__transactions")
	transactionsFields := result["external_db"]["analytics__transactions"]
	assert.Contains(t, transactionsFields, "metadata.fees.amount", "Deeply nested field 'metadata.fees.amount' should be preserved as-is")
	assert.Contains(t, transactionsFields, "type", "Condition field 'type' should be mapped")

	// analytics__sales should contain simple field
	assert.Contains(t, result["external_db"], "analytics__sales")
	salesFields := result["external_db"]["analytics__sales"]
	assert.Contains(t, salesFields, "amount", "Simple field 'amount' should be mapped")
	assert.Contains(t, salesFields, "region", "Condition field 'region' should be mapped")

	// Check datasource
	assert.Contains(t, result, "datasource")

	// public__orders should contain nested field
	assert.Contains(t, result["datasource"], "public__orders")
	ordersFields := result["datasource"]["public__orders"]
	assert.Contains(t, ordersFields, "customer.address.city", "Nested field 'customer.address.city' should be preserved as-is")
	assert.Contains(t, ordersFields, "active", "Condition field 'active' should be mapped")

	// public__products should contain nested field
	assert.Contains(t, result["datasource"], "public__products")
	productsFields := result["datasource"]["public__products"]
	assert.Contains(t, productsFields, "pricing.discount.percentage", "Nested field 'pricing.discount.percentage' should be preserved as-is")
	assert.Contains(t, productsFields, "available", "Condition field 'available' should be mapped")
}

func TestMappedFieldsOfTemplate_AggregationNestedFieldsNotTreatedAsDatasource(t *testing.T) {
	t.Parallel()

	// Regression test: ensure nested field paths like "fee_charge.totalAmount"
	// are NOT incorrectly split and treated as datasource references
	template := `{% sum_by db:schema.table by "fee_charge.totalAmount" if status == "COMPLETED" %}`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	// Should have "db" as datasource
	assert.Contains(t, result, "db")

	// Should have "schema__table" as collection
	assert.Contains(t, result["db"], "schema__table")

	// Should NOT have "fee_charge" as a separate datasource or collection
	// This was the bug: fee_charge.totalAmount was being split and fee_charge
	// was treated as a datasource
	_, hasFeeCharge := result["fee_charge"]
	assert.False(t, hasFeeCharge, "fee_charge should NOT be treated as a datasource")

	_, hasFeeChargeInDb := result["db"]["fee_charge"]
	assert.False(t, hasFeeChargeInDb, "fee_charge should NOT be treated as a collection under db")

	// The field should be preserved as-is in the correct collection
	tableFields := result["db"]["schema__table"]
	assert.Contains(t, tableFields, "fee_charge.totalAmount", "Nested field should be preserved as complete path")
}

func TestMappedFieldsOfTemplate_AggregationCompoundConditions(t *testing.T) {
	t.Parallel()

	// Test that compound conditions with "and" extract all field names
	template := `{% sum_by db:schema.transfers by "amount" if transfer_type == "CASHIN" and destination_person_type == "NATURAL_PERSON" and status == "COMPLETED" %}`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	// Should have "db" as datasource
	assert.Contains(t, result, "db")

	// Should have "schema__transfers" as collection
	assert.Contains(t, result["db"], "schema__transfers")

	// All fields from the compound condition should be extracted
	tableFields := result["db"]["schema__transfers"]
	assert.Contains(t, tableFields, "amount", "Field 'amount' from 'by' clause should be mapped")
	assert.Contains(t, tableFields, "transfer_type", "Field 'transfer_type' from first condition should be mapped")
	assert.Contains(t, tableFields, "destination_person_type", "Field 'destination_person_type' from second condition should be mapped")
	assert.Contains(t, tableFields, "status", "Field 'status' from third condition should be mapped")
}

func TestMappedFieldsOfTemplate_AggregationNestedJSONBWithCompoundConditions(t *testing.T) {
	t.Parallel()

	// Regression test: nested JSONB field path in "by" clause combined with compound conditions
	// This was causing TPL-0031 error because fee_charge.totalAmount was incorrectly treated as a datasource
	template := `{% sum_by sales_db:payment.transfers by "fee_charge.totalAmount" if transfer_type == "CASHIN" and destination_person_type == "NATURAL_PERSON" and status == "COMPLETED" %}`

	result := MappedFieldsOfTemplate(template)

	t.Logf("Result: %+v", result)

	// Should have "sales_db" as datasource
	assert.Contains(t, result, "sales_db")

	// Should have "payment__transfers" as collection
	assert.Contains(t, result["sales_db"], "payment__transfers")

	// Should NOT have "fee_charge" as a separate datasource
	_, hasFeeCharge := result["fee_charge"]
	assert.False(t, hasFeeCharge, "fee_charge should NOT be treated as a datasource")

	// The nested field should be preserved as-is
	tableFields := result["sales_db"]["payment__transfers"]
	assert.Contains(t, tableFields, "fee_charge.totalAmount", "Nested JSONB field should be preserved as complete path")
	assert.Contains(t, tableFields, "transfer_type", "Condition field should be mapped")
	assert.Contains(t, tableFields, "destination_person_type", "Condition field should be mapped")
	assert.Contains(t, tableFields, "status", "Condition field should be mapped")
}

func TestExtractFieldsFromConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions string
		expected   []string
	}{
		{
			name:       "single_condition",
			conditions: `transfer_type == "CASHIN"`,
			expected:   []string{"transfer_type", `"CASHIN"`},
		},
		{
			name:       "two_conditions",
			conditions: `transfer_type == "CASHIN" and status == "COMPLETED"`,
			expected:   []string{"transfer_type", `"CASHIN"`, "status", `"COMPLETED"`},
		},
		{
			name:       "three_conditions",
			conditions: `transfer_type == "CASHIN" and destination_person_type == "NATURAL_PERSON" and status == "COMPLETED"`,
			expected:   []string{"transfer_type", `"CASHIN"`, "destination_person_type", `"NATURAL_PERSON"`, "status", `"COMPLETED"`},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractFieldsFromConditions(tt.conditions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateNoScriptTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		// Valid templates (should pass)
		{
			name:        "Valid - Simple Pongo2 template",
			template:    `{% for item in db.table %}{{ item.name }}{% endfor %}`,
			expectError: false,
		},
		{
			name:        "Valid - Template with 'on' in text content",
			template:    `<div>organization</div><p>connection</p>`,
			expectError: false,
		},
		{
			name:        "Valid - Template with 'on' in variable names",
			template:    `{{ person.name }} {{ common_field }}`,
			expectError: false,
		},
		{
			name:        "Valid - HTML without XSS vectors",
			template:    `<html><body><h1>Report</h1><p>{{ data.content }}</p></body></html>`,
			expectError: false,
		},

		// Invalid templates - Script tags (original vulnerability)
		{
			name:        "Invalid - Exact <script> tag",
			template:    `<html><script>alert('x')</script></html>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag with src attribute",
			template:    `<script src="evil.js"></script>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag with type attribute",
			template:    `<script type="text/javascript">alert(1)</script>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag with multiple attributes",
			template:    `<script type="text/javascript" src="evil.js" defer></script>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag case insensitive",
			template:    `<SCRIPT>alert(1)</SCRIPT>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag mixed case",
			template:    `<ScRiPt>alert(1)</sCrIpT>`,
			expectError: true,
		},

		// Invalid templates - Inline event handlers (XSS bypasses)
		{
			name:        "Invalid - img onerror XSS",
			template:    `<img src="x" onerror="alert(1)">`,
			expectError: true,
		},
		{
			name:        "Invalid - svg onload XSS",
			template:    `<svg onload="alert(1)">`,
			expectError: true,
		},
		{
			name:        "Invalid - div onclick XSS",
			template:    `<div onclick="malicious()">Click me</div>`,
			expectError: true,
		},
		{
			name:        "Invalid - body onload XSS",
			template:    `<body onload="stealData()">`,
			expectError: true,
		},
		{
			name:        "Invalid - input onfocus XSS",
			template:    `<input type="text" onfocus="alert(1)">`,
			expectError: true,
		},
		{
			name:        "Invalid - a onmouseover XSS",
			template:    `<a href="#" onmouseover="alert(1)">Hover me</a>`,
			expectError: true,
		},
		{
			name:        "Invalid - iframe onload XSS",
			template:    `<iframe src="page.html" onload="stealCookies()"></iframe>`,
			expectError: true,
		},
		{
			name:        "Invalid - Event handler case insensitive",
			template:    `<img OnError="alert(1)">`,
			expectError: true,
		},
		{
			name:        "Invalid - Event handler with spaces",
			template:    `<img onerror = "alert(1)">`,
			expectError: true,
		},
		{
			name:        "Invalid - Multiple event handlers",
			template:    `<div onclick="x()" onmouseover="y()">Content</div>`,
			expectError: true,
		},

		// Invalid templates - Resource-loading tags (LFI vectors)
		{
			name:        "Invalid - iframe with file:// URL",
			template:    `<iframe src="file:///etc/passwd"></iframe>`,
			expectError: true,
		},
		{
			name:        "Invalid - iframe case insensitive",
			template:    `<IFRAME src="file:///etc/hostname"></IFRAME>`,
			expectError: true,
		},
		{
			name:        "Invalid - object with file:// URL",
			template:    `<object data="file:///etc/passwd"></object>`,
			expectError: true,
		},
		{
			name:        "Invalid - embed with file:// URL",
			template:    `<embed src="file:///etc/passwd">`,
			expectError: true,
		},
		{
			name:        "Invalid - iframe with http URL",
			template:    `<iframe src="http://evil.com/steal"></iframe>`,
			expectError: true,
		},

		// Edge cases
		{
			name:        "Invalid - Script tag at end of file",
			template:    `<html><body>content</body><script>alert(1)</script>`,
			expectError: true,
		},
		{
			name:        "Invalid - Multiple script tags",
			template:    `<script>x()</script><div>content</div><script>y()</script>`,
			expectError: true,
		},
		{
			name:        "Invalid - Script tag with newlines",
			template:    "<script>\nalert(1)\n</script>",
			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateNoScriptTag(tt.template)

			if tt.expectError {
				require.Error(t, err, "Expected error for template: %s", tt.template)
				assert.Equal(t, constant.ErrScriptTagDetected, err, "Should return ErrScriptTagDetected")
			} else {
				require.NoError(t, err, "Expected no error for template: %s", tt.template)
			}
		})
	}
}

func TestAppendIfMissingAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		slice    []any
		val      any
		expected []any
	}{
		{
			name:     "append new string to empty slice",
			slice:    []any{},
			val:      "field1",
			expected: []any{"field1"},
		},
		{
			name:     "skip duplicate string",
			slice:    []any{"field1"},
			val:      "field1",
			expected: []any{"field1"},
		},
		{
			name:     "append new string to existing slice",
			slice:    []any{"field1"},
			val:      "field2",
			expected: []any{"field1", "field2"},
		},
		{
			name:     "skip duplicate map by matching key",
			slice:    []any{map[string]any{"key1": []any{"a"}}},
			val:      map[string]any{"key1": []any{"b"}},
			expected: []any{map[string]any{"key1": []any{"a"}}},
		},
		{
			name:     "append new map with different key",
			slice:    []any{map[string]any{"key1": []any{"a"}}},
			val:      map[string]any{"key2": []any{"b"}},
			expected: []any{map[string]any{"key1": []any{"a"}}, map[string]any{"key2": []any{"b"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := appendIfMissingAny(tt.slice, tt.val)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInsertField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    map[string]any
		path       []string
		field      string
		checkField func(t *testing.T, m map[string]any)
	}{
		{
			name:    "empty path does nothing",
			initial: map[string]any{},
			path:    []string{},
			field:   "test",
			checkField: func(t *testing.T, m map[string]any) {
				assert.Empty(t, m)
			},
		},
		{
			name:    "single path creates field array",
			initial: map[string]any{},
			path:    []string{"collection"},
			field:   "field1",
			checkField: func(t *testing.T, m map[string]any) {
				fields, ok := m["collection"].([]any)
				require.True(t, ok)
				assert.Contains(t, fields, "field1")
			},
		},
		{
			name:    "nested path creates intermediate maps",
			initial: map[string]any{},
			path:    []string{"datasource", "collection"},
			field:   "field1",
			checkField: func(t *testing.T, m map[string]any) {
				dsMap, ok := m["datasource"].(map[string]any)
				require.True(t, ok)
				fields, ok := dsMap["collection"].([]any)
				require.True(t, ok)
				assert.Contains(t, fields, "field1")
			},
		},
		{
			name:    "appends to existing field array",
			initial: map[string]any{"collection": []any{"field1"}},
			path:    []string{"collection"},
			field:   "field2",
			checkField: func(t *testing.T, m map[string]any) {
				fields, ok := m["collection"].([]any)
				require.True(t, ok)
				assert.Contains(t, fields, "field1")
				assert.Contains(t, fields, "field2")
			},
		},
		{
			name: "navigates through existing array with map",
			initial: map[string]any{
				"datasource": []any{
					map[string]any{"collection": []any{"existing"}},
				},
			},
			path:  []string{"datasource", "collection"},
			field: "new_field",
			checkField: func(t *testing.T, m map[string]any) {
				dsArr, ok := m["datasource"].([]any)
				require.True(t, ok)

				dsMap, ok := dsArr[0].(map[string]any)
				require.True(t, ok)

				fields, ok := dsMap["collection"].([]any)
				require.True(t, ok)
				assert.Contains(t, fields, "existing")
				assert.Contains(t, fields, "new_field")
			},
		},
		{
			name: "creates new map when array has no map items",
			initial: map[string]any{
				"datasource": []any{"just_a_string"},
			},
			path:  []string{"datasource", "collection"},
			field: "field1",
			checkField: func(t *testing.T, m map[string]any) {
				dsArr, ok := m["datasource"].([]any)
				require.True(t, ok)
				// Should have appended a new map
				assert.Len(t, dsArr, 2)
			},
		},
		{
			name: "overwrites non-nil non-array non-map value",
			initial: map[string]any{
				"datasource": "not_a_map",
			},
			path:  []string{"datasource", "collection"},
			field: "field1",
			checkField: func(t *testing.T, m map[string]any) {
				dsMap, ok := m["datasource"].(map[string]any)
				require.True(t, ok)
				fields, ok := dsMap["collection"].([]any)
				require.True(t, ok)
				assert.Contains(t, fields, "field1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			insertField(tt.initial, tt.path, tt.field)
			tt.checkField(t, tt.initial)
		})
	}
}

func TestRegexBlockForOnPlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		templateFile string
		wantVars     map[string][]string
	}{
		{
			name:         "simple for loop",
			templateFile: `{% for item in datasource.collection %}{{ item.name }}{% endfor %}`,
			wantVars: map[string][]string{
				"item": {"datasource", "collection"},
			},
		},
		{
			name:         "for loop with DIMP where filter",
			templateFile: `{% for acc in midaz.account|where:"type:cacc" %}{{ acc.id }}{% endfor %}`,
			wantVars: map[string][]string{
				"acc": {"midaz", "account"},
			},
		},
		{
			name:         "for loop with schema syntax",
			templateFile: `{% for tx in db:schema.table %}{{ tx.id }}{% endfor %}`,
			wantVars: map[string][]string{
				"tx": {"db", "schema__table"},
			},
		},
		{
			name:         "multiple for loops",
			templateFile: `{% for a in ds1.tbl1 %}{% endfor %}{% for b in ds2.tbl2 %}{% endfor %}`,
			wantVars: map[string][]string{
				"a": {"ds1", "tbl1"},
				"b": {"ds2", "tbl2"},
			},
		},
		{
			name:         "for loop with three-part path",
			templateFile: `{% for item in datasource.schema.collection %}{{ item.name }}{% endfor %}`,
			wantVars: map[string][]string{
				"item": {"datasource", "schema", "collection"},
			},
		},
		{
			name:         "no for loop produces empty map",
			templateFile: `<html>{{ variable }}</html>`,
			wantVars:     map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := regexBlockForOnPlaceholder(tt.templateFile)

			for varName, expectedPath := range tt.wantVars {
				assert.Contains(t, result, varName)
				assert.Equal(t, expectedPath, result[varName])
			}

			assert.Len(t, result, len(tt.wantVars))
		})
	}
}

func TestInsertFieldToPath_NilCollection(t *testing.T) {
	t.Parallel()

	// Test when collection exists as nil in the map
	m := map[string]any{
		"datasource": map[string]any{
			"collection": nil,
		},
	}

	insertFieldToPath(m, []string{"datasource", "collection"}, "field1")

	dsMap := m["datasource"].(map[string]any)
	fields, ok := dsMap["collection"].([]any)
	require.True(t, ok)
	assert.Contains(t, fields, "field1")
}

func TestEnsureMapStructure_ExistingDatasource(t *testing.T) {
	t.Parallel()

	// Test when datasource exists but is not a map (edge case)
	m := map[string]any{
		"datasource": "not_a_map",
	}

	ensureMapStructure(m, []string{"datasource", "collection"})

	dsMap, ok := m["datasource"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, dsMap, "collection")
}

func TestRegexBlockForWithFilterOnPlaceholder_WithVariableMapResolution(t *testing.T) {
	t.Parallel()

	// Test with a param that references a known loop variable
	templateFile := `{% for item in filter(datasource.collection, related_party.field) %}{{ item.id }}{% endfor %}`
	variableMap := map[string][]string{
		"related_party": {"other_ds", "other_coll"},
	}
	result := map[string]any{}

	regexBlockForWithFilterOnPlaceholder(result, variableMap, templateFile)

	// "item" should be added to variableMap
	assert.Contains(t, variableMap, "item")
	assert.Equal(t, []string{"datasource", "collection"}, variableMap["item"])

	// The resolved param should be inserted into the other datasource
	assert.Contains(t, result, "other_ds")
}

func TestCleanPathLegacy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "simple path",
			path:     "a.b.c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "path with numeric index",
			path:     "a.0.b",
			expected: []string{"a", "b"},
		},
		{
			name:     "path with array bracket",
			path:     "a[0].b",
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := cleanPathLegacy(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFieldsFromExpressionOfAggregation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "simple if expression",
			expr:     `collection if field == "value"`,
			expected: []string{"collection", "field", `"value"`},
		},
		{
			name:     "by clause with single condition",
			expr:     `collection by "byField" if field == "value"`,
			expected: []string{"collection", "byField", "field", `"value"`},
		},
		{
			name:     "by clause with compound conditions",
			expr:     `collection by "byField" if f1 == "v1" and f2 == "v2"`,
			expected: []string{"collection", "byField", "f1", `"v1"`, "f2", `"v2"`},
		},
		{
			name:     "no match returns empty",
			expr:     "just_a_word",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractFieldsFromExpressionOfAggregation(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMappedFieldsOfTemplate_LastItemByGroup_Basic(t *testing.T) {
	t.Parallel()

	template := `{% last_item_by_group midaz_transaction.operation group_by "account_id" order_by "created_at" as listaOperations %}`

	result := MappedFieldsOfTemplate(template)

	// Should extract midaz_transaction.operation with fields account_id and created_at
	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	fields := result["midaz_transaction"]["operation"]
	assert.Contains(t, fields, "account_id")
	assert.Contains(t, fields, "created_at")
}

func TestMappedFieldsOfTemplate_LastItemByGroup_WithFilter(t *testing.T) {
	t.Parallel()

	template := `{% last_item_by_group midaz_transaction.operation group_by "account_id" order_by "created_at" if route as listaOperations %}`

	result := MappedFieldsOfTemplate(template)

	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	fields := result["midaz_transaction"]["operation"]
	assert.Contains(t, fields, "account_id")
	assert.Contains(t, fields, "created_at")
	assert.Contains(t, fields, "route")
}

func TestMappedFieldsOfTemplate_LastItemByGroup_WithSumBy(t *testing.T) {
	t.Parallel()

	template := `{% last_item_by_group midaz_transaction.operation group_by "account_id" order_by "created_at" if route as listaOperations %}
{%- for oproute in midaz_transaction.operation_route %}
{%- if oproute.code %}
    <registro>
      <conta>{{ oproute.code }}</conta>
      <saldoDia>{% sum_by listaOperations by "available_balance_after" if oproute.id == route %}</saldoDia>
    </registro>
{%- endif %}
{%- endfor %}`

	result := MappedFieldsOfTemplate(template)

	// Should extract midaz_transaction.operation with its fields
	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	operationFields := result["midaz_transaction"]["operation"]
	assert.Contains(t, operationFields, "account_id")
	assert.Contains(t, operationFields, "created_at")
	assert.Contains(t, operationFields, "route")
	assert.Contains(t, operationFields, "available_balance_after")

	// Should also extract midaz_transaction.operation_route with its fields
	require.Contains(t, result["midaz_transaction"], "operation_route")
	routeFields := result["midaz_transaction"]["operation_route"]
	assert.Contains(t, routeFields, "code")
	assert.Contains(t, routeFields, "id")
}

func TestMappedFieldsOfTemplate_LastItemByGroup_WithComplexFilter(t *testing.T) {
	t.Parallel()

	template := `{% last_item_by_group midaz_transaction.operation group_by "account_id" order_by "created_at" if status == "COMPLETED" as listaOperations %}`

	result := MappedFieldsOfTemplate(template)

	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	fields := result["midaz_transaction"]["operation"]
	assert.Contains(t, fields, "account_id")
	assert.Contains(t, fields, "created_at")
}

func TestMappedFieldsOfTemplate_LastItemByGroup_ExplicitSchema(t *testing.T) {
	t.Parallel()

	template := `{% last_item_by_group midaz_transaction:transaction.operation group_by "account_id" order_by "created_at" as listaOps %}`

	result := MappedFieldsOfTemplate(template)

	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "transaction__operation")
	fields := result["midaz_transaction"]["transaction__operation"]
	assert.Contains(t, fields, "account_id")
	assert.Contains(t, fields, "created_at")
}

func TestMappedFieldsOfTemplate_WithFilterAndNestedFor(t *testing.T) {
	t.Parallel()

	// Reproduces the pattern: with operations = filter(...) + for operation in operations
	// The "operations" variable is defined by "with" and iterated by "for".
	// Without re-resolving nested variables, "operations" would appear as a phantom data source.
	template := `{%- for holder in plugin_crm.holders %}
{%- for alias in plugin_crm.aliases %}
{%- if alias.holder_id == holder.id %}
{%- with operations = filter(midaz_transaction.operation, "account_id", alias.account_id) %}
{% sum_by operations by 'amount.value' %}
{%- for operation in operations %}
{{ operation.transaction_id }}
{%- endfor %}
{%- endwith %}
{%- endif %}
{%- endfor %}
{%- endfor %}`

	result := MappedFieldsOfTemplate(template)

	// "operations" must NOT appear as a data source - it's a local variable
	assert.NotContains(t, result, "operations", "local 'with' variable should not be a data source")

	// Fields should be correctly mapped to midaz_transaction.operation
	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	fields := result["midaz_transaction"]["operation"]
	assert.Contains(t, fields, "account_id")
	assert.Contains(t, fields, "transaction_id")
}

func TestMappedFieldsOfTemplate_ArithmeticWithLength(t *testing.T) {
	t.Parallel()

	// Realistic template: collections are used in for loops AND in an arithmetic |length expression.
	// The arithmetic expression must NOT create phantom data sources or overwrite existing field mappings.
	template := `{%- for holder in plugin_crm.holders %}{{ holder.name }}{%- endfor %}
{%- for alias in plugin_crm.aliases %}{{ alias.account_id }}{%- endfor %}
{%- for op in midaz_transaction.operation %}{{ op.amount }}{%- endfor %}
|9999|{{ 6 + plugin_crm.holders|length + plugin_crm.aliases|length + midaz_transaction.operation|length }}|`

	result := MappedFieldsOfTemplate(template)

	// "6 + plugin_crm" must NOT appear as a data source
	assert.NotContains(t, result, "6 + plugin_crm", "arithmetic operand should not be a data source")

	// Correct data sources and fields should be preserved from the for loops
	require.Contains(t, result, "plugin_crm")
	require.Contains(t, result["plugin_crm"], "holders")
	assert.Contains(t, result["plugin_crm"]["holders"], "name")
	require.Contains(t, result["plugin_crm"], "aliases")
	assert.Contains(t, result["plugin_crm"]["aliases"], "account_id")
	require.Contains(t, result, "midaz_transaction")
	require.Contains(t, result["midaz_transaction"], "operation")
	assert.Contains(t, result["midaz_transaction"]["operation"], "amount")
}
