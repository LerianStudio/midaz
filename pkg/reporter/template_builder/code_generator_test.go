// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCode_TextBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple text block",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello World"},
			},
			expected: "Hello World\n",
		},
		{
			name: "inline text block omits newline",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello", Inline: true},
				{Type: "text", Content: " World"},
			},
			expected: "Hello World\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_VariableBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple variable",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "name"},
			},
			expected: "{{ name }}\n",
		},
		{
			name: "variable with single filter",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "price", Filters: []FilterChain{
					{Name: "floatformat", Args: "2"},
				}},
			},
			expected: "{{ price|floatformat:\"2\" }}\n",
		},
		{
			name: "variable with multiple filters",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "items", Filters: []FilterChain{
					{Name: "where", Args: "state:SP"},
					{Name: "sum", Args: "value"},
				}},
			},
			expected: "{{ items|where:\"state:SP\"|sum:\"value\" }}\n",
		},
		{
			name: "variable with filter without args",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "total", Filters: []FilterChain{
					{Name: "strip_zeros"},
				}},
			},
			expected: "{{ total|strip_zeros }}\n",
		},
		{
			name: "inline variable omits newline",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "name", Inline: true},
			},
			expected: "{{ name }}",
		},
		{
			name: "filter args with injection attempt are escaped",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "name", Filters: []FilterChain{
					{Name: "replace", Args: `x" |safe`},
				}},
			},
			expected: "{{ name|replace:\"x\\\" |safe\" }}\n",
		},
		{
			name: "filter args with backslash and quotes are escaped",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "data", Filters: []FilterChain{
					{Name: "replace", Args: `a\b"c`},
				}},
			},
			expected: "{{ data|replace:\"a\\\\b\\\"c\" }}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_LoopBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "loop with text child",
			blocks: []TemplateBlock{
				{
					Type:       "loop",
					Iterator:   "item",
					Collection: "items",
					Children: []TemplateBlock{
						{Type: "variable", Variable: "item.name"},
					},
				},
			},
			expected: "{% for item in items %}\n    {{ item.name }}\n{% endfor %}\n",
		},
		{
			name: "nested loops",
			blocks: []TemplateBlock{
				{
					Type:       "loop",
					Iterator:   "group",
					Collection: "groups",
					Children: []TemplateBlock{
						{
							Type:       "loop",
							Iterator:   "item",
							Collection: "group.items",
							Children: []TemplateBlock{
								{Type: "variable", Variable: "item.value"},
							},
						},
					},
				},
			},
			expected: "{% for group in groups %}\n    {% for item in group.items %}\n        {{ item.value }}\n    {% endfor %}\n{% endfor %}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_ConditionalBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple conditional",
			blocks: []TemplateBlock{
				{
					Type:      "conditional",
					Condition: "show_total",
					Children: []TemplateBlock{
						{Type: "text", Content: "Total shown"},
					},
				},
			},
			expected: "{% if show_total %}\n    Total shown\n{% endif %}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_AggregationBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "sum aggregation",
			blocks: []TemplateBlock{
				{Type: "aggregation", Collection: "items", Function: "sum", Field: "value"},
			},
			expected: "{{ items|sum:\"value\" }}\n",
		},
		{
			name: "count aggregation",
			blocks: []TemplateBlock{
				{Type: "aggregation", Collection: "orders", Function: "count", Field: "status:active"},
			},
			expected: "{{ orders|count:\"status:active\" }}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_CalculationBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "percent_of calculation",
			blocks: []TemplateBlock{
				{Type: "calculation", Variable: "total", Function: "percent_of"},
			},
			expected: "{{ total|percent_of }}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_DateTimeBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "date with format",
			blocks: []TemplateBlock{
				{Type: "date_time", Variable: "created_at", Format: "02/01/2006"},
			},
			expected: "{{ created_at|date:\"02/01/2006\" }}\n",
		},
		{
			name: "date format with quotes is escaped",
			blocks: []TemplateBlock{
				{Type: "date_time", Variable: "created_at", Format: `02"01`},
			},
			expected: "{{ created_at|date:\"02\\\"01\" }}\n",
		},
		{
			name: "now mode format with quotes is escaped",
			blocks: []TemplateBlock{
				{Type: "date_time", Format: `dd\"MM`},
			},
			expected: "{% date_time \"dd\\\\\\\"MM\" %}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_CounterBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "counter increment mode",
			blocks: []TemplateBlock{
				{
					Type: "counter",
					Properties: map[string]interface{}{
						"counterMode":  "increment",
						"counterNames": []interface{}{"total"},
					},
				},
			},
			expected: "{% counter \"total\" %}\n",
		},
		{
			name: "counter show mode with multiple names",
			blocks: []TemplateBlock{
				{
					Type: "counter",
					Properties: map[string]interface{}{
						"counterMode":  "show",
						"counterNames": []interface{}{"name1", "name2"},
					},
				},
			},
			expected: "{% counter_show \"name1\" \"name2\" %}\n",
		},
		{
			name: "counter increment inline",
			blocks: []TemplateBlock{
				{
					Type:   "counter",
					Inline: true,
					Properties: map[string]interface{}{
						"counterMode":  "increment",
						"counterNames": []interface{}{"cnt"},
					},
				},
			},
			expected: "{% counter \"cnt\" %}",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_CounterBlock_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		expectErr string
	}{
		{
			name: "counter with nil properties",
			blocks: []TemplateBlock{
				{Type: "counter"},
			},
			expectErr: "counter block requires properties",
		},
		{
			name: "counter with empty names",
			blocks: []TemplateBlock{
				{
					Type: "counter",
					Properties: map[string]interface{}{
						"counterMode":  "increment",
						"counterNames": []interface{}{},
					},
				},
			},
			expectErr: "counter block requires at least one name",
		},
		{
			name: "counter with unknown mode",
			blocks: []TemplateBlock{
				{
					Type: "counter",
					Properties: map[string]interface{}{
						"counterMode":  "reset",
						"counterNames": []interface{}{"x"},
					},
				},
			},
			expectErr: "unknown counter mode: reset",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := GenerateCode(tt.blocks, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErr)
		})
	}
}

func TestGenerateCode_CommentBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple comment",
			blocks: []TemplateBlock{
				{Type: "comment", Content: "This is a comment"},
			},
			expected: "{# This is a comment #}\n",
		},
		{
			name: "comment with closing sequence sanitized",
			blocks: []TemplateBlock{
				{Type: "comment", Content: "safe #}{% set x = secret %}{{ x }}{# rest"},
			},
			expected: "{# safe # }{% set x = secret %}{{ x }}{# rest #}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_SectionBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "section generates only children",
			blocks: []TemplateBlock{
				{
					Type: "section",
					Children: []TemplateBlock{
						{Type: "text", Content: "Inside section"},
						{Type: "variable", Variable: "data"},
					},
				},
			},
			expected: "Inside section\n{{ data }}\n",
		},
		{
			name: "nested sections",
			blocks: []TemplateBlock{
				{
					Type: "section",
					Children: []TemplateBlock{
						{Type: "text", Content: "Outer"},
						{
							Type: "section",
							Children: []TemplateBlock{
								{Type: "text", Content: "Inner"},
							},
						},
					},
				},
			},
			expected: "Outer\nInner\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_FormatHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		format   string
		contains []string
	}{
		{
			name: "HTML format wraps with doctype",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello"},
			},
			format:   "html",
			contains: []string{"<!DOCTYPE html>", "<html>", "<body>", "</body>", "</html>", "Hello"},
		},
		{
			name: "XML format wraps with xml declaration",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello"},
			},
			format:   "xml",
			contains: []string{"<?xml version=\"1.0\" encoding=\"UTF-8\"?>", "Hello"},
		},
		{
			name: "TXT format no wrapping",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello"},
			},
			format:   "txt",
			contains: []string{"Hello"},
		},
		{
			name: "empty format no wrapping",
			blocks: []TemplateBlock{
				{Type: "text", Content: "Hello"},
			},
			format:   "",
			contains: []string{"Hello"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, tt.format)
			require.NoError(t, err)

			for _, substr := range tt.contains {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestGenerateCode_EmptyBlocks(t *testing.T) {
	t.Parallel()

	_, err := GenerateCode(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocks must not be empty")
}

func TestGenerateCode_UnknownBlockType(t *testing.T) {
	t.Parallel()

	_, err := GenerateCode([]TemplateBlock{
		{Type: "unknown"},
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown block type")
}

func TestGenerateCode_TrimWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "loop with trim whitespace",
			blocks: []TemplateBlock{
				{
					Type:           "loop",
					Iterator:       "item",
					Collection:     "items",
					TrimWhitespace: true,
					Children: []TemplateBlock{
						{Type: "variable", Variable: "item.name"},
					},
				},
			},
			expected: "{%- for item in items -%}\n{{ item.name }}\n{%- endfor -%}\n",
		},
		{
			name: "conditional with trim whitespace",
			blocks: []TemplateBlock{
				{
					Type:           "conditional",
					Condition:      "x > 0",
					TrimWhitespace: true,
					Children: []TemplateBlock{
						{Type: "text", Content: "yes"},
					},
				},
			},
			expected: "{%- if x > 0 -%}\nyes\n{%- endif -%}\n",
		},
		{
			name: "loop without trim keeps original format",
			blocks: []TemplateBlock{
				{
					Type:       "loop",
					Iterator:   "item",
					Collection: "items",
					Children: []TemplateBlock{
						{Type: "text", Content: "x"},
					},
				},
			},
			expected: "{% for item in items %}\n    x\n{% endfor %}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_ElseChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "conditional with else branch",
			blocks: []TemplateBlock{
				{
					Type:      "conditional",
					Condition: "user.name",
					Children: []TemplateBlock{
						{Type: "variable", Variable: "user.name"},
					},
					ElseChildren: []TemplateBlock{
						{Type: "text", Content: "Anonymous"},
					},
				},
			},
			expected: "{% if user.name %}\n    {{ user.name }}\n{% else %}\n    Anonymous\n{% endif %}\n",
		},
		{
			name: "conditional with else and trim",
			blocks: []TemplateBlock{
				{
					Type:           "conditional",
					Condition:      "holder.legal_person.trade_name",
					TrimWhitespace: true,
					Children: []TemplateBlock{
						{Type: "variable", Variable: "holder.legal_person.trade_name", Inline: true},
					},
					ElseChildren: []TemplateBlock{
						{Type: "variable", Variable: "holder.name", Inline: true},
					},
				},
			},
			expected: "{%- if holder.legal_person.trade_name -%}\n{{ holder.legal_person.trade_name }}{%- else -%}\n{{ holder.name }}{%- endif -%}\n",
		},
		{
			name: "conditional without else",
			blocks: []TemplateBlock{
				{
					Type:      "conditional",
					Condition: "x",
					Children: []TemplateBlock{
						{Type: "text", Content: "yes"},
					},
				},
			},
			expected: "{% if x %}\n    yes\n{% endif %}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_ElifBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "if/elif/else inline",
			blocks: []TemplateBlock{
				{
					Type:      "conditional",
					Condition: "account.type == \"deposit\"",
					Inline:    true,
					Children: []TemplateBlock{
						{Type: "text", Content: "1", Inline: true},
					},
					ElifBranches: []ElifBranch{
						{
							Condition: "account.type == \"savings\"",
							Children:  []TemplateBlock{{Type: "text", Content: "2", Inline: true}},
						},
						{
							Condition: "account.type == \"payment\"",
							Children:  []TemplateBlock{{Type: "text", Content: "3", Inline: true}},
						},
					},
					ElseChildren: []TemplateBlock{
						{Type: "text", Content: "6", Inline: true},
					},
				},
			},
			expected: "{% if account.type == \"deposit\" %}1{% elif account.type == \"savings\" %}2{% elif account.type == \"payment\" %}3{% else %}6{% endif %}",
		},
		{
			name: "if/elif/else block with trim",
			blocks: []TemplateBlock{
				{
					Type:           "conditional",
					Condition:      "alias.banking_details.type == \"CACC\"",
					TrimWhitespace: true,
					Children: []TemplateBlock{
						{Type: "text", Content: "<CtCli>", Inline: true},
						{Type: "variable", Variable: "alias.banking_details.account", Inline: true},
						{Type: "text", Content: "</CtCli>"},
					},
					ElifBranches: []ElifBranch{
						{
							Condition: "alias.banking_details.type == \"payment\"",
							Children: []TemplateBlock{
								{Type: "text", Content: "<CtPgto>", Inline: true},
								{Type: "variable", Variable: "alias.banking_details.account", Inline: true},
								{Type: "text", Content: "</CtPgto>"},
							},
						},
					},
				},
			},
			expected: "{%- if alias.banking_details.type == \"CACC\" -%}\n<CtCli>{{ alias.banking_details.account }}</CtCli>\n{%- elif alias.banking_details.type == \"payment\" -%}\n<CtPgto>{{ alias.banking_details.account }}</CtPgto>\n{%- endif -%}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_WithBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple with block",
			blocks: []TemplateBlock{
				{
					Type:       "with",
					Variable:   "ops",
					Assignment: "filter(midaz_transaction.operation, \"account_id\", alias.account_id)",
					Children: []TemplateBlock{
						{Type: "variable", Variable: "ops"},
					},
				},
			},
			expected: "{% with ops = filter(midaz_transaction.operation, \"account_id\", alias.account_id) %}\n    {{ ops }}\n{% endwith %}\n",
		},
		{
			name: "with block with trim",
			blocks: []TemplateBlock{
				{
					Type:           "with",
					Variable:       "operations",
					Assignment:     "filter(midaz_transaction.operation, \"account_id\", alias.account_id)",
					TrimWhitespace: true,
					Children: []TemplateBlock{
						{Type: "variable", Variable: "operations", Inline: true, Filters: []FilterChain{{Name: "length"}}},
					},
				},
			},
			expected: "{%- with operations = filter(midaz_transaction.operation, \"account_id\", alias.account_id) -%}\n{{ operations|length }}{%- endwith -%}\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_ExpressionBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "simple expression",
			blocks: []TemplateBlock{
				{Type: "expression", Expression: "6 + items|length"},
			},
			expected: "{{ 6 + items|length }}\n",
		},
		{
			name: "inline expression",
			blocks: []TemplateBlock{
				{Type: "expression", Expression: "6 + plugin_crm.holders|length + plugin_crm.aliases|length", Inline: true},
			},
			expected: "{{ 6 + plugin_crm.holders|length + plugin_crm.aliases|length }}",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_CustomTagBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected string
	}{
		{
			name: "sum_by tag",
			blocks: []TemplateBlock{
				{Type: "custom_tag", TagName: "sum_by", TagArgs: "listaOperations by \"available_balance_after\" if oproute.id == route", Inline: true},
			},
			expected: "{% sum_by listaOperations by \"available_balance_after\" if oproute.id == route %}",
		},
		{
			name: "date_time now mode",
			blocks: []TemplateBlock{
				{Type: "date_time", Format: "YYYY/MM/dd", Inline: true},
			},
			expected: "{% date_time \"YYYY/MM/dd\" %}",
		},
		{
			name: "last_item_by_group tag",
			blocks: []TemplateBlock{
				{Type: "custom_tag", TagName: "last_item_by_group", TagArgs: "midaz_transaction.operation group_by \"account_id,route\" order_by \"created_at\" if route as listaOperations"},
			},
			expected: "{% last_item_by_group midaz_transaction.operation group_by \"account_id,route\" order_by \"created_at\" if route as listaOperations %}\n",
		},
		{
			name: "custom_tag with trim",
			blocks: []TemplateBlock{
				{Type: "custom_tag", TagName: "sum_by", TagArgs: "ops by \"value\"", TrimWhitespace: true, Inline: true},
			},
			expected: "{%- sum_by ops by \"value\" -%}",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := GenerateCode(tt.blocks, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCode_ComplexTemplate(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "text", Content: "Report Title", Inline: true},
		{Type: "text", Content: "\n"},
		{
			Type:       "loop",
			Iterator:   "item",
			Collection: "transactions",
			Children: []TemplateBlock{
				{Type: "variable", Variable: "item.description", Inline: true},
				{Type: "text", Content: ": "},
				{Type: "variable", Variable: "item.amount", Filters: []FilterChain{
					{Name: "floatformat", Args: "2"},
				}},
			},
		},
		{Type: "aggregation", Collection: "transactions", Function: "sum", Field: "amount"},
	}

	result, err := GenerateCode(blocks, "")
	require.NoError(t, err)
	assert.Contains(t, result, "Report Title")
	assert.Contains(t, result, "{% for item in transactions %}")
	assert.Contains(t, result, "{{ item.description }}")
	assert.Contains(t, result, "{{ item.amount|floatformat:\"2\" }}")
	assert.Contains(t, result, "{% endfor %}")
	assert.Contains(t, result, "{{ transactions|sum:\"amount\" }}")
}

// ─── Security Tests ──────────────────────────────────────────────────────────

func TestGenerateCode_DepthLimitExceeded(t *testing.T) {
	t.Parallel()

	// Build 52 nested sections (exceeds maxGenDepth=50)
	block := TemplateBlock{Type: "text", Content: "leaf"}
	for i := 0; i < 52; i++ {
		block = TemplateBlock{Type: "section", Children: []TemplateBlock{block}}
	}

	_, err := GenerateCode([]TemplateBlock{block}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum nesting depth exceeded")
}

func TestGenerateCode_DepthAtExactLimit(t *testing.T) {
	t.Parallel()

	// Build exactly 50 nested sections (should pass)
	block := TemplateBlock{Type: "text", Content: "leaf"}
	for i := 0; i < 50; i++ {
		block = TemplateBlock{Type: "section", Children: []TemplateBlock{block}}
	}

	_, err := GenerateCode([]TemplateBlock{block}, "")
	assert.NoError(t, err)
}

func TestGenerateCode_CustomTag_UnknownName_Error(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{{
		Type:    "custom_tag",
		TagName: "malicious_tag",
		TagArgs: "some args",
	}}

	_, err := GenerateCode(blocks, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown custom tag: malicious_tag")
}

func TestGenerateCode_CustomTag_ValidName_OK(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{{
		Type:    "custom_tag",
		TagName: "sum_by",
		TagArgs: "data by \"amount\"",
	}}

	result, err := GenerateCode(blocks, "")
	require.NoError(t, err)
	assert.Contains(t, result, "sum_by data by \"amount\"")
}
